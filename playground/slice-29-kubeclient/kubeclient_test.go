package kubeclient

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// deployList is what `kubectl get deployments -o json` returns: a List wrapper
// whose items carry no "kind" of their own.
const deployList = `{"apiVersion":"v1","kind":"List","items":[
  {"metadata":{"name":"checkout-api","namespace":"shop"},
   "spec":{"replicas":2,"template":{"spec":{"containers":[
     {"name":"app","image":"eclipse-temurin:21-jre","resources":{"requests":{"cpu":"2000m","memory":"1Gi"}}}]}}}}
]}`

const stsList = `{"apiVersion":"v1","kind":"List","items":[
  {"metadata":{"name":"pg","namespace":"data"},
   "spec":{"template":{"spec":{"containers":[
     {"name":"db","resources":{"requests":{"cpu":"1","memory":"2Gi"}}}]}}}}
]}`

const emptyList = `{"apiVersion":"v1","kind":"List","items":[]}`

// fake records the commands it is asked to run and replays canned stdout by
// resource name.
type fake struct {
	calls [][]string
	byRes map[string]string
	err   error
}

func (f *fake) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.err != nil {
		return nil, f.err
	}
	for _, a := range args {
		if body, ok := f.byRes[a]; ok {
			return []byte(body), nil
		}
	}
	return []byte(emptyList), nil
}

func newFake() *fake {
	return &fake{byRes: map[string]string{"deployments": deployList, "statefulsets": stsList}}
}

func TestList_ParsesBothKindsAndFillsKind(t *testing.T) {
	f := newFake()
	c := &Client{Run: f.run}
	ws, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 {
		t.Fatalf("got %d workloads, want 2", len(ws))
	}
	// kubectl omitted "kind" on the items; the client must fill it from the
	// resource asked for, or safety's exclusion rules key off an empty Kind.
	if ws[0].Kind != "Deployment" || ws[0].Name != "checkout-api" || ws[0].Replicas != 2 {
		t.Errorf("deployment = %+v, want Deployment/checkout-api/2 replicas", ws[0])
	}
	if ws[1].Kind != "StatefulSet" || ws[1].Name != "pg" {
		t.Errorf("statefulset = %+v, want StatefulSet/pg", ws[1])
	}
	// Requests came through kubeparse → quantityparse.
	if ws[0].Containers[0].CPU != 2000 || ws[0].Containers[0].Mem != 1<<30 {
		t.Errorf("requests = %+v, want 2000m/1Gi", ws[0].Containers[0])
	}
}

// The tool must never mutate a cluster: every kubectl invocation is a `get`.
func TestList_OnlyEverIssuesGet(t *testing.T) {
	f := newFake()
	if _, err := (&Client{Run: f.run}).List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("want one call per kind, got %d", len(f.calls))
	}
	mutating := []string{"apply", "delete", "patch", "edit", "create", "replace", "scale", "annotate", "label"}
	for _, call := range f.calls {
		if call[0] != "kubectl" || call[1] != "get" {
			t.Errorf("call %v: want `kubectl get ...`", call)
		}
		for _, a := range call {
			for _, verb := range mutating {
				if a == verb {
					t.Errorf("call %v contains mutating verb %q", call, verb)
				}
			}
		}
	}
}

func TestList_AllNamespacesByDefault(t *testing.T) {
	f := newFake()
	if _, err := (&Client{Run: f.run}).List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.calls[0], " "), "--all-namespaces") {
		t.Errorf("want --all-namespaces by default, got %v", f.calls[0])
	}
}

func TestList_ScopesToNamespaceAndContext(t *testing.T) {
	f := newFake()
	c := &Client{Run: f.run, Namespace: "shop", Context: "prod"}
	if _, err := c.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(f.calls[0], " ")
	for _, want := range []string{"-n shop", "--context prod"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "--all-namespaces") {
		t.Errorf("args %q should not scope to all namespaces when -n is set", got)
	}
}

// A value beginning with "-" would be parsed by kubectl as a flag.
func TestList_RefusesFlagInjection(t *testing.T) {
	for _, c := range []*Client{
		{Run: newFake().run, Namespace: "--kubeconfig=/tmp/evil"},
		{Run: newFake().run, Context: "-n"},
	} {
		if _, err := c.List(context.Background()); err == nil {
			t.Errorf("want refusal for namespace=%q context=%q", c.Namespace, c.Context)
		}
	}
}

func TestList_EmptyClusterIsNotAnError(t *testing.T) {
	f := &fake{byRes: map[string]string{}}
	ws, err := (&Client{Run: f.run}).List(context.Background())
	if err != nil || len(ws) != 0 {
		t.Errorf("empty cluster: got %d workloads, err=%v; want 0, nil", len(ws), err)
	}
}

func TestList_SurfacesKubectlError(t *testing.T) {
	f := newFake()
	f.err = errors.New("forbidden: cannot list deployments")
	_, err := (&Client{Run: f.run}).List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("want the kubectl error surfaced, got %v", err)
	}
}

func TestList_MalformedJSONErrors(t *testing.T) {
	f := &fake{byRes: map[string]string{"deployments": "{not json"}}
	if _, err := (&Client{Run: f.run}).List(context.Background()); err == nil {
		t.Error("want error on malformed kubectl output")
	}
}

// A bad quantity in a live object must fail loudly, not silently zero the
// request and turn the workload into fake "waste".
func TestList_MalformedQuantityErrors(t *testing.T) {
	bad := `{"items":[{"metadata":{"name":"x"},"spec":{"template":{"spec":{"containers":[
	  {"name":"app","resources":{"requests":{"cpu":"lots"}}}]}}}}]}`
	f := &fake{byRes: map[string]string{"deployments": bad}}
	if _, err := (&Client{Run: f.run}).List(context.Background()); err == nil {
		t.Error("want error on malformed cpu quantity")
	}
}
