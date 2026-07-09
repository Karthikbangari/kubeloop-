// Package dirsource assembles a set of Kubernetes manifests plus a usage lookup
// into scan.Inputs — the offline "directory of manifests + usage export" read
// mode (a GitOps repo's manifests paired with a Prometheus usage dump). Reading
// files from disk is trivial glue on top of Assemble; Assemble itself is pure
// and testable.
package dirsource

import (
	"fmt"

	"github.com/Karthikbangari/kubeloop-/internal/readlayer/kubeparse"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/manifestsource"
	rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"
	"github.com/Karthikbangari/kubeloop-/internal/scan"
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
func Assemble(manifests [][]byte, usage map[string]Usage) ([]scan.Input, error) {
	inputs := make([]scan.Input, 0, len(manifests))
	for i, m := range manifests {
		w, err := kubeparse.Parse(m)
		if err != nil {
			return nil, fmt.Errorf("manifest %d: %w", i, err)
		}
		u := usage[Key(w.Namespace, w.Name)]
		inputs = append(inputs, manifestsource.FromWorkload(w, u.Usage, u.HistoryDays))
	}
	return inputs, nil
}
