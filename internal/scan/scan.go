// Package scan is the end-to-end offline path that ties the pieces together:
// assess → exclude → rank → score → render. It takes workloads already read
// from the cluster (the read-layer's job, not built here) and produces the
// report a user sees. No cluster access in this package.
package scan

import (
	"fmt"
	"strings"

	"github.com/kubeloop/kubeloop/internal/classify"
	"github.com/kubeloop/kubeloop/internal/labels"
	rp "github.com/kubeloop/kubeloop/internal/reporting"
	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
	sf "github.com/kubeloop/kubeloop/internal/safety"
	"github.com/kubeloop/kubeloop/internal/savings"
)

// Input is one workload plus the metadata safety needs to judge it.
type Input struct {
	rp.Workload
	Meta sf.Meta
}

// Excluded records a skipped workload and why (printed, never silent).
type Excluded struct {
	Namespace, Name, Reason string
}

// Report is the full scan result: ranked kept workloads (each carrying its
// confidence/caution), the skipped ones, the headline total, and the billing
// mode that decides when those dollars are realized.
type Report struct {
	Rows             []rp.Row // waste, ranked by $/month desc
	Underprovisioned []rp.Row // usage exceeds request — needs more, not waste
	RightSized       int      // count already at their proposal (nothing to do)
	Excluded         []Excluded
	Total            float64
	Mode             savings.Mode
}

func key(ns, name string) string { return ns + "/" + name }

// Scan applies safety exclusions, ranks the kept workloads by monthly waste,
// then scores each ranked row's confidence onto the row. Mode only affects how
// the total is described (immediate vs on node consolidation), not the numbers.
func Scan(inputs []Input, rec rs.Recommender, price rs.Price, mode savings.Mode) Report {
	var kept []rp.Workload
	metas := make(map[string]sf.Meta, len(inputs))
	var excluded []Excluded
	for _, in := range inputs {
		if e := sf.Assess(in.Meta, in.Usage); e.Excluded {
			excluded = append(excluded, Excluded{in.Namespace, in.Name, e.Reason})
			continue
		}
		kept = append(kept, in.Workload)
		metas[key(in.Namespace, in.Name)] = in.Meta
	}
	rows, total := rp.Rank(kept, rec, price)
	var waste, under []rp.Row
	rightSized := 0
	for i := range rows {
		c := sf.Score(metas[key(rows[i].Namespace, rows[i].Name)], rows[i].Usage)
		rows[i].Confidence = c.Level
		rows[i].Caution = c.Caution
		// Keep under-provisioned/right-sized out of the waste ranking so the
		// report never presents an "increase this" line as a saving. Their
		// MonthlyWaste is already 0, so total (from Rank) stays the waste total.
		switch classify.Classify(rows[i].Current, rows[i].Proposed) {
		case classify.Waste:
			waste = append(waste, rows[i])
		case classify.UnderProvisioned:
			under = append(under, rows[i])
		default:
			rightSized++
		}
	}
	return Report{Rows: waste, Underprovisioned: under, RightSized: rightSized, Excluded: excluded, Total: total, Mode: mode}
}

// Render delegates the ranked table (with CONF column + cautions) to reporting
// so resource formatting and collision labels stay single-sourced, then appends
// the excluded section.
func Render(r Report) string {
	var b strings.Builder
	b.WriteString(rp.Render(r.Rows, r.Total))
	fmt.Fprintf(&b, "  -> %s\n", savings.Realization(r.Mode))
	if len(r.Underprovisioned) > 0 {
		items := make([]labels.Item, len(r.Underprovisioned))
		for i, row := range r.Underprovisioned {
			items[i] = labels.Item{Namespace: row.Namespace, Name: row.Name}
		}
		lbls := labels.Qualify(items)
		b.WriteString("\nUnder-provisioned (usage exceeds request — needs more, not waste):\n")
		for _, l := range lbls {
			fmt.Fprintf(&b, "  - %s\n", l)
		}
	}
	if r.RightSized > 0 {
		fmt.Fprintf(&b, "\n%d workload(s) already right-sized.\n", r.RightSized)
	}
	if len(r.Excluded) > 0 {
		items := make([]labels.Item, len(r.Excluded))
		for i, e := range r.Excluded {
			items[i] = labels.Item{Namespace: e.Namespace, Name: e.Name}
		}
		lbls := labels.Qualify(items)
		b.WriteString("\nExcluded:\n")
		for i, e := range r.Excluded {
			fmt.Fprintf(&b, "  - %s: %s\n", lbls[i], e.Reason)
		}
	}
	return b.String()
}
