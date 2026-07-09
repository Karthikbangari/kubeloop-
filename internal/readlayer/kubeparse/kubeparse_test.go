package kubeparse

import (
	"reflect"
	"testing"

	"github.com/Karthikbangari/kubeloop-/internal/inventory"
)

const deploy = `{
  "kind": "Deployment",
  "metadata": { "name": "checkout-api", "namespace": "shop" },
  "spec": {
    "replicas": 3,
    "template": {
      "spec": {
        "initContainers": [
          { "name": "migrate", "image": "busybox", "resources": { "requests": { "cpu": "100m", "memory": "64Mi" } } }
        ],
        "containers": [
          { "name": "app", "image": "eclipse-temurin:21-jre", "command": ["java","-jar","app.jar"],
            "resources": { "requests": { "cpu": "2000m", "memory": "512Mi" } } },
          { "name": "sidecar", "image": "envoy:1.30",
            "resources": { "requests": { "cpu": "250m", "memory": "128Mi" } } }
        ]
      }
    }
  }
}`

func TestParse_FullWorkload(t *testing.T) {
	w, err := Parse([]byte(deploy))
	if err != nil {
		t.Fatal(err)
	}
	if w.Kind != "Deployment" || w.Name != "checkout-api" || w.Namespace != "shop" || w.Replicas != 3 {
		t.Fatalf("identity wrong: %+v", w)
	}
	if len(w.Containers) != 2 || len(w.InitContainers) != 1 {
		t.Fatalf("containers: %d regular, %d init", len(w.Containers), len(w.InitContainers))
	}
	wantApp := inventory.Container{Image: "eclipse-temurin:21-jre", Command: []string{"java", "-jar", "app.jar"}, CPU: 2000, Mem: 512 << 20}
	if !reflect.DeepEqual(w.Containers[0], wantApp) {
		t.Errorf("app container = %+v, want %+v", w.Containers[0], wantApp)
	}
	// Feeds the inventory primitives: init CPU peak 100 < regular sum 2250, so
	// pod CPU = 2250; runtime detected as jvm from the app image/command.
	if got := inventory.PodRequest(w.Containers, w.InitContainers); got.CPU != 2250 {
		t.Errorf("pod CPU = %d, want 2250 (sum of regular)", got.CPU)
	}
	if inventory.DetectRuntime(w.Containers) != "jvm" {
		t.Error("expected jvm runtime from the app container")
	}
}

func TestParse_ReplicasDefaultsToOne(t *testing.T) {
	w, err := Parse([]byte(`{"kind":"StatefulSet","metadata":{"name":"pg"},"spec":{"template":{"spec":{"containers":[{"name":"db","resources":{"requests":{"cpu":"1","memory":"1Gi"}}}]}}}}`))
	if err != nil || w.Replicas != 1 {
		t.Errorf("replicas = %d err=%v, want 1 (k8s default)", w.Replicas, err)
	}
}

func TestParse_NegativeReplicasErrors(t *testing.T) {
	bad := `{"kind":"Deployment","metadata":{"name":"x"},"spec":{"replicas":-1,"template":{"spec":{"containers":[{"name":"app"}]}}}}`
	if _, err := Parse([]byte(bad)); err == nil {
		t.Error("want error on negative replicas")
	}
}

func TestParse_MalformedQuantityErrors(t *testing.T) {
	bad := `{"kind":"Deployment","metadata":{"name":"x"},"spec":{"template":{"spec":{"containers":[{"name":"app","resources":{"requests":{"cpu":"lots","memory":"1Gi"}}}]}}}}`
	if _, err := Parse([]byte(bad)); err == nil {
		t.Error("want error on malformed cpu quantity, not a silent 0")
	}
}

func TestParse_MissingRequestsIsZeroNotError(t *testing.T) {
	w, err := Parse([]byte(`{"kind":"Deployment","metadata":{"name":"x"},"spec":{"template":{"spec":{"containers":[{"name":"app","image":"nginx"}]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if w.Containers[0].CPU != 0 || w.Containers[0].Mem != 0 {
		t.Errorf("absent requests should parse to 0, got %+v", w.Containers[0])
	}
}
