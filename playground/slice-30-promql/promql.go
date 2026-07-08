// Package promql builds the query strings that turn a workload into usage
// numbers. It is pure string construction — no I/O — so the escaping and the
// pod-matching rules are provable offline. promclient issues the strings;
// promusage parses the answers.
//
// ⚠ NOT VALIDATED AGAINST A LIVE PROMETHEUS. Every query below is unit-tested
// for exact output, but no one has yet run them against a real server with real
// cadvisor metrics. They assume cadvisor/kubelet metric names
// (container_cpu_usage_seconds_total, container_memory_working_set_bytes) and a
// `pod` label. Treat the numbers as unverified until that check happens.
package promql

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Range is the time shape of the usage queries.
type Range struct {
	Window  string // lookback for the percentile/max, e.g. "7d"
	Rate    string // rate() window for the CPU counter, e.g. "5m"
	Step    string // subquery resolution, e.g. "5m"
	History string // lookback for the history-days probe, e.g. "30d"
}

// Defaults matches the safety rule that <7d of history is not enough signal.
func Defaults() Range { return Range{Window: "7d", Rate: "5m", Step: "5m", History: "30d"} }

// Selector identifies exactly one workload's pods.
type Selector struct {
	Namespace string
	PodRegex  string
}

// WorkloadPods builds a pod regex that matches a workload's pods and *only*
// its pods.
//
// The obvious regex — `<name>-.*` — is wrong, and silently so: sizing
// "checkout-api" would also sweep in the pods of a *different* Deployment named
// "checkout-api-v2", inflating its usage with another service's load. So the
// regex is kind-aware and matches the real pod-naming schemes:
//
//	Deployment  → <name>-<replicaset-hash>-<rand>   e.g. checkout-api-6d4f8b9c7-abc12
//	StatefulSet → <name>-<ordinal>                  e.g. pg-0
//
// Because a hash segment can't contain "-", "checkout-api-v2-6d4f8b9c7-abc12"
// leaves the remainder "v2-6d4f8b9c7-abc12", which has one segment too many and
// therefore does not match. Prometheus anchors `=~` at both ends, so no ^ or $
// is needed — and adding them would be wrong for the same reason it looks right.
func WorkloadPods(kind, namespace, name string) (Selector, error) {
	n := regexp.QuoteMeta(name)
	var podRegex string
	switch kind {
	case "Deployment":
		podRegex = n + "-[a-z0-9]+-[a-z0-9]+"
	case "StatefulSet":
		podRegex = n + "-[0-9]+"
	default:
		return Selector{}, fmt.Errorf("no pod-naming rule for kind %q", kind)
	}
	return Selector{Namespace: namespace, PodRegex: podRegex}, nil
}

// escapeLabelValue escapes a value for a double-quoted PromQL label matcher.
// An unescaped quote would end the string early and change the query's meaning.
func escapeLabelValue(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}

// matchers renders the label set shared by every query. container!="" drops the
// per-pod cgroup rollup series and container!="POD" drops the pause container;
// counting either would overstate usage.
func (s Selector) matchers() string {
	return fmt.Sprintf(`namespace=%q,pod=~"%s",container!="",container!="POD"`,
		escapeLabelValue(s.Namespace), escapeLabelValue(s.PodRegex))
}

// CPUQuantile is the quantile of per-pod CPU (cores) over the window. Inner
// sum by (pod) totals a pod's containers; the outer max() takes the busiest
// pod, because a request is sized per pod, not per workload.
func CPUQuantile(s Selector, quantile float64, r Range) string {
	q := strconv.FormatFloat(quantile, 'g', -1, 64)
	return fmt.Sprintf(
		`max(quantile_over_time(%s, sum by (pod) (rate(container_cpu_usage_seconds_total{%s}[%s]))[%s:%s]))`,
		q, s.matchers(), r.Rate, r.Window, r.Step)
}

// MaxMemory is the peak working-set bytes of the busiest pod over the window.
// Working set (not RSS, not cache) is what the kubelet OOM-kills against.
func MaxMemory(s Selector, r Range) string {
	return fmt.Sprintf(
		`max(max_over_time(sum by (pod) (container_memory_working_set_bytes{%s})[%s:%s]))`,
		s.matchers(), r.Window, r.Step)
}

// HistoryDays counts distinct days that carry data, as a proxy for "how long
// has this workload been observed" — the input to safety's <7d exclusion.
//
// It is a proxy, not a truth: PromQL cannot cheaply report a series' age, so
// this counts 1-day-step samples over the history window and saturates there
// (a 200-day-old workload reports 30 with History="30d"). That is fine for a
// rule that only asks "≥ 7?", and it fails safe — undercounting excludes a
// workload rather than sizing it on thin data.
func HistoryDays(s Selector, r Range) string {
	return fmt.Sprintf(
		`max(count_over_time(sum by (pod) (container_cpu_usage_seconds_total{%s})[%s:1d]))`,
		s.matchers(), r.History)
}
