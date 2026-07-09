package main

import (
	rp "github.com/Karthikbangari/kubeloop-/internal/reporting"
	"github.com/Karthikbangari/kubeloop-/internal/savings"
	"github.com/Karthikbangari/kubeloop-/internal/scan"
)

// jsonReport is the explicit, stable wire format for --json. It is intentionally
// decoupled from the internal scan/reporting structs so refactors there don't
// silently change the public JSON contract.
type jsonReport struct {
	EstimatedMonthlyWasteUSD float64        `json:"estimatedMonthlyWasteUsd"`
	Realization              string         `json:"realization"`
	Workloads                []jsonWorkload `json:"workloads"`
	UnderProvisioned         []jsonWorkload `json:"underProvisioned"` // usage > request; needs more, not waste
	RightSizedCount          int            `json:"rightSizedCount"`
	Excluded                 []jsonExcluded `json:"excluded"`
}

type jsonWorkload struct {
	Namespace             string  `json:"namespace"`
	Name                  string  `json:"name"`
	Replicas              int     `json:"replicas"`
	CurrentCPUMillicores  int64   `json:"currentCpuMillicores"`
	CurrentMemBytes       int64   `json:"currentMemBytes"`
	ProposedCPUMillicores int64   `json:"proposedCpuMillicores"`
	ProposedMemBytes      int64   `json:"proposedMemBytes"`
	MonthlyWasteUSD       float64 `json:"monthlyWasteUsd"`
	Confidence            string  `json:"confidence"`
	Caution               string  `json:"caution,omitempty"`
}

type jsonExcluded struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
}

// toJSON maps the internal report onto the public wire type.
func toJSON(r scan.Report) jsonReport {
	out := jsonReport{
		EstimatedMonthlyWasteUSD: r.Total,
		Realization:              savings.Realization(r.Mode),
		Workloads:                mapRows(r.Rows),
		UnderProvisioned:         mapRows(r.Underprovisioned),
		RightSizedCount:          r.RightSized,
		Excluded:                 make([]jsonExcluded, len(r.Excluded)),
	}
	for i, e := range r.Excluded {
		out.Excluded[i] = jsonExcluded{Namespace: e.Namespace, Name: e.Name, Reason: e.Reason}
	}
	return out
}

func mapRows(rows []rp.Row) []jsonWorkload {
	out := make([]jsonWorkload, len(rows))
	for i, row := range rows {
		out[i] = jsonWorkload{
			Namespace:             row.Namespace,
			Name:                  row.Name,
			Replicas:              row.Replicas,
			CurrentCPUMillicores:  row.Current.CPU,
			CurrentMemBytes:       row.Current.Mem,
			ProposedCPUMillicores: row.Proposed.CPU,
			ProposedMemBytes:      row.Proposed.Mem,
			MonthlyWasteUSD:       row.MonthlyWaste,
			Confidence:            row.Confidence,
			Caution:               row.Caution,
		}
	}
	return out
}
