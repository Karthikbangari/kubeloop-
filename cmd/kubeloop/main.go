// Command kubeloop is the read-only Kubernetes rightsizing CLI. It scans
// workloads, ranks their waste in dollars, and prints a report (text or JSON).
// Read-only always: the only write path in the product is a human-reviewed PR,
// which this binary never performs. Workloads come from exactly one source:
//
//	--from-cluster    the live cluster (kubectl `get`, read-only) + Prometheus
//	--from-manifests  a directory of Kubernetes manifests + a usage export
//	--from-file       pre-assembled scan inputs (offline)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	pr "github.com/Karthikbangari/kubeloop-/internal/pr"
	"github.com/Karthikbangari/kubeloop-/internal/pr/ghclient"
	"github.com/Karthikbangari/kubeloop-/internal/pr/gitrepo"
	"github.com/Karthikbangari/kubeloop-/internal/pr/openpr"
	"github.com/Karthikbangari/kubeloop-/internal/pr/quantity"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/clustersource"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/dirsource"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/kubeclient"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/promclient"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/promql"
	rp "github.com/Karthikbangari/kubeloop-/internal/reporting"
	rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"
	"github.com/Karthikbangari/kubeloop-/internal/savings"
	"github.com/Karthikbangari/kubeloop-/internal/scan"
)

// version is stamped at release time via -ldflags "-X main.version=..."
// (see .goreleaser.yaml); "dev" for local/`go build` binaries.
var version = "dev"

func main() {
	if err := Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "kubeloop:", err)
		os.Exit(1)
	}
}

// Run dispatches subcommands and writes output. Split from main so it's
// testable with an in-memory writer. "scan" is the default, so both
// `kubeloop --from-file X` and the explicit `kubeloop scan --from-file X`
// (as the README/plan show) work.
func Run(args []string, out io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-version", "version":
			fmt.Fprintf(out, "kubeloop %s\n", version)
			return nil
		case "pr":
			return runPR(args[1:], out)
		case "scan":
			return runScan(args[1:], out)
		}
	}
	return runScan(args, out)
}

