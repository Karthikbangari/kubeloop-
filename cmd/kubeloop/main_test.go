package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One rankable deployment (2000m→576m) + one CronJob that must be excluded.
const sample = `[
 {"Namespace":"shop","Name":"checkout-api","Replicas":1,"Current":{"CPU":2000},"Usage":{"P95CPU":410,"P99CPU":480,"MaxMem":314572800},"Meta":{"Kind":"Deployment","HistoryDays":30}},
 {"Namespace":"batch","Name":"nightly","Usage":{"P95CPU":100,"P99CPU":120,"MaxMem":104857600},"Meta":{"Kind":"CronJob","HistoryDays":30}}
]`

func sampleFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(p, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRun_Text(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"--from-file", sampleFile(t)}, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"$/MONTH", "checkout-api", "consolidate", "Excluded:", "nightly"} {
		if !strings.Contains(s, want) {
			t.Errorf("text output missing %q:\n%s", want, s)
		}
	}
}

func TestRun_JSONSchema(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"--json", "--from-file", sampleFile(t)}, &out); err != nil {
		t.Fatal(err)
	}
	var r jsonReport // unmarshal into the public type: verifies the schema
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(r.Workloads) != 1 || len(r.Excluded) != 1 {
		t.Fatalf("want 1 workload + 1 excluded, got %d/%d", len(r.Workloads), len(r.Excluded))
	}
	if r.Workloads[0].ProposedCPUMillicores != 576 {
		t.Errorf("proposed CPU = %d, want 576", r.Workloads[0].ProposedCPUMillicores)
	}
	if r.EstimatedMonthlyWasteUSD <= 0 || r.Realization == "" {
		t.Errorf("want positive total + realization, got %v / %q", r.EstimatedMonthlyWasteUSD, r.Realization)
	}
}

func TestRun_JSONIncludesCaution(t *testing.T) {
	// The public JSON schema must carry the safety caution so machine consumers
	// (building their own automation) see the JVM warning, like the PR body does.
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	body := `[{"Namespace":"shop","Name":"search","Replicas":1,"Current":{"CPU":1000,"Mem":2147483648},"Usage":{"P95CPU":300,"P99CPU":350,"MaxMem":524288000},"Meta":{"Kind":"Deployment","HistoryDays":30,"Runtime":"jvm"}}]`
	if err := os.WriteFile(in, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"--json", "--from-file", in}, &out); err != nil {
		t.Fatal(err)
	}
	var r jsonReport
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(r.Workloads) != 1 || !strings.Contains(r.Workloads[0].Caution, "JVM") {
		t.Errorf("JSON should carry the JVM caution, got %+v", r.Workloads)
	}
}

func TestRun_PerRequestChangesRealization(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"--per-request", "--from-file", sampleFile(t)}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "immediately") {
		t.Errorf("--per-request should say immediate:\n%s", out.String())
	}
}

