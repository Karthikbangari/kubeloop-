// Package clustersource is the live read path: workloads listed from a cluster
// (kubeclient) + usage measured in Prometheus (promql/promclient) → scan.Input.
// It is the live twin of dirsource, and lands on the same
// readlayer.ToScanInput assembly via manifestsource.FromWorkload.
//
// ⚠ The query strings it issues are NOT yet validated against a live Prometheus
// (see the promql package). The composition below is proven offline.
package clustersource

import (
	"context"
	"fmt"

	"github.com/Karthikbangari/kubeloop-/internal/readlayer/kubeparse"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/manifestsource"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/promql"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/promusage"
	"github.com/Karthikbangari/kubeloop-/internal/scan"
)

// Querier issues one Prometheus instant query. promclient.Client implements it.
// The (value, ok, error) shape is load-bearing: ok=false means "no data", which
// is a legitimate answer, while error means the query or the server failed.
type Querier interface {
	Query(ctx context.Context, promQL string) (float64, bool, error)
}

// Collect measures each workload and assembles the scan inputs.
//
// The two failure modes are deliberately kept apart:
//
//   - A query that returns no data (ok=false) leaves that usage number at zero.
//     The scanner's missing-signal rule then excludes the workload *with a
//     printed reason* — an un-instrumented workload is reported, never sized.
//   - A query that errors aborts the whole collection. A Prometheus that is
//     down or rejecting queries must never be mistaken for "this cluster has no
//     usage": that would silently exclude every workload and report $0 waste,
//     which reads as "nothing to save" rather than "the scan failed".
func Collect(ctx context.Context, ws []kubeparse.Workload, q Querier, r promql.Range) ([]scan.Input, error) {
	inputs := make([]scan.Input, 0, len(ws))
	for _, w := range ws {
		sel, err := promql.WorkloadPods(w.Kind, w.Namespace, w.Name)
		if err != nil {
			// kubeclient only lists kinds promql knows how to match. Reaching
			// here means the two disagree — a bug, not a user's cluster.
			return nil, fmt.Errorf("%s/%s: %w", w.Namespace, w.Name, err)
		}
		p95, _, err := scalar(ctx, q, promql.CPUQuantile(sel, 0.95, r))
		if err != nil {
			return nil, fmt.Errorf("%s/%s p95 cpu: %w", w.Namespace, w.Name, err)
		}
		p99, _, err := scalar(ctx, q, promql.CPUQuantile(sel, 0.99, r))
		if err != nil {
			return nil, fmt.Errorf("%s/%s p99 cpu: %w", w.Namespace, w.Name, err)
		}
		mem, _, err := scalar(ctx, q, promql.MaxMemory(sel, r))
		if err != nil {
			return nil, fmt.Errorf("%s/%s max memory: %w", w.Namespace, w.Name, err)
		}
		days, _, err := scalar(ctx, q, promql.HistoryDays(sel, r))
		if err != nil {
			return nil, fmt.Errorf("%s/%s history: %w", w.Namespace, w.Name, err)
		}
		usage := promusage.AssembleUsage(p95, p99, mem)
		inputs = append(inputs, manifestsource.FromWorkload(w, usage, int(days+0.5)))
	}
	return inputs, nil
}

// scalar runs one query; "no data" collapses to a zero value, which downstream
// safety turns into an exclusion with a reason.
func scalar(ctx context.Context, q Querier, promQL string) (float64, bool, error) {
	v, ok, err := q.Query(ctx, promQL)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	return v, true, nil
}
