// Package reporting ranks workloads by monthly waste and renders the report.
// Pure assembly over internal/rightsizing — no cluster access, testable offline.
package reporting

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kubeloop/kubeloop/internal/labels"
	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
)

// Workload is one scanned workload with its current requests and observed usage.
// The read-layer slice will produce these from the cluster; here they're input.
type Workload struct {
	Namespace string
	Name      string
	Replicas  int
	Current   rs.Resources
	Usage     rs.Usage
}

// Row is one line of the ranked report. Confidence/Caution are optional plain
// strings the caller fills (kept as strings so reporting stays decoupled from
// the safety package); a blank Confidence hides the CONF column entirely.
type Row struct {
	Workload
	Proposed     rs.Resources
	MonthlyWaste float64
	Confidence   string
	Caution      string
}

// Rank computes proposed requests and monthly waste per workload, sorted by
// waste descending, and returns the headline total. Stable sort so equal-waste
// workloads keep input order (deterministic output for tests and diffs).
func Rank(ws []Workload, r rs.Recommender, p rs.Price) (rows []Row, total float64) {
	rows = make([]Row, len(ws))
	for i, w := range ws {
		proposed := r.Recommend(w.Usage)
		rows[i] = Row{Workload: w, Proposed: proposed, MonthlyWaste: rs.MonthlyWaste(w.Current, proposed, w.Replicas, p)}
		total += rows[i].MonthlyWaste
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].MonthlyWaste > rows[j].MonthlyWaste })
	return rows, total
}

// Render is a plain aligned table. The CONF column appears only when at least
// one row carries a Confidence; per-row Cautions print under the table. This is
// the single source of resource formatting and namespace-collision labels — the
// scan report renders through here rather than duplicating it.
// ponytail: no color until the CLI wires it — stdlib tabwriter handles alignment.
func Render(rows []Row, total float64) string {
	lbls := rowLabels(rows)
	showConf := false
	for _, r := range rows {
		if r.Confidence != "" {
			showConf = true
			break
		}
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	header := "WORKLOAD\tCURRENT\tPROPOSED\t$/MONTH"
	if showConf {
		header += "\tCONF"
	}
	fmt.Fprintln(w, header)
	for i, r := range rows {
		line := fmt.Sprintf("%s\t%s\t%s\t$%.2f", lbls[i], resStr(r.Current), resStr(r.Proposed), r.MonthlyWaste)
		if showConf {
			line += "\t" + r.Confidence
		}
		fmt.Fprintln(w, line)
	}
	w.Flush()
	fmt.Fprintf(&b, "\nEstimated waste: $%.2f/month across %s.\n", total, Plural(len(rows), "workload"))
	for i, r := range rows {
		if r.Caution != "" {
			fmt.Fprintf(&b, "  ! %s: %s\n", lbls[i], r.Caution)
		}
	}
	return b.String()
}

// rowLabels qualifies each row's name via the shared collision-aware helper, so
// the ranked table and the excluded list disambiguate identically.
func rowLabels(rows []Row) []string {
	items := make([]labels.Item, len(rows))
	for i, r := range rows {
		items[i] = labels.Item{Namespace: r.Namespace, Name: r.Name}
	}
	return labels.Qualify(items)
}

// Plural formats a count with its noun, adding "s" for anything but 1:
// Plural(1,"workload") == "1 workload", Plural(2,"workload") == "2 workloads".
func Plural(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

func resStr(r rs.Resources) string { return fmt.Sprintf("%dm/%s", r.CPU, memStr(r.Mem)) }

func memStr(b int64) string {
	if b >= gib {
		return fmt.Sprintf("%.1fGi", float64(b)/gib)
	}
	return fmt.Sprintf("%dMi", b/(1024*1024))
}
