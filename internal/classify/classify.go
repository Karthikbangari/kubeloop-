// Package classify labels a workload's proposal relative to its current request
// so the report never presents an under-provisioning *increase* as if it were
// waste. The tool's point is waste (a reduction, real $ saved); an increase
// means the workload needs MORE (a risk worth flagging, not ranking as savings);
// equal means it's already right-sized. Keeping these apart stops a "save money"
// report from listing a scary "raise this to 2880m" line at $0.
package classify

import rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"

// Class is how a proposal relates to the current request.
type Class int

const (
	// Waste: at least one resource reduces → there is money to save. (A
	// simultaneous increase on the other resource is handled by reduce-only in
	// the PR path; there is still a saving to capture, so it ranks as waste.)
	Waste Class = iota
	// UnderProvisioned: nothing reduces and at least one resource's proposal
	// exceeds the current request — the workload needs more, not less.
	UnderProvisioned
	// RightSized: the proposal equals the current request on every resource.
	RightSized
)

func (c Class) String() string {
	switch c {
	case Waste:
		return "waste"
	case UnderProvisioned:
		return "under-provisioned"
	default:
		return "right-sized"
	}
}

// Classify compares proposed against current.
func Classify(current, proposed rs.Resources) Class {
	reduces := proposed.CPU < current.CPU || proposed.Mem < current.Mem
	increases := proposed.CPU > current.CPU || proposed.Mem > current.Mem
	switch {
	case reduces:
		return Waste
	case increases:
		return UnderProvisioned
	default:
		return RightSized
	}
}
