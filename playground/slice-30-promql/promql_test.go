package promql

import (
	"regexp"
	"strings"
	"testing"
)

// anchored mimics Prometheus, which anchors =~ matchers at both ends.
func anchored(t *testing.T, podRegex string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(`^(?:` + podRegex + `)$`)
	if err != nil {
		t.Fatalf("regex %q does not compile: %v", podRegex, err)
	}
	return re
}

// The bug this package exists to avoid: `checkout-api-.*` would also match the
// pods of a different Deployment named `checkout-api-v2`, quietly summing
// another service's load into checkout-api's usage.
func TestWorkloadPods_DeploymentDoesNotMatchSiblingWorkload(t *testing.T) {
	sel, err := WorkloadPods("Deployment", "shop", "checkout-api")
	if err != nil {
		t.Fatal(err)
	}
	re := anchored(t, sel.PodRegex)

	if !re.MatchString("checkout-api-6d4f8b9c7-abc12") {
		t.Errorf("regex %q must match its own pod", sel.PodRegex)
	}
	for _, other := range []string{
		"checkout-api-v2-6d4f8b9c7-abc12", // sibling workload, longer name
		"checkout-api",                    // the bare name is not a pod
		"other-api-6d4f8b9c7-abc12",
	} {
		if re.MatchString(other) {
			t.Errorf("regex %q must NOT match %q — that is another workload's pod", sel.PodRegex, other)
		}
	}
}

func TestWorkloadPods_StatefulSetOrdinalsOnly(t *testing.T) {
	sel, err := WorkloadPods("StatefulSet", "data", "pg")
	if err != nil {
		t.Fatal(err)
	}
	re := anchored(t, sel.PodRegex)
	for _, own := range []string{"pg-0", "pg-11"} {
		if !re.MatchString(own) {
			t.Errorf("regex %q must match %q", sel.PodRegex, own)
		}
	}
	for _, other := range []string{"pg-backup-0", "pgbouncer-0", "pg"} {
		if re.MatchString(other) {
			t.Errorf("regex %q must NOT match %q", sel.PodRegex, other)
		}
	}
}

// A workload name containing a regex metacharacter must be quoted, or "a.b"
// would match "axb".
func TestWorkloadPods_QuotesRegexMetacharacters(t *testing.T) {
	sel, err := WorkloadPods("StatefulSet", "ns", "a.b")
	if err != nil {
		t.Fatal(err)
	}
	re := anchored(t, sel.PodRegex)
	if !re.MatchString("a.b-0") {
		t.Errorf("regex %q must match the literal name", sel.PodRegex)
	}
	if re.MatchString("axb-0") {
		t.Errorf("regex %q treated '.' as a wildcard", sel.PodRegex)
	}
}

func TestWorkloadPods_UnknownKindErrors(t *testing.T) {
	if _, err := WorkloadPods("CronJob", "ns", "x"); err == nil {
		t.Error("want error for a kind with no pod-naming rule, not a wrong guess")
	}
}

