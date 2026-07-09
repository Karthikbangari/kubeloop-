// Package inventory maps Kubernetes workload objects to what the scanner needs:
// the resource request a pod actually reserves, and a runtime hint. Quantity-
// string parsing (k8s Quantity like "512Mi") is deliberately NOT here — it
// belongs at the apimachinery boundary in the live read-layer. This package
// takes already-numeric requests so it stays dep-free and testable.
package inventory

import (
	"strings"

	rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"
)

// Container is one container as the read-layer sees it: identity (image/command,
// for runtime detection) and requests (CPU millicores, memory bytes). A zero
// CPU/Mem means no request set for that resource.
type Container struct {
	Image   string
	Command []string
	CPU     int64
	Mem     int64
}

// PodRequest returns the resources a pod reserves, per the Kubernetes rule: for
// each resource independently, max(sum of regular containers, max of init
// containers). Init containers run sequentially so their peak is the max, not
// the sum; the pod reserves whichever is larger. Native sidecars are out of
// scope until the read-layer distinguishes them.
func PodRequest(regular, init []Container) rs.Resources {
	var sumCPU, sumMem, maxInitCPU, maxInitMem int64
	for _, c := range regular {
		sumCPU += c.CPU
		sumMem += c.Mem
	}
	for _, c := range init {
		maxInitCPU = max64(maxInitCPU, c.CPU)
		maxInitMem = max64(maxInitMem, c.Mem)
	}
	return rs.Resources{CPU: max64(sumCPU, maxInitCPU), Mem: max64(sumMem, maxInitMem)}
}

// jvmImageMarkers are substrings of common JVM base images (lowercased).
// ponytail: curated list, extend when a real workload slips through.
var jvmImageMarkers = []string{
	"openjdk", "eclipse-temurin", "amazoncorretto", "adoptopenjdk",
	"ibm-semeru", "azul/zulu", "-jdk", "-jre", "/jdk", "/jre",
}

// DetectRuntime returns "jvm" if any container looks JVM-based, else "". A pod
// is treated as JVM if any container is, since the memory caution applies to
// the whole recommendation. A miss just omits a caution — never a wrong number.
func DetectRuntime(cs []Container) string {
	for _, c := range cs {
		if isJVM(c) {
			return "jvm"
		}
	}
	return ""
}

func isJVM(c Container) bool {
	img := strings.ToLower(c.Image)
	for _, m := range jvmImageMarkers {
		if strings.Contains(img, m) {
			return true
		}
	}
	for _, tok := range c.Command {
		t := strings.ToLower(tok)
		if t == "java" || strings.HasSuffix(t, "/java") {
			return true
		}
	}
	return false
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
