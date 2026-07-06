package inventory

import (
	"testing"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
)

func TestDetectRuntime_JVMImageOrCommand(t *testing.T) {
	if got := DetectRuntime([]Container{{Image: "eclipse-temurin:21-jre"}}); got != "jvm" {
		t.Errorf("temurin image = %q, want jvm", got)
	}
	if got := DetectRuntime([]Container{{Image: "mycorp/app:1.2", Command: []string{"/usr/bin/java", "-jar", "app.jar"}}}); got != "jvm" {
		t.Errorf("java command = %q, want jvm", got)
	}
}

func TestDetectRuntime_AnyContainerCounts(t *testing.T) {
	if got := DetectRuntime([]Container{{Image: "nginx:1.27"}, {Image: "openjdk:21"}}); got != "jvm" {
		t.Errorf("mixed pod = %q, want jvm (any container counts)", got)
	}
}

func TestDetectRuntime_NonJVMNoFalsePositive(t *testing.T) {
	// node/javascript and python must not be mistaken for java.
	cs := []Container{{Image: "node:20", Command: []string{"node", "server.js"}}, {Image: "python:3.12"}}
	if got := DetectRuntime(cs); got != "" {
		t.Errorf("non-jvm pod = %q, want empty (no false caution)", got)
	}
}

func TestPodRequest_SumsRegularContainers(t *testing.T) {
	got := PodRequest([]Container{{CPU: 200, Mem: 100}, {CPU: 300, Mem: 150}}, nil)
	if got != (rs.Resources{CPU: 500, Mem: 250}) {
		t.Errorf("got %+v, want CPU 500 / Mem 250 (sum of regular)", got)
	}
}

func TestPodRequest_InitPeakWinsWhenLarger(t *testing.T) {
	// Regular sum CPU=500; an init container needs 800 → pod reserves 800.
	got := PodRequest([]Container{{CPU: 200}, {CPU: 300}}, []Container{{CPU: 800}, {CPU: 100}})
	if got.CPU != 800 {
		t.Errorf("CPU = %d, want 800 (max init > sum regular)", got.CPU)
	}
}

func TestPodRequest_RegularSumWinsWhenLarger(t *testing.T) {
	got := PodRequest([]Container{{CPU: 600}}, []Container{{CPU: 100}})
	if got.CPU != 600 {
		t.Errorf("CPU = %d, want 600 (sum regular > max init)", got.CPU)
	}
}

func TestPodRequest_PerResourceIndependent(t *testing.T) {
	// Init wins CPU (max 900 > sum 400); regular wins memory (sum 1000 > init 300).
	got := PodRequest(
		[]Container{{CPU: 400, Mem: 700}, {CPU: 0, Mem: 300}},
		[]Container{{CPU: 900, Mem: 300}},
	)
	if got.CPU != 900 || got.Mem != 1000 {
		t.Errorf("got %+v, want CPU 900 / Mem 1000 (resolved per resource)", got)
	}
}

func TestPodRequest_Empty(t *testing.T) {
	if got := PodRequest(nil, nil); got != (rs.Resources{}) {
		t.Errorf("empty = %+v, want zero", got)
	}
}
