// Package savings labels *when* a dollar figure is actually realized, so
// kubeloop never implies node-based savings land the moment a PR merges. This
// is the bill-honesty guardrail from the plan in code, not just prose.
package savings

import "fmt"

// Mode is how the cluster is billed, which decides when rightsizing shows up on
// the bill. Detection (e.g. spotting GKE Autopilot) is the read-layer's job;
// this package only does the labeling.
type Mode int

const (
	// PerRequest bills per pod request (e.g. GKE Autopilot): a smaller request
	// cuts the bill immediately.
	PerRequest Mode = iota
	// NodeBased bills per node (EKS / GKE Standard / AKS): freeing requests
	// only saves money once the autoscaler consolidates nodes.
	NodeBased
)

// Realization is the clause describing when savings land. NodeBased is the safe
// default for any unknown mode — never over-promise immediacy.
func Realization(m Mode) string {
	if m == PerRequest {
		return "cut immediately (billed per pod request)"
	}
	return "realized when nodes consolidate (Cluster Autoscaler / Karpenter)"
}

// Headline states the waste total and, honestly, when it is realized.
func Headline(total float64, m Mode) string {
	return fmt.Sprintf("$%.2f/month — %s", total, Realization(m))
}
