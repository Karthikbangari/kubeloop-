package manifestsource

import (
	"testing"

	rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"
	"github.com/Karthikbangari/kubeloop-/internal/savings"
	"github.com/Karthikbangari/kubeloop-/internal/scan"
)

const deploy = `{
  "kind": "Deployment",
  "metadata": { "name": "checkout-api", "namespace": "shop" },
  "spec": {
    "replicas": 2,
    "template": { "spec": { "containers": [
      { "name": "app", "image": "eclipse-temurin:21-jre",
        "resources": { "requests": { "cpu": "2000m", "memory": "1Gi" } } }
    ] } }
  }
}`

func TestFromManifest_EndToEndIntoScan(t *testing.T) {
	in, err := FromManifest([]byte(deploy), rs.Usage{P95CPU: 410, P99CPU: 480, MaxMem: 512 << 20}, 30)
	if err != nil {
		t.Fatal(err)
	}
	// Current comes from the manifest; runtime detected from the JVM image.
	if in.Current.CPU != 2000 || in.Current.Mem != 1<<30 {
		t.Errorf("current = %+v, want 2000m / 1Gi from the manifest", in.Current)
	}
	if in.Meta.Runtime != "jvm" || in.Replicas != 2 {
		t.Errorf("meta/replicas wrong: %+v replicas=%d", in.Meta, in.Replicas)
	}

	// The whole offline path: manifest → scan.Input → ranked report with the
	// JVM caution surfaced.
	r := scan.Scan([]scan.Input{in}, rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)
	if len(r.Rows) != 1 || r.Rows[0].Name != "checkout-api" {
		t.Fatalf("scan rows = %+v, want checkout-api", r.Rows)
	}
	if r.Rows[0].Proposed.CPU != 576 { // P99×1.2
		t.Errorf("proposed CPU = %d, want 576", r.Rows[0].Proposed.CPU)
	}
	if r.Rows[0].Caution == "" {
		t.Error("JVM workload should carry a caution through the offline read path")
	}
}

func TestFromManifest_PropagatesParseError(t *testing.T) {
	if _, err := FromManifest([]byte(`{bad`), rs.Usage{}, 30); err == nil {
		t.Error("want parse error propagated")
	}
}
