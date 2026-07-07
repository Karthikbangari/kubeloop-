// Command kubeloop is the read-only Kubernetes rightsizing CLI. It scans
// workloads, ranks their waste in dollars, and prints a report (text or JSON).
// Read-only always: the only write path in the product is a human-reviewed PR,
// which this binary never performs. The live read-layer (kubeconfig +
// Prometheus) is a later slice; today workloads come from --from-file.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	pr "github.com/kubeloop/kubeloop/internal/pr"
	"github.com/kubeloop/kubeloop/internal/pr/quantity"
	rp "github.com/kubeloop/kubeloop/internal/reporting"
	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
	"github.com/kubeloop/kubeloop/internal/savings"
	"github.com/kubeloop/kubeloop/internal/scan"
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
	fromFile := fs.String("from-file", "", "read workloads from a JSON file (offline; cluster read-layer TBD)")
	pricingFile := fs.String("pricing-file", "", "override list prices from a pricing.json file")
	perRequest := fs.Bool("per-request", false, "cluster bills per pod request (e.g. GKE Autopilot): savings are immediate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromFile == "" {
		return fmt.Errorf("--from-file required until the cluster read-layer lands")
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromFile == "" || *workload == "" || *container == "" || *outFile == "" || len(manifests) == 0 {
		return fmt.Errorf("pr requires --from-file, --manifest, --workload, --container, and --out")
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
	prepared, err := pr.Prepare(pr.Request{
		Files:       files,
		Ref:         pr.Ref{Kind: *kind, Name: row.Name, Namespace: row.Namespace},
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
	if err := os.WriteFile(*outFile, prepared.Content, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Patched manifest: %s -> %s\n\n%s\n\n%s", prepared.Path, *outFile, prepared.Title, prepared.Body)
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
