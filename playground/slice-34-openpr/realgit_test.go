package openpr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pr "github.com/kubeloop/kubeloop/internal/pr"
	gitrepo "github.com/kubeloop/kubeloop/playground/slice-32-gitrepo"
)

// realGit is the real gitrepo.Repo with only OriginRepo stubbed: a local bare
// repo standing in for GitHub has a filesystem path, which ParseOrigin rightly
// refuses. Everything else — clean check, branch, commit, push — is real git.
type realGit struct{ *gitrepo.Repo }

func (realGit) OriginRepo(context.Context) (string, string, error) {
	return "Karthikbangari", "kubeloop-", nil
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitrepo.ExecRunner(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// The whole local flow against real git and a real filesystem, with only the
// GitHub POST faked: branch → write the patched manifest → commit → push, and
// then verify the branch landed in origin with the patch, base untouched.
func TestOpen_RealGitEndToEnd(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")

	git(t, root, "init", "--bare", "--initial-branch=main", origin)
	git(t, root, "clone", origin, work)
	git(t, work, "config", "user.email", "test@example.com")
	git(t, work, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "deploy.yaml"), []byte("cpu: 2000m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", "deploy.yaml")
	git(t, work, "commit", "-m", "seed")
	git(t, work, "push", "origin", "main")

	g := realGit{&gitrepo.Repo{Dir: work}}
	gh := &fakeGH{}
	req := Request{
		Prepared: pr.Prepared{
			Path: "deploy.yaml", Content: []byte("cpu: 576m\n"),
			Title: "kubeloop: rightsize checkout-api (saves $32/mo)",
			Body:  "Evidence: P99 480m ×1.2 = 576m.",
		},
		RepoDir: work,
		Ref:     pr.Ref{Kind: "Deployment", Namespace: "shop", Name: "checkout-api"},
	}

	res, err := Open(context.Background(), g, gh, req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pushed || res.PRNumber != 7 {
		t.Fatalf("result = %+v", res)
	}

	// The branch really exists in origin, carrying exactly the patched content.
	if git(t, origin, "rev-parse", "--verify", res.Branch) == "" {
		t.Fatal("branch missing from origin")
	}
	if got := git(t, origin, "show", res.Branch+":deploy.yaml"); got != "cpu: 576m" {
		t.Errorf("origin branch content = %q, want the patched manifest", got)
	}
	// main is byte-for-byte untouched: kubeloop never writes to the base branch.
	if got := git(t, origin, "show", "main:deploy.yaml"); got != "cpu: 2000m" {
		t.Errorf("base branch was modified: %q", got)
	}
	// The commit subject is the PR title's first line, not the whole body.
	if subj := git(t, work, "log", "-1", "--pretty=%s"); subj != req.Prepared.Title {
		t.Errorf("commit subject = %q", subj)
	}
	// Exactly one file changed in that commit.
	if files := git(t, work, "show", "--name-only", "--pretty=format:", "HEAD"); files != "deploy.yaml" {
		t.Errorf("commit touched %q, want only deploy.yaml", files)
	}
	if gh.got.Head != res.Branch || gh.got.Base != "main" {
		t.Errorf("pull request = %+v", gh.got)
	}
}

// A dirty tree must be refused by the real implementation, and nothing pushed.
func TestOpen_RealGitRefusesDirtyTree(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	git(t, root, "init", "--bare", "--initial-branch=main", origin)
	git(t, root, "clone", origin, work)
	git(t, work, "config", "user.email", "t@e.com")
	git(t, work, "config", "user.name", "T")
	os.WriteFile(filepath.Join(work, "deploy.yaml"), []byte("cpu: 2000m\n"), 0o600)
	git(t, work, "add", "deploy.yaml")
	git(t, work, "commit", "-m", "seed")
	git(t, work, "push", "origin", "main")

	// The user has unrelated uncommitted work.
	os.WriteFile(filepath.Join(work, "notes.txt"), []byte("wip"), 0o600)
	git(t, work, "add", "notes.txt")

	gh := &fakeGH{}
	_, err := Open(context.Background(), realGit{&gitrepo.Repo{Dir: work}}, gh, Request{
		Prepared: pr.Prepared{Path: "deploy.yaml", Content: []byte("cpu: 576m\n"), Title: "t", Body: "b"},
		RepoDir:  work,
		Ref:      pr.Ref{Namespace: "shop", Name: "checkout-api"},
	})
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("want dirty-tree refusal, got %v", err)
	}
	if gh.n != 0 {
		t.Error("must not call GitHub")
	}
	if b := git(t, work, "rev-parse", "--abbrev-ref", "HEAD"); b != "main" {
		t.Errorf("refusal must not switch branches, on %q", b)
	}
	if out := git(t, origin, "branch", "--list"); strings.Contains(out, "kubeloop/") {
		t.Errorf("nothing should have been pushed, origin has: %q", out)
	}
}
