package dirsource

import (
	"testing"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
	"github.com/kubeloop/kubeloop/internal/savings"
	"github.com/kubeloop/kubeloop/internal/scan"
)

func manifest(ns, name, cpu string) []byte {
	return []byte(`{"kind":"Deployment","metadata":{"name":"` + name + `","namespace":"` + ns +
		`"},"spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"app","resources":{"requests":{"cpu":"` + cpu + `","memory":"1Gi"}}}]}}}}`)
}

func TestAssemble_AttachesUsageAndScans(t *testing.T) {
	manifests := [][]byte{
		manifest("shop", "checkout-api", "2000m"),
		manifest("shop", "no-metrics", "2000m"), // no usage entry → excluded
	}
	usage := map[string]Usage{
		Key("shop", "checkout-api"): {Usage: rs.Usage{P95CPU: 410, P99CPU: 480, MaxMem: 512 << 20}, HistoryDays: 30},
	}

	inputs, err := Assemble(manifests, usage)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 {
		t.Fatalf("assembled %d inputs, want 2", len(inputs))
	}

	r := scan.Scan(inputs, rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)
	if len(r.Rows) != 1 || r.Rows[0].Name != "checkout-api" {
		t.Fatalf("ranked = %+v, want only checkout-api", r.Rows)
	}
	// The un-instrumented workload is excluded via missing-signal, not sized.
	if len(r.Excluded) != 1 || r.Excluded[0].Name != "no-metrics" {
		t.Fatalf("excluded = %+v, want no-metrics", r.Excluded)
	}
	if r.Excluded[0].Reason == "" {
		t.Error("excluded workload should carry a reason")
	}
}

func TestAssemble_MalformedManifestErrors(t *testing.T) {
	if _, err := Assemble([][]byte{[]byte("{not json")}, nil); err == nil {
		t.Error("want error on malformed manifest")
	}
}
