package guards

import (
	"strings"
	"testing"
)

const single = `kind: Deployment
metadata:
  name: app
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 2000m
`

const multi = `kind: Deployment
metadata:
  name: app
spec:
  template:
    spec:
      containers:
        - name: app
        - name: sidecar
`

func TestContainerCount(t *testing.T) {
	if n, _ := ContainerCount([]byte(single)); n != 1 {
		t.Errorf("single = %d, want 1", n)
	}
	if n, _ := ContainerCount([]byte(multi)); n != 2 {
		t.Errorf("multi = %d, want 2", n)
	}
}

func TestRequireSingleContainer(t *testing.T) {
	if err := RequireSingleContainer([]byte(single)); err != nil {
		t.Errorf("single container should pass, got %v", err)
	}
	err := RequireSingleContainer([]byte(multi))
	if err == nil || !strings.Contains(err.Error(), "2 containers") {
		t.Errorf("multi should be refused citing 2 containers, got %v", err)
	}
}

func TestContainerCount_MalformedErrors(t *testing.T) {
	if _, err := ContainerCount([]byte("\tnot: [valid")); err == nil {
		t.Error("want parse error on malformed manifest")
	}
}
