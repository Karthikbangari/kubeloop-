package pr

import (
	"fmt"
	"strings"
)

// Change describes one workload's proposed rightsizing. CPU/Mem are Kubernetes
// quantity strings; unchanged or empty resource pairs are omitted from the PR
// body.
type Change struct {
	Namespace, Name, Container string
	CurrentCPU, ProposedCPU    string
	CurrentMem, ProposedMem    string
	MonthlyUSD                 float64
	Confidence                 string
	Realization                string // e.g. savings.Realization(mode)
}

// Title is the PR title — the dollars lead, because that's what gets a review
// prioritized and merged.
func Title(c Change) string {
	return fmt.Sprintf("Right-size %s: save ~$%.0f/month", c.Name, c.MonthlyUSD)
}

// Body is the PR description (Markdown): savings + realization, a before/after
// table, confidence, the evidence behind the numbers, and a rollback note.
func Body(c Change) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Right-size `%s` (container `%s`)\n\n", c.Name, c.Container)
	fmt.Fprintf(&b, "Estimated savings: **$%.2f/month** — %s\n\n", c.MonthlyUSD, c.Realization)

	b.WriteString("| Resource | Current | Proposed |\n|---|---|---|\n")
	if changed(c.CurrentCPU, c.ProposedCPU) {
		fmt.Fprintf(&b, "| CPU | %s | %s |\n", c.CurrentCPU, c.ProposedCPU)
	}
	if changed(c.CurrentMem, c.ProposedMem) {
		fmt.Fprintf(&b, "| Memory | %s | %s |\n", c.CurrentMem, c.ProposedMem)
	}

	if c.Confidence != "" {
		fmt.Fprintf(&b, "\nConfidence: **%s**.\n", c.Confidence)
	}
	b.WriteString("\n**Evidence:** proposed from observed usage with safety floors in code " +
		"(CPU ≥ P99×1.2, memory ≥ max observed + buffer). Dollar figure uses directional " +
		"list prices — the ranking is the point, not the cent.\n")
	b.WriteString("\n**Rollback:** revert this commit, or set the requests back to " + rollbackValues(c) + ".\n")
	b.WriteString("\n_kubeloop is read-only; it prepared this PR for you to review and merge. " +
		"Nothing was applied to the cluster._\n")
	return b.String()
}

func changed(current, proposed string) bool {
	return current != "" && proposed != "" && current != proposed
}

func rollbackValues(c Change) string {
	var parts []string
	if changed(c.CurrentCPU, c.ProposedCPU) {
		parts = append(parts, "CPU `"+c.CurrentCPU+"`")
	}
	if changed(c.CurrentMem, c.ProposedMem) {
		parts = append(parts, "memory `"+c.CurrentMem+"`")
	}
	return strings.Join(parts, " / ")
}
