package scan

import (
	"strings"
	"testing"

	rp "github.com/kubeloop/kubeloop/internal/reporting"
	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
	sf "github.com/kubeloop/kubeloop/internal/safety"
	"github.com/kubeloop/kubeloop/internal/savings"
)

func approx(a, b float64) bool { return a-b < 0.01 && b-a < 0.01 }

// Two rankable deployments plus a CronJob and a too-new deployment that must be
// excluded. CPU-only clean price so the math is hand-checkable:
//   recommendations: freed 2920 ×$0.0001×2×730 = $426.32  (ranked first)
//   checkout-api:    freed 1424 ×$0.0001×1×730 = $103.95
func fixtures() []Input {
	dep := func(days int) sf.Meta { return sf.Meta{Kind: "Deployment", HistoryDays: days} }
	return []Input{
		{Workload: rp.Workload{Namespace: "shop", Name: "checkout-api", Replicas: 1, Current: rs.Resources{CPU: 2000}, Usage: rs.Usage{P95CPU: 410, P99CPU: 480}}, Meta: dep(30)},
		{Workload: rp.Workload{Namespace: "shop", Name: "recommendations", Replicas: 2, Current: rs.Resources{CPU: 4000}, Usage: rs.Usage{P95CPU: 800, P99CPU: 900}}, Meta: dep(30)},
		{Workload: rp.Workload{Namespace: "batch", Name: "nightly"}, Meta: sf.Meta{Kind: "CronJob", HistoryDays: 30}},
		{Workload: rp.Workload{Namespace: "shop", Name: "new-svc"}, Meta: dep(3)},
	}
}

func TestScan_ExcludesRanksAndScores(t *testing.T) {
	r := Scan(fixtures(), rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)

	if len(r.Excluded) != 2 {
		t.Fatalf("excluded %d, want 2 (CronJob + <7d)", len(r.Excluded))
	}
	reasons := r.Excluded[0].Reason + " " + r.Excluded[1].Reason
	if !strings.Contains(reasons, "batch") || !strings.Contains(reasons, "3d") {
		t.Errorf("exclusion reasons = %q, want batch + 3d", reasons)
	}
	if len(r.Rows) != 2 || r.Rows[0].Name != "recommendations" {
		t.Fatalf("kept/ordered wrong: %+v", r.Rows)
	}
	if !approx(r.Total, 530.272) {
		t.Errorf("total = %.4f, want 530.272", r.Total)
	}
	if r.Rows[0].Confidence != "high" || r.Rows[1].Confidence != "high" {
		t.Errorf("conf = [%s,%s], want [high,high]", r.Rows[0].Confidence, r.Rows[1].Confidence)
	}
}

func TestRender_ExcludedNamesDisambiguateOnCollision(t *testing.T) {
	// Same excluded name "batch-job" in two namespaces must be namespace-qualified.
	in := []Input{
		{Workload: rp.Workload{Namespace: "team-a", Name: "batch-job"}, Meta: sf.Meta{Kind: "CronJob", HistoryDays: 30}},
		{Workload: rp.Workload{Namespace: "team-b", Name: "batch-job"}, Meta: sf.Meta{Kind: "CronJob", HistoryDays: 30}},
	}
	out := Render(Scan(in, rs.Percentile{}, rs.Price{}, savings.NodeBased))
	if !strings.Contains(out, "team-a/batch-job") || !strings.Contains(out, "team-b/batch-job") {
		t.Errorf("excluded collision not disambiguated:\n%s", out)
	}
}

func TestScan_JVMGetsCaution(t *testing.T) {
	in := []Input{{
		Workload: rp.Workload{Namespace: "shop", Name: "search", Replicas: 1, Current: rs.Resources{CPU: 1000, Mem: 2 * 1024 * 1024 * 1024}, Usage: rs.Usage{P95CPU: 300, P99CPU: 350, MaxMem: 500 * 1024 * 1024}},
		Meta:     sf.Meta{Kind: "Deployment", HistoryDays: 30, Runtime: "jvm"},
	}}
	r := Scan(in, rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)
	if r.Rows[0].Confidence == "high" || !strings.Contains(r.Rows[0].Caution, "JVM") {
		t.Errorf("jvm row = %+v, want <high with JVM caution", r.Rows[0])
	}
}

func TestRender_HasColumnsCautionsAndExclusions(t *testing.T) {
	r := Scan(fixtures(), rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)
	out := Render(r)
	for _, want := range []string{"CONF", "$530.27", "consolidate", "Excluded:", "batch", "high"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "recommendations") > strings.Index(out, "checkout-api") {
		t.Error("render should list higher-waste workload first")
	}
}
