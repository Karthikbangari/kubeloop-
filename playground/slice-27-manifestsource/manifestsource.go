// Package manifestsource bridges a real serialized Kubernetes manifest to the
// scanner: parse the object (kubeparse) → aggregate its containers (inventory)
// → emit a scan.Input. It proves the offline read path composes end to end.
// The live reader supplies usage from Prometheus; here usage is passed in, so
// the whole bridge stays offline-testable.
package manifestsource

import (
	"github.com/kubeloop/kubeloop/internal/inventory"
	rp "github.com/kubeloop/kubeloop/internal/reporting"
	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
	sf "github.com/kubeloop/kubeloop/internal/safety"
	"github.com/kubeloop/kubeloop/internal/scan"
	kubeparse "github.com/kubeloop/kubeloop/playground/slice-26-kubeparse"
)

// FromManifest parses a serialized workload object and pairs it with the
// workload's observed usage + history-day count to produce a scan.Input:
// current requests come from the manifest (via inventory.PodRequest), the
// runtime hint from the container images/commands, and usage from the caller.
func FromManifest(manifestJSON []byte, usage rs.Usage, historyDays int) (scan.Input, error) {
	w, err := kubeparse.Parse(manifestJSON)
	if err != nil {
		return scan.Input{}, err
	}
	all := append(append([]inventory.Container{}, w.Containers...), w.InitContainers...)
	return scan.Input{
		Workload: rp.Workload{
			Namespace: w.Namespace,
			Name:      w.Name,
			Replicas:  w.Replicas,
			Current:   inventory.PodRequest(w.Containers, w.InitContainers),
			Usage:     usage,
		},
		Meta: sf.Meta{
			Kind:        w.Kind,
			HistoryDays: historyDays,
			Runtime:     inventory.DetectRuntime(all),
		},
	}, nil
}
