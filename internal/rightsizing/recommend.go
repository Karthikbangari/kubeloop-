// Package rightsizing turns observed usage into proposed resource requests and
// dollar-ranked waste. Pure functions, no cluster access — the safety-critical
// core, testable against hand math. See plan/MASTER-PLAN-LOOPED.md safety floors.
package rightsizing

// Usage is the observed usage of one container over the lookback window.
// CPU in millicores, memory in bytes.
type Usage struct {
	P95CPU, P99CPU int64 // millicores
	MaxMem         int64 // bytes
}

// Resources is a request pair. CPU millicores, memory bytes.
type Resources struct {
	CPU int64 // millicores
	Mem int64 // bytes
}

// memBuffer is the absolute memory cushion added on top of observed max, so a
// small workload still gets headroom a flat percentage wouldn't give.
// ponytail: 128Mi constant, make it configurable when a real workload needs it.
const memBuffer = 128 * 1024 * 1024

// Recommender maps observed usage to proposed requests. An interface so the
// engine is swappable (own PromQL now, KRR-ingest later) per the plan.
type Recommender interface {
	Recommend(Usage) Resources
}

// Percentile is the default engine: CPU≈P95, mem=max+15%, with safety floors
// applied so a recommendation can never drop below a proven-needed level.
type Percentile struct{}

func (Percentile) Recommend(u Usage) Resources {
	// CPU: floor at P99×1.2 governs for real data (P99 ≥ P95 always, so the
	// floor is ≥ 1.2×P95 > P95). P95 is the fallback that keeps us safe when
	// P99 is missing/degenerate (e.g. a Prometheus gap yields P99=0).
	// ponytail: not a headroom knob — the floor is the CPU policy; P95 is defense.
	cpu := max64(u.P95CPU, u.P99CPU*12/10)
	// Mem: base at max+15%, floored at max+buffer for an absolute cushion.
	mem := max64(u.MaxMem*115/100, u.MaxMem+memBuffer)
	return Resources{CPU: cpu, Mem: mem}
}

// hoursPerMonth is the standard 730h/month used for list-price waste math.
const hoursPerMonth = 730

// Price is the per-unit-per-hour list price. CPU per millicore-hour, mem per
// byte-hour. Directional (list prices) — the ranking is the point, not the cent.
type Price struct {
	PerMilliCPUHour float64
	PerByteMemHour  float64
}

// MonthlyWaste returns the dollars/month freed by moving from current to
// proposed requests across replicas. Never negative: if proposed ≥ current on a
// dimension (a floor pushed it up), that dimension contributes zero, not a
// negative "saving".
func MonthlyWaste(current, proposed Resources, replicas int, p Price) float64 {
	cpuFreed := float64(max64(current.CPU-proposed.CPU, 0))
	memFreed := float64(max64(current.Mem-proposed.Mem, 0))
	perReplica := cpuFreed*p.PerMilliCPUHour + memFreed*p.PerByteMemHour
	return perReplica * float64(replicas) * hoursPerMonth
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
