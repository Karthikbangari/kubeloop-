package readlayer

import (
	"testing"

	"github.com/Karthikbangari/kubeloop-/internal/inventory"
	rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"
	"github.com/Karthikbangari/kubeloop-/internal/savings"
	"github.com/Karthikbangari/kubeloop-/internal/scan"
)

func TestToScanInputs_AssemblesRequestsRuntimeAndUsage(t *testing.T) {
	in := []Workload{{
		Namespace:   "shop",
		Name:        "checkout",
		Kind:        "Deployment",
		Replicas:    3,
		HistoryDays: 21,
		Containers: []inventory.Container{
			{Image: "nginx:1.27", CPU: 200, Mem: 256 * 1024 * 1024},
			{Image: "eclipse-temurin:21-jre", CPU: 300, Mem: 512 * 1024 * 1024},
		},
		InitContainers: []inventory.Container{{CPU: 900, Mem: 128 * 1024 * 1024}},
		Usage:          rs.Usage{P95CPU: 410, P99CPU: 480, MaxMem: 600 * 1024 * 1024},
	}}

	got := ToScanInputs(in)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	w := got[0].Workload
	if w.Namespace != "shop" || w.Name != "checkout" || w.Replicas != 3 {
		t.Fatalf("identity = %+v, want shop/checkout replicas=3", w)
	}
	if w.Current.CPU != 900 {
		t.Errorf("CPU request = %d, want 900 (init peak wins over regular sum)", w.Current.CPU)
	}
	if w.Current.Mem != 768*1024*1024 {
		t.Errorf("mem request = %d, want 768Mi (regular sum wins over init)", w.Current.Mem)
	}
	if w.Usage != in[0].Usage {
		t.Errorf("usage = %+v, want %+v", w.Usage, in[0].Usage)
	}
	if got[0].Meta.Kind != "Deployment" || got[0].Meta.HistoryDays != 21 || got[0].Meta.Runtime != "jvm" {
		t.Errorf("meta = %+v, want Deployment/21/jvm", got[0].Meta)
	}
}

func TestToScanInputs_FeedsScanExclusionAndConfidence(t *testing.T) {
	inputs := ToScanInputs([]Workload{
		{
			Namespace:   "shop",
			Name:        "api",
			Kind:        "Deployment",
			Replicas:    1,
			HistoryDays: 30,
			Containers:  []inventory.Container{{CPU: 2000, Mem: 1024 * 1024 * 1024}},
			Usage:       rs.Usage{P95CPU: 300, P99CPU: 350, MaxMem: 256 * 1024 * 1024},
		},
		{
			Namespace:   "batch",
			Name:        "nightly",
			Kind:        "CronJob",
			HistoryDays: 30,
			Containers:  []inventory.Container{{CPU: 500}},
		},
	})

	report := scan.Scan(inputs, rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)
	if len(report.Rows) != 1 || report.Rows[0].Name != "api" {
		t.Fatalf("ranked rows = %+v, want only api", report.Rows)
	}
	if report.Rows[0].Confidence != "high" {
		t.Errorf("confidence = %q, want high", report.Rows[0].Confidence)
	}
	if len(report.Excluded) != 1 || report.Excluded[0].Name != "nightly" {
		t.Fatalf("excluded = %+v, want nightly", report.Excluded)
	}
	if report.Total <= 0 {
		t.Errorf("total = %.2f, want positive waste", report.Total)
	}
}