func runScan(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("kubeloop", flag.ContinueOnError)
	fs.SetOutput(out)
	jsonOut := fs.Bool("json", false, "machine-readable JSON output")
	cloud := fs.String("cloud", "aws", "cloud for list pricing (aws|gcp|azure)")
	var src source
	fs.StringVar(&src.fromFile, "from-file", "", "read workloads from a JSON file (offline)")
	fs.StringVar(&src.fromManifests, "from-manifests", "", "read workloads from a directory of Kubernetes manifest JSON files (offline)")
	fs.StringVar(&src.usageFile, "usage-file", "", "JSON map of \"namespace/name\" -> {P95CPU,P99CPU,MaxMem,HistoryDays}, paired with --from-manifests")
	fs.BoolVar(&src.fromCluster, "from-cluster", false, "read workloads from the live cluster via kubectl, and usage from Prometheus (read-only)")
	fs.StringVar(&src.prometheus, "prometheus", "", "Prometheus base URL (e.g. http://localhost:9090), required with --from-cluster")
	fs.StringVar(&src.namespace, "namespace", "", "limit --from-cluster to one namespace (default: all namespaces)")
	fs.StringVar(&src.kubeContext, "context", "", "kubeconfig context for --from-cluster (default: current context)")
	pricingFile := fs.String("pricing-file", "", "override list prices from a pricing.json file")
	perRequest := fs.Bool("per-request", false, "cluster bills per pod request (e.g. GKE Autopilot): savings are immediate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	inputs, err := loadWorkloads(ctx, src)
	if err != nil {
		return err
	}
	price, err := rp.LoadPrice(*cloud, *pricingFile)
	if err != nil {
		return err
	}
	mode := savings.NodeBased
	if *perRequest {
		mode = savings.PerRequest
	}
	report := scan.Scan(inputs, rs.Percentile{}, price, mode)
	if *jsonOut {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(toJSON(report))
	}
	fmt.Fprint(out, scan.Render(report))
	return nil
}

type manifestFiles []string

func (m *manifestFiles) String() string { return strings.Join(*m, ",") }

func (m *manifestFiles) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func runPR(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("kubeloop pr", flag.ContinueOnError)
	fs.SetOutput(out)
	fromFile := fs.String("from-file", "", "read workloads from a JSON file (offline; cluster read-layer TBD)")
	cloud := fs.String("cloud", "aws", "cloud for list pricing (aws|gcp|azure)")
	pricingFile := fs.String("pricing-file", "", "override list prices from a pricing.json file")
	perRequest := fs.Bool("per-request", false, "cluster bills per pod request (e.g. GKE Autopilot): savings are immediate")
	workload := fs.String("workload", "", "workload name to prepare a PR for")
	namespace := fs.String("namespace", "", "workload namespace (optional unless needed to disambiguate)")
	kind := fs.String("kind", "Deployment", "workload kind")
	container := fs.String("container", "", "container name to patch")
	outFile := fs.String("out", "", "write patched manifest to this path")
	var manifests manifestFiles
	fs.Var(&manifests, "manifest", "manifest file to search; repeat for multiple files")
	open := fs.Bool("open", false, "open a pull request on GitHub (requires GITHUB_TOKEN with `repo` scope)")
	repoDir := fs.String("repo-dir", ".", "git checkout containing the manifest, used with --open")
	base := fs.String("base", "", "base branch for the pull request (default: the checked-out branch)")
	dryRun := fs.Bool("dry-run", false, "with --open: print what would happen and change nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromFile == "" || *workload == "" || *container == "" || len(manifests) == 0 {
		return fmt.Errorf("pr requires --from-file, --manifest, --workload, --container, and one of --out or --open")
	}
	// --out writes the patched manifest locally; --open branches, commits,
	// pushes, and opens a pull request. Requiring exactly one keeps the side
	// effects of the command unambiguous.
	switch {
	case *outFile == "" && !*open:
		return fmt.Errorf("pr requires one of --out (write the patched manifest) or --open (open a pull request)")
	case *outFile != "" && *open:
		return fmt.Errorf("use either --out or --open, not both")
	case *dryRun && !*open:
		return fmt.Errorf("--dry-run applies to --open")
	}
	inputs, err := loadInputs(*fromFile)
	if err != nil {
		return err
	}
	price, err := rp.LoadPrice(*cloud, *pricingFile)
	if err != nil {
		return err
	}
	mode := savings.NodeBased
	if *perRequest {
		mode = savings.PerRequest
	}
	report := scan.Scan(inputs, rs.Percentile{}, price, mode)
	row, err := pr.FindRow(report.Rows, *namespace, *workload)
	if err != nil {
		return err
	}
	reduceCPU, reduceMem := pr.Reductions(row.Current, row.Proposed)
	if reduceCPU == 0 && reduceMem == 0 {
		return fmt.Errorf("workload %q has no request reductions to patch", *workload)
	}
	proposedCPU, proposedMem := "", ""
	if reduceCPU > 0 {
		proposedCPU = quantity.CPU(reduceCPU)
	}
	if reduceMem > 0 {
		proposedMem = quantity.Mem(reduceMem)
	}
	files, err := loadManifestFiles(manifests)
	if err != nil {
		return err
	}
	ref := pr.Ref{Kind: *kind, Name: row.Name, Namespace: row.Namespace}
	prepared, err := pr.Prepare(pr.Request{
		Files:       files,
		Ref:         ref,
		Container:   *container,
		CurrentCPU:  quantity.CPU(row.Current.CPU),
		ProposedCPU: proposedCPU,
		CurrentMem:  quantity.Mem(row.Current.Mem),
		ProposedMem: proposedMem,
		MonthlyUSD:  row.MonthlyWaste,
		Confidence:  row.Confidence,
		Realization: savings.Realization(report.Mode),
		Caution:     row.Caution,
	})
	if err != nil {
		return err
	}
	if *open {
		// openpr writes Prepared.Path inside --repo-dir, so the path must be
		// repo-relative. Users naturally pass absolute or CWD-relative manifest
		// paths, so relativize here rather than making them do it.
		rel, err := repoRelative(*repoDir, prepared.Path)
		if err != nil {
			return err
		}
		prepared.Path = rel
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return openPullRequest(ctx, out, prepared, ref, *repoDir, *base, *dryRun)
	}
	if err := os.WriteFile(*outFile, prepared.Content, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Patched manifest: %s -> %s\n\n%s\n\n%s", prepared.Path, *outFile, prepared.Title, prepared.Body)
	return nil
}

// repoRelative expresses a manifest path relative to the repo checkout. A
// manifest outside the checkout is refused by name, rather than deferring to
// openpr's generic "escapes the repository directory".
func repoRelative(repoDir, manifest string) (string, error) {
	root, err := filepath.Abs(repoDir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(manifest)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("manifest %s is not inside --repo-dir %s", manifest, repoDir)
	}
	return rel, nil
}

// openPullRequest branches, commits the patched manifest, pushes, and opens a
// pull request. This is the only outward-facing side effect kubeloop has; the
// cluster is never touched. --dry-run performs none of it.
func openPullRequest(ctx context.Context, out io.Writer, prepared pr.Prepared, ref pr.Ref, repoDir, base string, dryRun bool) error {
	token := ghclient.TokenFromEnv()
	if token == "" && !dryRun {
		return fmt.Errorf("no GitHub token: set GITHUB_TOKEN (needs `repo` scope), or use --dry-run to see the plan")
	}
	res, err := openpr.Open(ctx,
		&gitrepo.Repo{Dir: repoDir},
		ghclient.New(token, &http.Client{Timeout: 30 * time.Second}),
		openpr.Request{Prepared: prepared, RepoDir: repoDir, Ref: ref, Base: base, DryRun: dryRun},
	)
	if err != nil {
		return err
	}
	if res.DryRun {
		fmt.Fprintf(out, "Dry run — nothing was changed.\n\n"+
			"Would patch:  %s\nWould branch: %s\nWould open:   %s\n",
			prepared.Path, res.Branch, prepared.Title)
		return nil
	}
	fmt.Fprintf(out, "Opened pull request #%d\n  %s\n  branch: %s\n", res.PRNumber, res.PRURL, res.Branch)
	return nil
}

func loadManifestFiles(paths []string) ([]pr.File, error) {
	files := make([]pr.File, len(paths))
	for i, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		files[i] = pr.File{Path: p, Content: b}
	}
	return files, nil
}

func loadInputs(path string) ([]scan.Input, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Reject unknown fields so a typo ("MaxMemory" for "MaxMem") fails loudly
	// instead of silently zeroing usage — which would then exclude the workload
	// with a misleading "metrics gap" reason and corrupt the scan.
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var inputs []scan.Input
	if err := dec.Decode(&inputs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("parse %s: trailing JSON data", path)
	}
	return inputs, nil
}

// source is the workload input selection: exactly one of --from-file,
// --from-manifests, or --from-cluster.
type source struct {
	fromFile      string
	fromManifests string
	usageFile     string
	fromCluster   bool
	prometheus    string
	namespace     string
	kubeContext   string
}

// loadWorkloads selects the input source. Exactly one is required, and the
// flags that belong to one source are rejected on another rather than silently
// ignored — a user who passes --usage-file with --from-cluster has a wrong
// mental model, and a quietly-dropped flag would confirm it.
func loadWorkloads(ctx context.Context, s source) ([]scan.Input, error) {
	n := 0
	for _, chosen := range []bool{s.fromFile != "", s.fromManifests != "", s.fromCluster} {
		if chosen {
			n++
		}
	}
	if n > 1 {
		return nil, fmt.Errorf("use exactly one of --from-file, --from-manifests, or --from-cluster")
	}
	switch {
	case s.fromCluster:
		if s.usageFile != "" {
			return nil, fmt.Errorf("--usage-file applies to --from-manifests; --from-cluster reads usage from Prometheus")
		}
		return loadCluster(ctx, s)
	case s.fromManifests != "":
		if err := s.rejectClusterFlags("--from-manifests"); err != nil {
			return nil, err
		}
		return loadManifestDir(s.fromManifests, s.usageFile)
	case s.fromFile != "":
		if s.usageFile != "" {
			return nil, fmt.Errorf("--usage-file applies to --from-manifests, not --from-file")
		}
		if err := s.rejectClusterFlags("--from-file"); err != nil {
			return nil, err
		}
		return loadInputs(s.fromFile)
	default:
		return nil, fmt.Errorf("one of --from-file, --from-manifests, or --from-cluster is required")
	}
}

// rejectClusterFlags refuses cluster-only flags on an offline source.
func (s source) rejectClusterFlags(mode string) error {
	for flag, set := range map[string]bool{
		"--prometheus": s.prometheus != "",
		"--namespace":  s.namespace != "",
		"--context":    s.kubeContext != "",
	} {
		if set {
			return fmt.Errorf("%s applies to --from-cluster, not %s", flag, mode)
		}
	}
	return nil
}

// loadCluster reads workloads from the live cluster (via kubectl) and their
// usage from Prometheus. Read-only: kubectl is only ever invoked with `get`.
//
// A workload Prometheus has no data for is excluded with a printed reason, never
// sized on no data. A Prometheus that errors aborts the scan rather than looking
// like a cluster with no waste.
func loadCluster(ctx context.Context, s source) ([]scan.Input, error) {
	if s.prometheus == "" {
		return nil, fmt.Errorf("--prometheus is required with --from-cluster (e.g. --prometheus http://localhost:9090)")
	}
	kc := &kubeclient.Client{Namespace: s.namespace, Context: s.kubeContext}
	workloads, err := kc.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(workloads) == 0 {
		return nil, fmt.Errorf("no Deployments or StatefulSets found (namespace %q)", s.namespace)
	}
	pc := promclient.New(s.prometheus, &http.Client{Timeout: 30 * time.Second})
	return clustersource.Collect(ctx, workloads, pc, promql.Defaults())
}

// loadManifestDir reads every *.json manifest in dir and attaches usage looked
// up by "namespace/name" from usageFile (workloads with no usage entry are
// excluded by the scanner's missing-signal rule, never sized on no data).
func loadManifestDir(dir, usageFile string) ([]scan.Input, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var manifests [][]byte
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, b)
	}
	if len(manifests) == 0 {
		return nil, fmt.Errorf("no .json manifests found in %s", dir)
	}
	usage, err := loadUsage(usageFile)
	if err != nil {
		return nil, err
	}
	return dirsource.Assemble(manifests, usage)
}

// loadUsage parses the "namespace/name" -> usage map. Unknown fields fail loudly
// (a typo like "MaxMemory" would otherwise zero usage and silently exclude the
// workload). An empty path yields no usage — every workload is then reported as
// excluded with a reason rather than sized.
func loadUsage(path string) (map[string]dirsource.Usage, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var usage map[string]dirsource.Usage
	if err := dec.Decode(&usage); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("parse %s: trailing JSON data", path)
	}
	return usage, nil
}
