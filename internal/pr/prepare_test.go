package pr

import (
	"strings"
	"testing"
)

const checkout = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
  namespace: shop
spec:
  template:
    spec:
      containers:
        - name: app
          image: myco/checkout:1.2
          resources:
            requests:
              cpu: 2000m
              memory: 512Mi
`

func req() Request {
	return Request{
		Files: []File{
			{Path: "helm/checkout.yaml", Content: []byte(checkout)},
			{Path: "helm/reco.yaml", Content: []byte("kind: Deployment\nmetadata:\n  name: reco\n")},
		},
		Ref:        Ref{Kind: "Deployment", Name: "checkout-api", Namespace: "shop"},
		Container:  "app",
		CurrentCPU: "2000m", ProposedCPU: "492m",
		CurrentMem: "512Mi", ProposedMem: "428Mi",
		MonthlyUSD:  131.2,
		Confidence:  "high",
		Realization: "realized when nodes consolidate",
	}
}

func TestPrepare_EndToEnd(t *testing.T) {
	got, err := Prepare(req())
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "helm/checkout.yaml" {
		t.Errorf("path = %q, want helm/checkout.yaml", got.Path)
	}
	if !strings.Contains(string(got.Content), "cpu: 492m") || strings.Contains(string(got.Content), "2000m") {
		t.Errorf("patched content wrong:\n%s", got.Content)
	}
	if !strings.Contains(got.Title, "$131") {
		t.Errorf("title = %q", got.Title)
	}
	if !strings.Contains(got.Body, "revert this commit") || !strings.Contains(got.Body, "CPU `2000m`") {
		t.Errorf("body missing rollback:\n%s", got.Body)
	}
}

func TestPrepare_PropagatesLocateError(t *testing.T) {
	r := req()
	r.Ref.Name = "missing"
	if _, err := Prepare(r); err == nil {
		t.Error("want error when workload not found")
	}
}

func TestPrepare_PropagatesPatchError(t *testing.T) {
	r := req()
	r.Container = "nonexistent"
	if _, err := Prepare(r); err == nil {
		t.Error("want error when container absent (patch fails)")
	}
}
