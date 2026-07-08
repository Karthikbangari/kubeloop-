// Package readlayer is a fake read-layer model for proving how Kubernetes
// workload inventory becomes scan input without needing a live cluster.
package readlayer

import (
	"github.com/kubeloop/kubeloop/internal/inventory"
	rp "github.com/kubeloop/kubeloop/internal/reporting"
	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
	sf "github.com/kubeloop/kubeloop/internal/safety"
	"github.com/kubeloop/kubeloop/internal/scan"
)

// Workload is the minimal shape a future Kubernetes read-layer must assemble
// after it has already parsed resource.Quantity values into numeric requests.
type Workload struct {
	Namespace      string
	Name           string
	Kind           string
	Replicas       int
	HistoryDays    int
	Containers     []inventory.Container
	InitContainers []inventory.Container
	Usage          rs.Usage
}

// ToScanInput converts one inventory record to the stable offline scan input.
// It is intentionally pure: no kubeconfig, no Prometheus, no file IO. The
// manifest read-layer (readlayer/manifestsource) reuses this so there is a
// single place that assembles a scan.Input from a workload.
func ToScanInput(w Workload) scan.Input {
	allContainers := append([]inventory.Container{}, w.Containers...)
	allContainers = append(allContainers, w.InitContainers...)
	return scan.Input{
		Workload: rp.Workload{
			Namespace: w.Namespace,
			Name:      w.Name,
			Replicas:  w.Replicas,
			Current:   inventory.PodRequest(w.Containers, w.InitContainers),
			Usage:     w.Usage,
		},
		Meta: sf.Meta{
			Kind:        w.Kind,
			HistoryDays: w.HistoryDays,
			Runtime:     inventory.DetectRuntime(allContainers),
		},
	}
}

// ToScanInputs converts many inventory records to the stable offline scan input.
func ToScanInputs(ws []Workload) []scan.Input {
	out := make([]scan.Input, len(ws))
	for i, w := range ws {
		out[i] = ToScanInput(w)
	}
	return out
}
