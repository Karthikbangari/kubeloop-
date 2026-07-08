// Package kubeclient lists workloads from a live cluster by shelling out to
// `kubectl get ... -o json` and parsing the result with kubeparse.
//
// Why kubectl and not client-go: kubeparse already consumes exactly what
// `kubectl get -o json` emits, and kubectl inherits the user's kubeconfig auth
// — including the EKS/GKE/AKS exec credential plugins that are the hard,
// security-sensitive part of talking to a real cluster. That keeps the project
// on its single dependency (yaml.v3) instead of pulling in client-go's tree.
// The cost is a kubectl binary on PATH; a future hosted scanner will need a
// real in-process client.
//
// Read-only by construction: the only verb this package ever passes to kubectl
// is `get`. There is no code path that mutates a cluster.
//
// Offline-provable: the command is behind a Runner, so every branch below is
// tested with a fake. What is NOT proven here is that a real kubectl against a
// real cluster returns what the fixtures assume.
package kubeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kubeloop/kubeloop/internal/readlayer/kubeparse"
)

// listKinds are the workload kinds kubeloop sizes, as kubectl resource names
// mapped to the Kind string the scanner's safety rules match on. Pods, Jobs and
// CronJobs are deliberately absent: safety excludes batch work anyway, and a
// bare Pod has no spec.template for kubeparse to read.
var listKinds = []struct{ resource, kind string }{
	{"deployments", "Deployment"},
	{"statefulsets", "StatefulSet"},
}

// Runner executes a command and returns its stdout. Injectable so the whole
// package is testable without a cluster.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner runs the command for real, surfacing stderr in the error (kubectl
// puts "connection refused" / "forbidden" there, and a bare exit-1 is useless
// to a user debugging RBAC).
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		}
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

// Client lists workloads via kubectl.
type Client struct {
	Binary    string // kubectl binary; "" → "kubectl"
	Context   string // --context; "" → kubeconfig's current context
	Namespace string // -n; "" → --all-namespaces
	Run       Runner // "" → ExecRunner
}

// New returns a Client that shells out to kubectl across all namespaces.
func New() *Client { return &Client{} }

func (c *Client) binary() string {
	if c.Binary == "" {
		return "kubectl"
	}
	return c.Binary
}

func (c *Client) runner() Runner {
	if c.Run == nil {
		return ExecRunner
	}
	return c.Run
}

// List returns every Deployment and StatefulSet in scope. A cluster with none
// yields an empty slice, not an error — "nothing to size" is a valid answer.
func (c *Client) List(ctx context.Context) ([]kubeparse.Workload, error) {
	// A namespace or context beginning with "-" would be read by kubectl as a
	// flag, not a value. Refuse rather than let a crafted value inject flags.
	if strings.HasPrefix(c.Namespace, "-") {
		return nil, fmt.Errorf("invalid namespace %q: must not begin with '-'", c.Namespace)
	}
	if strings.HasPrefix(c.Context, "-") {
		return nil, fmt.Errorf("invalid context %q: must not begin with '-'", c.Context)
	}
	var out []kubeparse.Workload
	for _, k := range listKinds {
		ws, err := c.listKind(ctx, k.resource, k.kind)
		if err != nil {
			return nil, err
		}
		out = append(out, ws...)
	}
	return out, nil
}

// listKind runs one read-only `kubectl get <resource> -o json`.
func (c *Client) listKind(ctx context.Context, resource, kind string) ([]kubeparse.Workload, error) {
	args := []string{"get", resource, "-o", "json"}
	if c.Namespace == "" {
		args = append(args, "--all-namespaces")
	} else {
		args = append(args, "-n", c.Namespace)
	}
	if c.Context != "" {
		args = append(args, "--context", c.Context)
	}
	raw, err := c.runner()(ctx, c.binary(), args...)
	if err != nil {
		return nil, err
	}
	return parseList(raw, kind)
}

// parseList reads kubectl's List wrapper and parses each item.
//
// kubectl does not reliably stamp `kind` onto the items of a single-resource
// List, so we fill it from the resource we asked for. Getting this wrong would
// be silent and expensive: safety's exclusion rules key off Kind, and an empty
// Kind would sail past the CronJob/Job exclusion as if it were sizable.
func parseList(raw []byte, kind string) ([]kubeparse.Workload, error) {
	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parse %s list: %w", kind, err)
	}
	out := make([]kubeparse.Workload, 0, len(list.Items))
	for i, item := range list.Items {
		w, err := kubeparse.Parse(item)
		if err != nil {
			return nil, fmt.Errorf("%s item %d: %w", kind, i, err)
		}
		if w.Kind == "" {
			w.Kind = kind
		}
		out = append(out, w)
	}
	return out, nil
}
