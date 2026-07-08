//go:build unix

package openpr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pr "github.com/kubeloop/kubeloop/internal/pr"
)

func TestOpen_RefusesHardlinkedTarget(t *testing.T) {
	repo, outsideDir := t.TempDir(), t.TempDir()
	outside := filepath.Join(outsideDir, "outside.yaml")
	if err := os.WriteFile(outside, []byte("outside: original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(repo, "deploy.yaml")
	if err := os.Link(outside, target); err != nil {
		t.Skipf("hard links unsupported on this filesystem: %v", err)
	}

	g, c := &fakeGit{}, &fakeGH{}
	_, err := Open(context.Background(), g, c, Request{
		Prepared: pr.Prepared{Path: "deploy.yaml", Content: []byte("outside: patched\n"), Title: "t", Body: "b"},
		RepoDir:  repo,
		Ref:      pr.Ref{Namespace: "shop", Name: "checkout-api"},
	})
	if err == nil || !strings.Contains(err.Error(), "hard links") {
		t.Fatalf("want hard-link refusal, got %v", err)
	}
	for _, s := range g.steps {
		if strings.HasPrefix(s, "branch:") || strings.HasPrefix(s, "commit:") || strings.HasPrefix(s, "push:") {
			t.Errorf("hard-linked target: mutated git before refusing: %v", g.steps)
		}
	}
	if c.n != 0 {
		t.Error("must not call GitHub when refusing a hard-linked target")
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != "outside: original\n" {
		t.Errorf("outside file = %q, %v", got, err)
	}
}
