// Package manifestsource bridges a real serialized Kubernetes manifest to the
// scanner: parse the object (kubeparse) → assemble a scan.Input (readlayer).
// It proves the offline read path composes end to end. The live reader supplies
// usage from Prometheus; here usage is passed in, so the bridge stays offline-
// testable.
package manifestsource

import (
	"github.com/kubeloop/kubeloop/internal/readlayer"
	"github.com/kubeloop/kubeloop/internal/readlayer/kubeparse"
	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
	"github.com/kubeloop/kubeloop/internal/scan"
)

// FromWorkload pairs an already-parsed workload with its observed usage +
// history-day count and assembles a scan.Input via readlayer.ToScanInput —
// the single place that turns a workload into scan input. Callers that have
// already parsed the manifest (e.g. dirsource) use this to avoid re-parsing.
func FromWorkload(w kubeparse.Workload, usage rs.Usage, historyDays int) scan.Input {
	return readlayer.ToScanInput(readlayer.Workload{
		Namespace:      w.Namespace,
		Name:           w.Name,
		Kind:           w.Kind,
		Replicas:       w.Replicas,
		HistoryDays:    historyDays,
		Containers:     w.Containers,
		InitContainers: w.InitContainers,
		Usage:          usage,
	})
}

// FromManifest parses a serialized workload object and pairs it with the
// workload's observed usage + history-day count to produce a scan.Input:
// current requests come from the manifest (via inventory.PodRequest), the
// runtime hint from the container images/commands, and usage from the caller.
func FromManifest(manifestJSON []byte, usage rs.Usage, historyDays int) (scan.Input, error) {
	w, err := kubeparse.Parse(manifestJSON)
	if err != nil {
		return scan.Input{}, err
	}
	return FromWorkload(w, usage, historyDays), nil
}