// A quote in a label value would otherwise close the string and change the query.
func TestEscapeLabelValue(t *testing.T) {
	for in, want := range map[string]string{
		`plain`:    `plain`,
		`a"b`:      `a\"b`,
		`a\b`:      `a\\b`,
		"a\nb":     `a\nb`,
		`x"; up{}`: `x\"; up{}`,
	} {
		if got := escapeLabelValue(in); got != want {
			t.Errorf("escapeLabelValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCPUQuantile_ExactQuery(t *testing.T) {
	sel, _ := WorkloadPods("Deployment", "shop", "checkout-api")
	got := CPUQuantile(sel, 0.95, Defaults())
	want := `max(quantile_over_time(0.95, sum by (pod) (rate(container_cpu_usage_seconds_total{` +
		`namespace="shop",pod=~"checkout-api-[a-z0-9]+-[a-z0-9]+",container!="",container!="POD"}[5m]))[7d:5m]))`
	if got != want {
		t.Errorf("CPUQuantile =\n  %s\nwant\n  %s", got, want)
	}
	// 0.99 must render as 0.99, not 0.990000 or 9.9e-01.
	if q := CPUQuantile(sel, 0.99, Defaults()); !strings.Contains(q, "quantile_over_time(0.99,") {
		t.Errorf("quantile 0.99 rendered wrong: %s", q)
	}
}

func TestMaxMemory_ExactQuery(t *testing.T) {
	sel, _ := WorkloadPods("StatefulSet", "data", "pg")
	got := MaxMemory(sel, Defaults())
	want := `max(max_over_time(sum by (pod) (container_memory_working_set_bytes{` +
		`namespace="data",pod=~"pg-[0-9]+",container!="",container!="POD"})[7d:5m]))`
	if got != want {
		t.Errorf("MaxMemory =\n  %s\nwant\n  %s", got, want)
	}
}

func TestHistoryDays_UsesDailyStepOverHistoryWindow(t *testing.T) {
	sel, _ := WorkloadPods("Deployment", "shop", "checkout-api")
	got := HistoryDays(sel, Defaults())
	if !strings.Contains(got, "[30d:1d]") {
		t.Errorf("HistoryDays should count daily steps over the history window: %s", got)
	}
	if !strings.HasPrefix(got, "count_over_time(") {
		t.Errorf("HistoryDays should count samples: %s", got)
	}
}

// Regression: HistoryDays must measure the WORKLOAD's age, not the longest-lived
// pod's. Pods are replaced on every deploy, so `max(... sum by (pod) ...)` would
// report ~1 day for a year-old service that ships daily, and safety would
// silently exclude it as "<7d of history". Aggregating the pod label away first
// counts the days on which any pod existed.
func TestHistoryDays_AggregatesPodLabelAway(t *testing.T) {
	sel, _ := WorkloadPods("Deployment", "shop", "checkout-api")
	got := HistoryDays(sel, Defaults())
	if strings.Contains(got, "by (pod)") {
		t.Errorf("HistoryDays must not group by pod — that measures pod lifetime, not workload age: %s", got)
	}
	if strings.HasPrefix(got, "max(") {
		t.Errorf("HistoryDays must not take max across pods: %s", got)
	}
	want := `count_over_time(sum(container_cpu_usage_seconds_total{` +
		`namespace="shop",pod=~"checkout-api-[a-z0-9]+-[a-z0-9]+",container!="",container!="POD"})[30d:1d])`
	if got != want {
		t.Errorf("HistoryDays =\n  %s\nwant\n  %s", got, want)
	}
}

// CPU and memory, unlike history, are correctly per-pod: a request is sized per
// pod, so the busiest pod is the one that matters.
func TestUsageQueries_AreStillPerPodThenMaxed(t *testing.T) {
	sel, _ := WorkloadPods("Deployment", "shop", "checkout-api")
	for name, q := range map[string]string{
		"CPUQuantile": CPUQuantile(sel, 0.95, Defaults()),
		"MaxMemory":   MaxMemory(sel, Defaults()),
	} {
		if !strings.Contains(q, "sum by (pod)") {
			t.Errorf("%s must aggregate per pod: %s", name, q)
		}
		if !strings.HasPrefix(q, "max(") {
			t.Errorf("%s must take the busiest pod: %s", name, q)
		}
	}
}

// Every query must exclude the pause container and the cgroup rollup series,
// or usage is overstated.
func TestQueries_ExcludeRollupAndPauseSeries(t *testing.T) {
	sel, _ := WorkloadPods("Deployment", "shop", "api")
	r := Defaults()
	for name, q := range map[string]string{
		"CPUQuantile": CPUQuantile(sel, 0.95, r),
		"MaxMemory":   MaxMemory(sel, r),
		"HistoryDays": HistoryDays(sel, r),
	} {
		if !strings.Contains(q, `container!=""`) || !strings.Contains(q, `container!="POD"`) {
			t.Errorf("%s does not exclude rollup/pause series: %s", name, q)
		}
		if !strings.Contains(q, `namespace="shop"`) {
			t.Errorf("%s is not namespace-scoped: %s", name, q)
		}
	}
}
