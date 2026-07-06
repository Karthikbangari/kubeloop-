// Package safety decides which workloads to exclude from rightsizing and how
// much to trust a recommendation. Pure logic, no cluster access. One bad
// confident number can OOM-kill prod and end the project's credibility, so the
// cautious paths live here in code, not convention.
//
// Kind/Runtime matching is case-insensitive so casing from different sources
// (kube kinds, image hints) can't silently skip a rule. A future read-layer
// should still normalize at its boundary; this is defense in depth.
package safety

import (
	"fmt"
	"strings"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
)

// Meta is the workload metadata that governs exclusion and confidence,
// independent of the usage numbers. The read-layer slice will populate it.
type Meta struct {
	Kind        string // "Deployment","StatefulSet","CronJob","Job", ...
	HistoryDays int    // days of usage history available
	Runtime     string // optional hint: "jvm", "" if unknown
}

const (
	minHistoryDays  = 7  // below this we lack signal to size at all
	fullHistoryDays = 14 // below this we can size but with less confidence
)

// Exclusion is a decision to skip a workload, with a human-readable reason so
// the user sees why nothing was proposed rather than a silent gap.
type Exclusion struct {
	Excluded bool
	Reason   string
}

// Assess returns whether to exclude a workload. Batch workloads are bursty by
// design and short-history workloads lack signal — both would produce
// confident-looking nonsense, so we skip them and say why.
func Assess(m Meta) Exclusion {
	switch {
	case strings.EqualFold(m.Kind, "CronJob") || strings.EqualFold(m.Kind, "Job"):
		return Exclusion{true, "batch workload (" + m.Kind + ") — bursty by design, request-sizing doesn't apply"}
	case m.HistoryDays < minHistoryDays:
		return Exclusion{true, fmt.Sprintf("only %dd usage history (<%dd) — not enough signal", m.HistoryDays, minHistoryDays)}
	}
	return Exclusion{false, ""}
}

// Confidence grades how much to trust a recommendation.
type Confidence struct {
	Level   string // "high" | "med" | "low"
	Caution string // non-empty when a runtime needs a caveat, not a number
}

// Score grades confidence from history length and CPU burstiness, and flags
// memory-sensitive runtimes (JVM) where observed usage doesn't drive the real
// requirement — heap is configured, not measured.
func Score(m Meta, u rs.Usage) Confidence {
	level := "high"
	// Spiky CPU (P99 well above P95) means the tail dominates → less certain.
	// ponytail: 1.5× is a starting threshold, tune when real data says so.
	if u.P95CPU > 0 && u.P99CPU*2 > u.P95CPU*3 {
		level = "med"
	}
	if m.HistoryDays < fullHistoryDays {
		level = downgrade(level)
	}
	c := Confidence{Level: level}
	if strings.EqualFold(m.Runtime, "jvm") {
		c.Level = downgrade(c.Level) // never "high" for JVM memory
		c.Caution = "JVM: memory request is heap-configured, not usage-driven — treat the memory number as a caution"
	}
	return c
}

func downgrade(level string) string {
	if level == "high" {
		return "med"
	}
	return "low"
}
