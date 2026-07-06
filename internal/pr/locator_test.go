package pr

import (
	"strings"
	"testing"
)

func deploy(name, ns string) []byte {
	return []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: " + name + "\n  namespace: " + ns + "\n")
}

func files() []File {
	return []File{
		{Path: "helm/checkout.yaml", Content: deploy("checkout-api", "shop")},
		{Path: "helm/reco.yaml", Content: deploy("recommendations", "shop")},
		{Path: "README.md", Content: []byte("# not yaml at all: {")},
	}
}

func TestFindSource_ReturnsMatchingFile(t *testing.T) {
	f, err := FindSource(files(), Ref{Kind: "Deployment", Name: "checkout-api"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Path != "helm/checkout.yaml" {
		t.Errorf("path = %q, want helm/checkout.yaml", f.Path)
	}
}

func TestFindSource_NoMatchErrors(t *testing.T) {
	if _, err := FindSource(files(), Ref{Kind: "Deployment", Name: "missing"}); err == nil {
		t.Error("want error when no manifest matches")
	}
}

func TestFindSource_AmbiguousErrors(t *testing.T) {
	dup := append(files(), File{Path: "dup/checkout.yaml", Content: deploy("checkout-api", "shop")})
	_, err := FindSource(dup, Ref{Kind: "Deployment", Name: "checkout-api"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("want ambiguous error, got %v", err)
	}
}

func TestFindSource_NamespaceDisambiguates(t *testing.T) {
	fs := []File{
		{Path: "a/api.yaml", Content: deploy("api", "team-a")},
		{Path: "b/api.yaml", Content: deploy("api", "team-b")},
	}
	f, err := FindSource(fs, Ref{Kind: "Deployment", Name: "api", Namespace: "team-b"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Path != "b/api.yaml" {
		t.Errorf("path = %q, want b/api.yaml", f.Path)
	}
}
