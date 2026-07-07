package pr

import (
	"strings"
	"testing"
)

const singleContainerDoc = `kind: Deployment
metadata:
  name: app
  namespace: shop
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 2000m
`

const multiContainerDoc = `kind: Deployment
metadata:
  name: app
  namespace: shop
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 2000m
        - name: sidecar
          resources:
            requests:
              cpu: 500m
`

func TestContainerCount(t *testing.T) {
	if n, _ := ContainerCount([]byte(singleContainerDoc)); n != 1 {
		t.Errorf("single = %d, want 1", n)
	}
	if n, _ := ContainerCount([]byte(multiContainerDoc)); n != 2 {
		t.Errorf("multi = %d, want 2", n)
	}
}

func TestRequireSingleContainer(t *testing.T) {
	if err := RequireSingleContainer([]byte(singleContainerDoc)); err != nil {
		t.Errorf("single should pass, got %v", err)
	}
	if err := RequireSingleContainer([]byte(multiContainerDoc)); err == nil || !strings.Contains(err.Error(), "2 containers") {
		t.Errorf("multi should be refused citing 2 containers, got %v", err)
	}
}

// Prepare must inherit the refusal so every PR caller is protected.
func TestPrepare_RefusesMultiContainer(t *testing.T) {
	_, err := Prepare(Request{
		Files:       []File{{Path: "deploy.yaml", Content: []byte(multiContainerDoc)}},
		Ref:         Ref{Kind: "Deployment", Name: "app", Namespace: "shop"},
		Container:   "app",
		CurrentCPU:  "2000m",
		ProposedCPU: "576m",
	})
	if err == nil || !strings.Contains(err.Error(), "containers") {
		t.Errorf("Prepare should refuse multi-container, got %v", err)
	}
}
