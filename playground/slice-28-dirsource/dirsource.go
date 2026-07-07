// Package dirsource assembles a set of Kubernetes manifests plus a usage lookup
// into scan.Inputs — the offline "directory of manifests + usage export" read
// mode (a GitOps repo's manifests paired with a Prometheus usage dump). Reading
// files from disk is trivial glue on top of Assemble; Assemble itself is pure
// and testable.
package dirsource

import (
	"fmt"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
	"github.com/kubeloop/kubeloop/internal/scan"
	kubeparse "github.com/kubeloop/kubeloop/playground/slice-26-kubeparse"
	manifestsource "github.com/kubeloop/kubeloop/playground/slice-27-manifestsource"
)

// Usage is a workload's observed usage plus its history-day count.
type Usage struct {
	rs.Usage
	HistoryDays int
}

// Key builds the "namespace/name" usage-lookup key.
func Key(namespace, name string) string { return namespace + "/" + name }

// Assemble parses each manifest and attaches usage looked up by namespace/name.
// A workload with no usage entry gets zero usage, which the scanner then
// excludes via the missing-signal rule — so an un-instrumented workload is
// reported, never sized on no data.
//
// ponytail: parses each manifest twice (once for the lookup key, once inside
// FromManifest); fold into one parse when this graduates next to manifestsource.
func Assemble(manifests [][]byte, usage map[string]Usage) ([]scan.Input, error) {
	inputs := make([]scan.Input, 0, len(manifests))
	for i, m := range manifests {
		w, err := kubeparse.Parse(m)
		if err != nil {
			return nil, fmt.Errorf("manifest %d: %w", i, err)
		}
		u := usage[Key(w.Namespace, w.Name)]
		in, err := manifestsource.FromManifest(m, u.Usage, u.HistoryDays)
		if err != nil {
			return nil, fmt.Errorf("manifest %d (%s/%s): %w", i, w.Namespace, w.Name, err)
		}
		inputs = append(inputs, in)
	}
	return inputs, nil
}
