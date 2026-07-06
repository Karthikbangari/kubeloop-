package rightsizing

import "testing"

const Mi = 1024 * 1024

func TestRecommend_NormalWorkload(t *testing.T) {
	// Realistic percentiles (P99 ≥ P95). Floor governs CPU: 480×1.2=576.
	got := Percentile{}.Recommend(Usage{P95CPU: 410, P99CPU: 480, MaxMem: 900 * Mi})
	if got.CPU != 576 {
		t.Errorf("CPU = %d, want 576 (P99×1.2 floor governs)", got.CPU)
	}
	// Mem: max(900Mi×1.15=1035Mi, 900Mi+128Mi=1028Mi) → 1035Mi.
	if got.Mem != 1035*Mi {
		t.Errorf("Mem = %d, want %d (max+15%%)", got.Mem, 1035*Mi)
	}
}

func TestRecommend_SmallWorkload(t *testing.T) {
	// Realistic percentiles. Floor governs CPU: 250×1.2=300.
	got := Percentile{}.Recommend(Usage{P95CPU: 200, P99CPU: 250, MaxMem: 100 * Mi})
	if got.CPU != 300 {
		t.Errorf("CPU = %d, want 300 (P99×1.2 floor governs)", got.CPU)
	}
	// Small workload: max(100Mi×1.15=115Mi, 100Mi+128Mi=228Mi) → buffer wins.
	if got.Mem != 228*Mi {
		t.Errorf("Mem = %d, want %d (max+buffer)", got.Mem, 228*Mi)
	}
}

func TestRecommend_P99MissingFallsBackToP95(t *testing.T) {
	// Degraded metrics: P99 query returned nothing → 0. P95 keeps us safe
	// instead of proposing a near-zero CPU request. This is the only path
	// where the P95 term wins, and it is a fallback, not normal operation.
	got := Percentile{}.Recommend(Usage{P95CPU: 410, P99CPU: 0, MaxMem: 900 * Mi})
	if got.CPU != 410 {
		t.Errorf("CPU = %d, want 410 (P95 fallback when P99 missing)", got.CPU)
	}
}

func TestMonthlyWaste_CPU(t *testing.T) {
	// freed 1500m × $0.0001/m-h × 1 replica × 730h = $109.50.
	got := MonthlyWaste(Resources{CPU: 2000}, Resources{CPU: 500}, 1, Price{PerMilliCPUHour: 0.0001})
	if got < 109.49 || got > 109.51 {
		t.Errorf("waste = %.4f, want 109.50", got)
	}
}

func TestMonthlyWaste_NeverNegative(t *testing.T) {
	// Floor pushed proposed (576) above current (500): no negative "saving".
	got := MonthlyWaste(Resources{CPU: 500}, Resources{CPU: 576}, 3, Price{PerMilliCPUHour: 0.0001})
	if got != 0 {
		t.Errorf("waste = %.4f, want 0", got)
	}
}