func TestRun_PricingFileRaisesWaste(t *testing.T) {
	// A higher CPU price must increase the reported waste vs the default.
	base := &bytes.Buffer{}
	if err := Run([]string{"--json", "--from-file", sampleFile(t)}, base); err != nil {
		t.Fatal(err)
	}
	pf := filepath.Join(t.TempDir(), "pricing.json")
	if err := os.WriteFile(pf, []byte(`{"clouds":{"aws":{"perVCPUHour":0.5}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	over := &bytes.Buffer{}
	if err := Run([]string{"--json", "--pricing-file", pf, "--from-file", sampleFile(t)}, over); err != nil {
		t.Fatal(err)
	}
	var b, o jsonReport
	json.Unmarshal(base.Bytes(), &b)
	json.Unmarshal(over.Bytes(), &o)
	if !(o.EstimatedMonthlyWasteUSD > b.EstimatedMonthlyWasteUSD) {
		t.Errorf("override total %v should exceed default %v", o.EstimatedMonthlyWasteUSD, b.EstimatedMonthlyWasteUSD)
	}
}

func TestRun_ScanSubcommandAndBareBothWork(t *testing.T) {
	// Regression: `kubeloop scan --from-file X` used to fail with
	// "--from-file required" because "scan" was swallowed as a positional arg.
	f := sampleFile(t)
	for _, args := range [][]string{
		{"--from-file", f},         // bare invocation
		{"scan", "--from-file", f}, // explicit subcommand (README/plan form)
	} {
		var out bytes.Buffer
		if err := Run(args, &out); err != nil {
			t.Fatalf("Run(%v) errored: %v", args, err)
		}
		if !strings.Contains(out.String(), "$/MONTH") {
			t.Errorf("Run(%v) produced no table:\n%s", args, out.String())
		}
	}
}

func TestRun_RequiresFromFile(t *testing.T) {
	if err := Run(nil, &bytes.Buffer{}); err == nil {
		t.Error("want error when --from-file missing")
	}
}

// manifestDir writes a JVM deployment (rankable, carries a caution) and an
// un-instrumented one (no usage entry → excluded, never sized), plus a usage
// export keyed by namespace/name. Returns the manifest dir and the usage path.
func manifestDir(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	deploy := func(name, image, cpu string) string {
		return `{"kind":"Deployment","metadata":{"name":"` + name + `","namespace":"shop"},` +
			`"spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"app","image":"` + image + `",` +
			`"resources":{"requests":{"cpu":"` + cpu + `","memory":"2Gi"}}}]}}}}`
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("search.json", deploy("search", "eclipse-temurin:21-jre", "1000m"))
	write("no-metrics.json", deploy("no-metrics", "nginx", "1000m"))
	write("notes.txt", "ignored: not a .json manifest")

	usage := filepath.Join(t.TempDir(), "usage.json")
	body := `{"shop/search":{"P95CPU":300,"P99CPU":350,"MaxMem":524288000,"HistoryDays":30}}`
	if err := os.WriteFile(usage, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, usage
}

// The offline manifest read path end to end: manifests + usage export → ranked
// report, with the un-instrumented workload excluded rather than sized on no data.
func TestRun_FromManifests(t *testing.T) {
	dir, usage := manifestDir(t)
	var out bytes.Buffer
	if err := Run([]string{"scan", "--json", "--from-manifests", dir, "--usage-file", usage}, &out); err != nil {
		t.Fatal(err)
	}
	var r jsonReport
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(r.Workloads) != 1 || r.Workloads[0].Name != "search" {
		t.Fatalf("want only search ranked, got %+v", r.Workloads)
	}
	// Current requests come from the manifest; the proposal from the usage export.
	if r.Workloads[0].CurrentCPUMillicores != 1000 || r.Workloads[0].ProposedCPUMillicores != 420 {
		t.Errorf("current/proposed CPU = %d/%d, want 1000/420 (P99×1.2)",
			r.Workloads[0].CurrentCPUMillicores, r.Workloads[0].ProposedCPUMillicores)
	}
	if r.Workloads[0].Caution == "" {
		t.Error("JVM caution should survive the manifest read path")
	}
	if len(r.Excluded) != 1 || r.Excluded[0].Name != "no-metrics" {
		t.Fatalf("want no-metrics excluded (metrics gap), got %+v", r.Excluded)
	}
}

func TestRun_FromManifestsRejectsBothSources(t *testing.T) {
	dir, usage := manifestDir(t)
	err := Run([]string{"--from-file", sampleFile(t), "--from-manifests", dir, "--usage-file", usage}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exactly one of") {
		t.Fatalf("want mutually-exclusive-source error, got %v", err)
	}
}

// A typo in the usage export must fail loudly rather than zero usage and
// silently exclude the workload with a misleading "metrics gap" reason.
func TestRun_FromManifestsRejectsUnknownUsageField(t *testing.T) {
	dir, _ := manifestDir(t)
	usage := filepath.Join(t.TempDir(), "usage.json")
	if err := os.WriteFile(usage, []byte(`{"shop/search":{"MaxMemory":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"--from-manifests", dir, "--usage-file", usage}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("want unknown-field error, got %v", err)
	}
}

func TestRun_FromManifestsEmptyDirErrors(t *testing.T) {
	err := Run([]string{"--from-manifests", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no .json manifests") {
		t.Fatalf("want empty-dir error, got %v", err)
	}
}

func TestRun_UsageFileRejectedWithFromFile(t *testing.T) {
	_, usage := manifestDir(t)
	err := Run([]string{"--from-file", sampleFile(t), "--usage-file", usage}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--usage-file applies to --from-manifests") {
		t.Fatalf("want usage-file misuse error, got %v", err)
	}
}

// --from-cluster without a Prometheus URL cannot measure usage. Fail before
// shelling out to kubectl, not after.
func TestRun_FromClusterRequiresPrometheus(t *testing.T) {
	err := Run([]string{"scan", "--from-cluster"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--prometheus is required") {
		t.Fatalf("want a prometheus-required error, got %v", err)
	}
}

func TestRun_FromClusterRejectsUsageFile(t *testing.T) {
	_, usage := manifestDir(t)
	err := Run([]string{"scan", "--from-cluster", "--prometheus", "http://x:9090", "--usage-file", usage}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "reads usage from Prometheus") {
		t.Fatalf("want usage-file misuse error, got %v", err)
	}
}

func TestRun_RejectsThreeSourcesAtOnce(t *testing.T) {
	dir, _ := manifestDir(t)
	err := Run([]string{"--from-file", sampleFile(t), "--from-manifests", dir, "--from-cluster"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exactly one of") {
		t.Fatalf("want mutually-exclusive-source error, got %v", err)
	}
}

// Cluster-only flags on an offline source are a wrong mental model. Say so
// rather than silently ignoring them.
func TestRun_ClusterFlagsRejectedOnOfflineSources(t *testing.T) {
	dir, _ := manifestDir(t)
	cases := [][]string{
		{"--from-file", sampleFile(t), "--prometheus", "http://x:9090"},
		{"--from-file", sampleFile(t), "--context", "prod"},
		{"--from-manifests", dir, "--namespace", "shop"},
		{"--from-manifests", dir, "--prometheus", "http://x:9090"},
	}
	for _, args := range cases {
		err := Run(args, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "applies to --from-cluster") {
			t.Errorf("args %v: want cluster-flag misuse error, got %v", args, err)
		}
	}
}

func TestRun_NoSourceIsAnError(t *testing.T) {
	err := Run([]string{"scan"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("want a source-required error, got %v", err)
	}
}

// A misspelled field must fail loudly, not silently zero usage and drop the
// workload with a misleading "metrics gap" reason.
func TestRun_RejectsUnknownInputField(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	body := `[{"Namespace":"s","Name":"x","Current":{"CPU":2000},"Usage":{"P95CPU":410,"P99CPU":480,"MaxMemory":1},"Meta":{"Kind":"Deployment","HistoryDays":30}}]`
	if err := os.WriteFile(in, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"--from-file", in}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("want unknown-field error, got %v", err)
	}
}

func TestRun_RejectsTrailingInputJSON(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	if err := os.WriteFile(in, []byte(sample+"[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"--from-file", in}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("want trailing JSON error, got %v", err)
	}
}

const prManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
  namespace: shop
spec:
  template:
    spec:
      containers:
        - name: app
          image: myco/checkout:1.2
          resources:
            requests:
              cpu: 2000m
              memory: 0Mi
`

func TestRun_PRWritesPatchedManifestAndPrintsBody(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(manifest, []byte(prManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "patched.yaml")
	var out bytes.Buffer
	err := Run([]string{
		"pr",
		"--from-file", sampleFile(t),
		"--manifest", manifest,
		"--namespace", "shop",
		"--workload", "checkout-api",
		"--container", "app",
		"--out", outPath,
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	patched, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched), "cpu: 576m") || strings.Contains(string(patched), "cpu: 2000m") {
		t.Errorf("patched manifest wrong:\n%s", patched)
	}
	if strings.Contains(string(patched), "memory: 128Mi") || !strings.Contains(string(patched), "memory: 0Mi") {
		t.Errorf("memory increase should not be patched:\n%s", patched)
	}
	s := out.String()
	for _, want := range []string{"Patched manifest:", "Right-size checkout-api", "$32", "Nothing was applied to the cluster"} {
		if !strings.Contains(s, want) {
			t.Errorf("pr output missing %q:\n%s", want, s)
		}
	}
}

func TestRun_PRErrorsWhenWorkloadNotRankable(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(manifest, []byte(prManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{
		"pr",
		"--from-file", sampleFile(t),
		"--manifest", manifest,
		"--workload", "nightly",
		"--container", "app",
		"--out", filepath.Join(dir, "patched.yaml"),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("want error for excluded/non-rankable workload")
	}
	if !strings.Contains(err.Error(), "no rankable workload") {
		t.Errorf("error = %v", err)
	}
}

func TestRun_PRErrorsWhenWorkloadNameAmbiguous(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	body := `[
 {"Namespace":"team-a","Name":"api","Replicas":1,"Current":{"CPU":2000,"Mem":536870912},"Usage":{"P95CPU":410,"P99CPU":480,"MaxMem":314572800},"Meta":{"Kind":"Deployment","HistoryDays":30}},
 {"Namespace":"team-b","Name":"api","Replicas":1,"Current":{"CPU":2000,"Mem":536870912},"Usage":{"P95CPU":410,"P99CPU":480,"MaxMem":314572800},"Meta":{"Kind":"Deployment","HistoryDays":30}}
]`
	if err := os.WriteFile(in, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(manifest, []byte(prManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{
		"pr",
		"--from-file", in,
		"--manifest", manifest,
		"--workload", "api",
		"--container", "app",
		"--out", filepath.Join(dir, "patched.yaml"),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--namespace") {
		t.Fatalf("want namespace ambiguity error, got %v", err)
	}
}

// An under-provisioned workload (usage > request) is not in the waste ranking,
// so pr refuses it up front rather than emitting a no-op or an increase.
func TestRun_PRRefusesUnderProvisioned(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	body := `[
 {"Namespace":"shop","Name":"tiny","Replicas":1,"Current":{"CPU":500,"Mem":134217728},"Usage":{"P95CPU":410,"P99CPU":480,"MaxMem":314572800},"Meta":{"Kind":"Deployment","HistoryDays":30}}
]`
	if err := os.WriteFile(in, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "deploy.yaml")
	manifestBody := strings.Replace(prManifest, "checkout-api", "tiny", 1)
	if err := os.WriteFile(manifest, []byte(manifestBody), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{
		"pr",
		"--from-file", in,
		"--manifest", manifest,
		"--namespace", "shop",
		"--workload", "tiny",
		"--container", "app",
		"--out", filepath.Join(dir, "patched.yaml"),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no rankable workload") {
		t.Fatalf("want under-provisioned workload refused as not-rankable, got %v", err)
	}
}

// The pr.Prepare multi-container guard must surface all the way to the CLI, so
// a sidecar pod can't get a misleading pod-level PR.
func TestRun_PRRefusesMultiContainer(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "deploy.yaml")
	multi := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
  namespace: shop
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 2000m
        - name: sidecar
          resources:
            requests:
              cpu: 500m
`
	if err := os.WriteFile(manifest, []byte(multi), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{
		"pr",
		"--from-file", sampleFile(t),
		"--manifest", manifest,
		"--namespace", "shop",
		"--workload", "checkout-api",
		"--container", "app",
		"--out", filepath.Join(dir, "patched.yaml"),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "containers") {
		t.Fatalf("want multi-container refusal, got %v", err)
	}
}

func TestRun_Version(t *testing.T) {
	for _, arg := range []string{"--version", "version"} {
		var out bytes.Buffer
		if err := Run([]string{arg}, &out); err != nil {
			t.Fatalf("Run(%q) errored: %v", arg, err)
		}
		if !strings.Contains(out.String(), "kubeloop") {
			t.Errorf("Run(%q) = %q, want it to name kubeloop + version", arg, out.String())
		}
	}
}
