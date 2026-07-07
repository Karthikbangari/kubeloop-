package pr

import (
	"strings"
	"testing"
)

const manifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
  namespace: shop
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: app
          image: myco/checkout:1.2  # pinned image
          resources:
            requests:
              cpu: 2000m
              memory: 512Mi
`

func TestPatch_UpdatesRequestsPreservesRest(t *testing.T) {
	out, err := Patch([]byte(manifest), Target{Kind: "Deployment", Name: "checkout-api", Namespace: "shop", Container: "app"}, "492m", "428Mi")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "cpu: 492m") || !strings.Contains(s, "memory: 428Mi") {
		t.Errorf("requests not updated:\n%s", s)
	}
	if strings.Contains(s, "2000m") || strings.Contains(s, "512Mi") {
		t.Errorf("old values still present:\n%s", s)
	}
	// Untouched fields and the comment survive.
	for _, want := range []string{"name: checkout-api", "replicas: 3", "image: myco/checkout:1.2", "# pinned image"} {
		if !strings.Contains(s, want) {
			t.Errorf("lost %q:\n%s", want, s)
		}
	}
}

func TestPatch_OnlyCPUWhenMemEmpty(t *testing.T) {
	out, err := Patch([]byte(manifest), Target{Kind: "Deployment", Name: "checkout-api", Container: "app"}, "492m", "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "cpu: 492m") || !strings.Contains(s, "memory: 512Mi") {
		t.Errorf("cpu-only patch wrong:\n%s", s)
	}
}

func TestPatch_IdentityMismatchErrors(t *testing.T) {
	if _, err := Patch([]byte(manifest), Target{Kind: "StatefulSet", Name: "checkout-api", Container: "app"}, "492m", ""); err == nil {
		t.Error("want error on kind mismatch")
	}
	if _, err := Patch([]byte(manifest), Target{Kind: "Deployment", Name: "other", Container: "app"}, "492m", ""); err == nil {
		t.Error("want error on name mismatch")
	}
	if _, err := Patch([]byte(manifest), Target{Kind: "Deployment", Name: "checkout-api", Namespace: "prod", Container: "app"}, "492m", ""); err == nil {
		t.Error("want error on namespace mismatch")
	}
}

func TestPatch_MissingContainerOrRequestsErrors(t *testing.T) {
	if _, err := Patch([]byte(manifest), Target{Kind: "Deployment", Name: "checkout-api", Container: "sidecar"}, "492m", ""); err == nil {
		t.Error("want error when container absent")
	}
	noReq := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
spec:
  template:
    spec:
      containers:
        - name: app
          image: myco/checkout:1.2
`
	if _, err := Patch([]byte(noReq), Target{Kind: "Deployment", Name: "checkout-api", Container: "app"}, "492m", ""); err == nil {
		t.Error("want error when resources.requests absent")
	}
}

// Regression: a StatefulSet with flow-style requests must patch correctly and
// keep the inline `{cpu: ..., memory: ...}` style.
func TestPatch_StatefulSetFlowStyleRequests(t *testing.T) {
	ss := `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: pg
  namespace: data
spec:
  template:
    spec:
      containers:
        - name: db
          resources:
            requests: {cpu: 2000m, memory: 1Gi}
`
	out, err := Patch([]byte(ss), Target{Kind: "StatefulSet", Name: "pg", Namespace: "data", Container: "db"}, "576m", "528Mi")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "requests: {cpu: 576m, memory: 528Mi}") {
		t.Errorf("expected flow-style patched requests, got:\n%s", s)
	}
}
