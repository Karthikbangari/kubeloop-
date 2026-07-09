package reporting

import (
	"strings"
	"testing"

	rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"
)

func approx(a, b float64) bool { return a-b < 0.01 && b-a < 0.01 }

// Two workloads, CPU-only clean price so the math is hand-checkable.
//
//	checkout-api:    2000→576 (P99×1.2), freed 1424 ×$0.0001×1×730 = $103.952
//	recommendations: 4000→1080,          freed 2920 ×$0.0001×2×730 = $426.320
func fixtures() []Workload {
	return []Workload{
		{Name: "checkout-api", Replicas: 1, Current: rs.Resources{CPU: 2000}, Usage: rs.Usage{P95CPU: 410, P99CPU: 480}},
		{Name: "recommendations", Replicas: 2, Current: rs.Resources{CPU: 4000}, Usage: rs.Usage{P95CPU: 800, P99CPU: 900}},
	}
}

func TestRank_SortsByWasteAndTotals(t *testing.T) {
	rows, total := Rank(fixtures(), rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001})
	if rows[0].Name != "recommendations" || rows[1].Name != "checkout-api" {
		t.Fatalf("order = [%s, %s], want [recommendations, checkout-api]", rows[0].Name, rows[1].Name)
	}
	if rows[0].Proposed.CPU != 1080 {
		t.Errorf("top proposed CPU = %d, want 1080", rows[0].Proposed.CPU)
	}
	if !approx(rows[0].MonthlyWaste, 426.32) {
		t.Errorf("top waste = %.4f, want 426.32", rows[0].MonthlyWaste)
	}
	if !approx(total, 530.272) {
		t.Errorf("total = %.4f, want 530.272", total)
	}
}

func TestDefaultPrice_DerivationAndFallback(t *testing.T) {
	aws := DefaultPrice("aws")
	if aws.PerMilliCPUHour != 0.031/1000 || aws.PerByteMemHour != 0.0042/gib {
		t.Errorf("aws price = %v, want derived from 0.031/vCPU-h, 0.0042/GB-h", aws)
	}
	if DefaultPrice("nope") != aws {
		t.Error("unknown cloud should fall back to aws prices")
	}
}

func TestRender_HeadlineAndOrder(t *testing.T) {
	rows, total := Rank(fixtures(), rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001})
	out := Render(rows, total)
	if !strings.Contains(out, "$/MONTH") || !strings.Contains(out, "$530.27") {
		t.Errorf("render missing header or total:\n%s", out)
	}
	if strings.Index(out, "recommendations") > strings.Index(out, "checkout-api") {
		t.Error("render should list higher-waste workload first")
	}
}

func TestRender_ConfColumnAndCautionOnlyWhenSet(t *testing.T) {
	// No confidence set → no CONF column, no caution lines.
	plain := Render([]Row{{Workload: Workload{Name: "a"}}}, 0)
	if strings.Contains(plain, "CONF") || strings.Contains(plain, "! ") {
		t.Errorf("blank confidence should hide CONF/cautions:\n%s", plain)
	}
	// Confidence + caution set → column appears and caution prints under table.
	rich := Render([]Row{{Workload: Workload{Name: "a"}, Confidence: "high", Caution: "watch heap"}}, 0)
	if !strings.Contains(rich, "CONF") || !strings.Contains(rich, "high") || !strings.Contains(rich, "! a: watch heap") {
		t.Errorf("confidence set should show CONF + caution:\n%s", rich)
	}
}

func TestRender_QualifiesNamespaceOnCollision(t *testing.T) {
	// Same name "api" in two namespaces must be disambiguated; the unique
	// "web" stays bare.
	rows := []Row{
		{Workload: Workload{Namespace: "team-a", Name: "api"}},
		{Workload: Workload{Namespace: "team-b", Name: "api"}},
		{Workload: Workload{Namespace: "team-a", Name: "web"}},
	}
	out := Render(rows, 0)
	if !strings.Contains(out, "team-a/api") || !strings.Contains(out, "team-b/api") {
		t.Errorf("collision not disambiguated:\n%s", out)
	}
	if strings.Contains(out, "team-a/web") {
		t.Errorf("unique name should stay bare:\n%s", out)
	}
}
