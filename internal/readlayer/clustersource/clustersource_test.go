package clustersource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Karthikbangari/kubeloop-/internal/inventory"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/kubeparse"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/promclient"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/promql"
	rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"
	"github.com/Karthikbangari/kubeloop-/internal/savings"
	"github.com/Karthikbangari/kubeloop-/internal/scan"
)

// promclient is the real Querier this package is built for.
var _ Querier = (*promclient.Client)(nil)

func workload(kind, ns, name string, cpuMilli, memBytes int64) kubeparse.Workload {
	return kubeparse.Workload{
		Kind: kind, Namespace: ns, Name: name, Replicas: 1,
		Containers: []inventory.Container{{Image: "nginx", CPU: cpuMilli, Mem: memBytes}},
	}
}

// fakeQuerier answers by matching a distinguishing substring of each query.
type fakeQuerier struct {
	answers map[string]float64 // substring → value
	missing bool               // every query returns no data
	err     error
	queries []string
}

func (f *fakeQuerier) Query(_ context.Context, q string) (float64, bool, error) {
	f.queries = append(f.queries, q)
	if f.err != nil {
		return 0, false, f.err
	}
	if f.missing {
		return 0, false, nil
	}
	for sub, v := range f.answers {
		if strings.Contains(q, sub) {
			return v, true, nil
		}
	}
	return 0, false, nil
}

func TestCollect_AssemblesUsageAndScans(t *testing.T) {
	q := &fakeQuerier{answers: map[string]float64{
		"quantile_over_time(0.95": 0.41,      // cores
		"quantile_over_time(0.99": 0.48,      // cores
		"working_set_bytes":       300 << 20, // bytes
		"count_over_time":         30,        // days
	}}
	ws := []kubeparse.Workload{workload("Deployment", "shop", "checkout-api", 2000, 512<<20)}

	inputs, err := Collect(context.Background(), ws, q, promql.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 {
		t.Fatalf("got %d inputs, want 1", len(inputs))
	}
	// Cores → millicores, and the history probe feeds safety's <7d rule.
	if got := inputs[0].Usage; got.P95CPU != 410 || got.P99CPU != 480 || got.MaxMem != 300<<20 {
		t.Errorf("usage = %+v, want 410/480 millicores + 300Mi", got)
	}
	if inputs[0].Meta.HistoryDays != 30 {
		t.Errorf("historyDays = %d, want 30", inputs[0].Meta.HistoryDays)
	}
	// Current requests come from the cluster object, not Prometheus.
	if inputs[0].Current.CPU != 2000 {
		t.Errorf("current CPU = %d, want 2000 from the workload spec", inputs[0].Current.CPU)
	}

	r := scan.Scan(inputs, rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)
	if len(r.Rows) != 1 || r.Rows[0].Proposed.CPU != 576 { // P99×1.2
		t.Fatalf("scan = %+v, want checkout-api proposed 576m", r.Rows)
	}
}

// A workload Prometheus has never seen is reported as excluded, never sized.
func TestCollect_NoDataYieldsExclusionNotSizing(t *testing.T) {
	q := &fakeQuerier{missing: true}
	ws := []kubeparse.Workload{workload("Deployment", "shop", "ghost", 2000, 512<<20)}

	inputs, err := Collect(context.Background(), ws, q, promql.Defaults())
	if err != nil {
		t.Fatalf("no data is not an error: %v", err)
	}
	if inputs[0].Usage != (rs.Usage{}) {
		t.Errorf("usage = %+v, want zero", inputs[0].Usage)
	}
	r := scan.Scan(inputs, rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)
	if len(r.Rows) != 0 || len(r.Excluded) != 1 || r.Excluded[0].Reason == "" {
		t.Fatalf("want 1 excluded-with-reason and 0 ranked, got rows=%d excluded=%+v", len(r.Rows), r.Excluded)
	}
}

// A broken Prometheus must abort, not masquerade as "this cluster has no usage"
// — which would print $0 waste and read as "nothing to save".
func TestCollect_QueryErrorAborts(t *testing.T) {
	q := &fakeQuerier{err: errors.New("connection refused")}
	ws := []kubeparse.Workload{workload("Deployment", "shop", "checkout-api", 2000, 512<<20)}

	_, err := Collect(context.Background(), ws, q, promql.Defaults())
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("want the Prometheus failure surfaced, got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "checkout-api") {
		t.Errorf("error should name the workload: %v", err)
	}
}

func TestCollect_UnknownKindErrors(t *testing.T) {
	q := &fakeQuerier{}
	ws := []kubeparse.Workload{workload("DaemonSet", "shop", "agent", 100, 1<<20)}
	if _, err := Collect(context.Background(), ws, q, promql.Defaults()); err == nil {
		t.Error("want error when promql has no pod-naming rule for the listed kind")
	}
}

func TestCollect_QueriesAreNamespaceScopedToTheWorkload(t *testing.T) {
	q := &fakeQuerier{answers: map[string]float64{}}
	ws := []kubeparse.Workload{workload("StatefulSet", "data", "pg", 1000, 1<<30)}
	if _, err := Collect(context.Background(), ws, q, promql.Defaults()); err != nil {
		t.Fatal(err)
	}
	if len(q.queries) != 4 {
		t.Fatalf("want 4 queries (p95, p99, mem, history), got %d", len(q.queries))
	}
	for _, got := range q.queries {
		if !strings.Contains(got, `namespace="data"`) || !strings.Contains(got, `pod=~"pg-[0-9]+"`) {
			t.Errorf("query not scoped to the workload: %s", got)
		}
	}
}

// End to end over HTTP with the real promclient: promql → promclient →
// promusage → clustersource → scan. Only the cluster and Prometheus are faked.
func TestCollect_EndToEndWithRealPromClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		val := "0"
		switch {
		case strings.Contains(query, "quantile_over_time(0.95"):
			val = "0.41"
		case strings.Contains(query, "quantile_over_time(0.99"):
			val = "0.48"
		case strings.Contains(query, "working_set_bytes"):
			val = "314572800"
		case strings.Contains(query, "count_over_time"):
			val = "30"
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"value":[1720200000,%q]}]}}`, val)
	}))
	defer srv.Close()

	ws := []kubeparse.Workload{workload("Deployment", "shop", "checkout-api", 2000, 512<<20)}
	inputs, err := Collect(context.Background(), ws, promclient.New(srv.URL, srv.Client()), promql.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	r := scan.Scan(inputs, rs.Percentile{}, rs.Price{PerMilliCPUHour: 0.0001}, savings.NodeBased)
	if len(r.Rows) != 1 || r.Rows[0].Name != "checkout-api" {
		t.Fatalf("ranked = %+v, want checkout-api", r.Rows)
	}
	if r.Rows[0].Proposed.CPU != 576 {
		t.Errorf("proposed CPU = %d, want 576 (P99 0.48 cores ×1.2)", r.Rows[0].Proposed.CPU)
	}
	if r.Total <= 0 {
		t.Errorf("want positive waste, got %v", r.Total)
	}
}
