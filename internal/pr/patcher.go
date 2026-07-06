// Package pr contains the offline-testable pieces of the PR engine: manifest
// patching and reviewer-facing PR text. It does not locate manifests or call
// GitHub; those require a target repo, render tools, and credentials.
package pr

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Target identifies which container's requests to change.
type Target struct {
	Kind      string // "Deployment", "StatefulSet"
	Name      string
	Namespace string // optional; if set, must match
	Container string
}

// Patch updates the target container's cpu/memory requests in a single-document
// YAML manifest and returns the rewritten YAML. cpu/mem are Kubernetes quantity
// strings ("492m", "1Gi"); an empty string leaves that resource unchanged.
// It errors rather than guessing if the identity doesn't match or the
// resources.requests path is absent — a wrong or blind patch is worse than none.
func Patch(doc []byte, t Target, cpu, mem string) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return nil, fmt.Errorf("expected a single YAML document")
	}
	obj := root.Content[0]

	if got := scalar(obj, "kind"); got != t.Kind {
		return nil, fmt.Errorf("kind %q does not match target %q", got, t.Kind)
	}
	meta := child(obj, "metadata")
	if got := scalar(meta, "name"); got != t.Name {
		return nil, fmt.Errorf("metadata.name %q does not match target %q", got, t.Name)
	}
	if t.Namespace != "" {
		if got := scalar(meta, "namespace"); got != t.Namespace {
			return nil, fmt.Errorf("metadata.namespace %q does not match target %q", got, t.Namespace)
		}
	}

	containers := child(child(child(obj, "spec"), "template"), "spec")
	containers = child(containers, "containers")
	if containers == nil || containers.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("spec.template.spec.containers not found")
	}
	var target *yaml.Node
	for _, c := range containers.Content {
		if scalar(c, "name") == t.Container {
			target = c
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("container %q not found", t.Container)
	}

	requests := child(child(target, "resources"), "requests")
	if requests == nil || requests.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("container %q has no resources.requests to patch", t.Container)
	}
	if cpu != "" {
		setScalar(requests, "cpu", cpu)
	}
	if mem != "" {
		setScalar(requests, "memory", mem)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2) // ponytail: assumes 2-space manifests (the k8s norm)
	if err := enc.Encode(&root); err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// child returns the value node for key in a mapping, or nil.
func child(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// scalar returns the string value of a scalar child, or "".
func scalar(m *yaml.Node, key string) string {
	if v := child(m, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

// setScalar updates an existing scalar value in place (preserving its style and
// comments — the minimal change), or appends the key if absent.
func setScalar(m *yaml.Node, key, val string) {
	if v := child(m, key); v != nil {
		v.Value = val
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: val},
	)
}
