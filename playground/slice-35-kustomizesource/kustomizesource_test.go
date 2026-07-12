package kustomizesource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

const deployYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
spec:
  replicas: 2
`

func TestFindSource_SimpleResourcesList(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "kustomization.yaml", "resources:\n  - deployment.yaml\n  - service.yaml\n")
	write(t, dir, "deployment.yaml", deployYAML)
	write(t, dir, "service.yaml", "apiVersion: v1\nkind: Service\nmetadata:\n  name: checkout-api\n")

	src, err := FindSource(dir, Ref{Kind: "Deployment", Name: "checkout-api"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(src.Path) != "deployment.yaml" || !strings.Contains(string(src.Content), "replicas: 2") {
		t.Errorf("located %q, want deployment.yaml with content", src.Path)
	}
}

// The rendered name carries the overlay's prefix/suffix; the source file does
// not. Mapping back must strip them.
func TestFindSource_StripsNamePrefixAndSuffix(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "kustomization.yaml", "namePrefix: prod-\nnameSuffix: -v2\nresources:\n  - deployment.yaml\n")
	write(t, dir, "deployment.yaml", deployYAML) // source name is bare "checkout-api"

	src, err := FindSource(dir, Ref{Kind: "Deployment", Name: "prod-checkout-api-v2"})
	if err != nil {
		t.Fatalf("prefix/suffix not stripped: %v", err)
	}
	if filepath.Base(src.Path) != "deployment.yaml" {
		t.Errorf("located %q", src.Path)
	}
}

// An overlay whose resources: points at a base directory must be followed.
func TestFindSource_DescendsIntoBaseDir(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "overlay/kustomization.yaml", "namePrefix: prod-\nresources:\n  - ../base\n")
	write(t, dir, "base/kustomization.yaml", "resources:\n  - deployment.yaml\n")
	write(t, dir, "base/deployment.yaml", deployYAML)

	src, err := FindSource(filepath.Join(dir, "overlay"), Ref{Kind: "Deployment", Name: "prod-checkout-api"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(src.Path) != "deployment.yaml" || !strings.Contains(src.Path, "base") {
		t.Errorf("located %q, want base/deployment.yaml", src.Path)
	}
}

// A patch can rename a resource, so a name-based map would be wrong. Refuse.
func TestFindSource_RefusesPatchesItCannotMap(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "kustomization.yaml", "resources:\n  - deployment.yaml\npatches:\n  - path: rename.yaml\n")
	write(t, dir, "deployment.yaml", deployYAML)

	_, err := FindSource(dir, Ref{Kind: "Deployment", Name: "checkout-api"})
	if err == nil || !strings.Contains(err.Error(), "patches") {
		t.Fatalf("want a refusal naming patches, got %v", err)
	}
}

func TestFindSource_RefusesGenerators(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "kustomization.yaml", "resources:\n  - deployment.yaml\nconfigMapGenerator:\n  - name: cfg\n")
	write(t, dir, "deployment.yaml", deployYAML)
	if _, err := FindSource(dir, Ref{Kind: "Deployment", Name: "checkout-api"}); err == nil || !strings.Contains(err.Error(), "generators") {
		t.Fatalf("want a generators refusal, got %v", err)
	}
}

func TestFindSource_MultiDocFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "kustomization.yaml", "resources:\n  - all.yaml\n")
	write(t, dir, "all.yaml", "apiVersion: v1\nkind: Service\nmetadata:\n  name: checkout-api\n---\n"+deployYAML)

	src, err := FindSource(dir, Ref{Kind: "Deployment", Name: "checkout-api"})
	if err != nil || filepath.Base(src.Path) != "all.yaml" {
		t.Fatalf("want all.yaml, got %q err=%v", src.Path, err)
	}
}

func TestFindSource_NotKustomizeDir(t *testing.T) {
	if _, err := FindSource(t.TempDir(), Ref{Kind: "Deployment", Name: "x"}); err == nil ||
		!strings.Contains(err.Error(), "no kustomization") {
		t.Fatalf("want not-a-kustomize-dir error, got %v", err)
	}
}

func TestFindSource_NoMatch(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "kustomization.yaml", "resources:\n  - deployment.yaml\n")
	write(t, dir, "deployment.yaml", deployYAML)
	if _, err := FindSource(dir, Ref{Kind: "Deployment", Name: "not-here"}); err == nil ||
		!strings.Contains(err.Error(), "no source manifest") {
		t.Fatalf("want a no-match error, got %v", err)
	}
}

// Two resource files defining the same workload is ambiguous — refuse, like the
// raw locator does.
func TestFindSource_AmbiguousRefuses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "kustomization.yaml", "resources:\n  - a.yaml\n  - b.yaml\n")
	write(t, dir, "a.yaml", deployYAML)
	write(t, dir, "b.yaml", deployYAML)
	if _, err := FindSource(dir, Ref{Kind: "Deployment", Name: "checkout-api"}); err == nil ||
		!strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("want ambiguity refusal, got %v", err)
	}
}

func TestIsKustomizeDir(t *testing.T) {
	dir := t.TempDir()
	if IsKustomizeDir(dir) {
		t.Error("empty dir is not a kustomize dir")
	}
	write(t, dir, "kustomization.yml", "resources: []\n")
	if !IsKustomizeDir(dir) {
		t.Error("dir with kustomization.yml should be detected")
	}
}
