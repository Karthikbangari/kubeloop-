package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOrigin(t *testing.T) {
	ok := map[string][2]string{
		// The real remote. A cutset trim of ".git" would eat the trailing dash
		// and silently target a repository that does not exist.
		"https://github.com/Karthikbangari/kubeloop-.git": {"Karthikbangari", "kubeloop-"},
		"https://github.com/Karthikbangari/kubeloop-":     {"Karthikbangari", "kubeloop-"},
		"https://github.com/o/r.git":                      {"o", "r"},
		"https://github.com/o/r":                          {"o", "r"},
		"https://github.com/o/r/":                         {"o", "r"},
		"git@github.com:o/r.git":                          {"o", "r"},
		"git@github.com:o/r":                              {"o", "r"},
		"ssh://git@github.com/o/r.git":                    {"o", "r"},
		"  https://github.com/o/r.git  ":                  {"o", "r"},
		// A repo whose name merely contains "git" must not be truncated.
		"https://github.com/o/gitgit.git": {"o", "gitgit"},
	}
	for url, want := range ok {
		owner, name, err := ParseOrigin(url)
		if err != nil || owner != want[0] || name != want[1] {
			t.Errorf("ParseOrigin(%q) = %q/%q, %v; want %q/%q", url, owner, name, err, want[0], want[1])
		}
	}
	for _, bad := range []string{"", "not a url", "https://github.com/onlyowner", "git@github.com", "https://github.com/a/b/c"} {
		if _, _, err := ParseOrigin(bad); err == nil {
			t.Errorf("ParseOrigin(%q) should error", bad)
		}
	}
}

// fakeRepo records git invocations and replays canned stdout per subcommand.
type fakeRepo struct {
	calls [][]string
	out   map[string]string
}

func (f *fakeRepo) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return []byte(f.out[args[0]]), nil
}

func TestCreateBranch_RefusesDirtyTree(t *testing.T) {
	f := &fakeRepo{out: map[string]string{"status": " M some/other/file.go"}}
	err := (&Repo{Run: f.run}).CreateBranch(context.Background(), "kubeloop/rightsize")
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("branching on a dirty tree would sweep the user's work into the PR; got %v", err)
	}
	for _, c := range f.calls {
		if c[0] == "checkout" {
			t.Error("must not checkout when the tree is dirty")
		}
	}
}

func TestCommitFile_RefusesEmptyCommit(t *testing.T) {
	// `git add` succeeded but nothing is staged: the patch changed no bytes.
	f := &fakeRepo{out: map[string]string{"diff": ""}}
	err := (&Repo{Run: f.run}).CommitFile(context.Background(), "d.yaml", "msg")
	if err == nil || !strings.Contains(err.Error(), "changed no bytes") {
		t.Fatalf("a PR that changes nothing must never be opened; got %v", err)
	}
	for _, c := range f.calls {
		if c[0] == "commit" {
			t.Error("must not commit when nothing is staged")
		}
	}
}

func TestCommitFile_StagesOnlyTheTargetPath(t *testing.T) {
	f := &fakeRepo{out: map[string]string{"diff": "d.yaml"}}
	if err := (&Repo{Run: f.run}).CommitFile(context.Background(), "d.yaml", "msg"); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		joined := strings.Join(c, " ")
		if c[0] == "add" && joined != "add -- d.yaml" {
			t.Errorf("add must be path-scoped, got %q", joined)
		}
		if c[0] == "commit" && (strings.Contains(joined, " -a") || strings.Contains(joined, "--all")) {
			t.Errorf("commit must not sweep unrelated files, got %q", joined)
		}
	}
}

func TestPush_RefusesBaseBranch(t *testing.T) {
	f := &fakeRepo{out: map[string]string{}}
	err := (&Repo{Run: f.run}).Push(context.Background(), "main", "main")
	if err == nil || !strings.Contains(err.Error(), "refusing to push directly") {
		t.Fatalf("want refusal to push to base, got %v", err)
	}
	if len(f.calls) != 0 {
		t.Error("must not invoke git at all when refusing")
	}
}

func TestRefsRejectFlagInjection(t *testing.T) {
	f := &fakeRepo{out: map[string]string{"status": ""}}
	r := &Repo{Run: f.run}
	if err := r.CreateBranch(context.Background(), "--upload-pack=evil"); err == nil {
		t.Error("branch beginning with '-' must be refused")
	}
	if err := r.Push(context.Background(), "-x", "main"); err == nil {
		t.Error("branch beginning with '-' must be refused on push")
	}
	if err := r.CommitFile(context.Background(), "--output=/tmp/x", "m"); err == nil {
		t.Error("path beginning with '-' must be refused")
	}
}

// ---- Real git, real filesystem, local bare repo standing in for GitHub ----

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := ExecRunner(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// Exercises ExecRunner against real git: clone from a bare "origin", branch,
// commit a patched file, push, and verify the branch landed in origin. No
// network and no GitHub involved.
func TestRealGit_BranchCommitPush(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")

	git(t, root, "init", "--bare", "--initial-branch=main", origin)
	git(t, root, "clone", origin, work)
	git(t, work, "config", "user.email", "test@example.com")
	git(t, work, "config", "user.name", "Test")

	manifest := filepath.Join(work, "deploy.yaml")
	if err := os.WriteFile(manifest, []byte("cpu: 2000m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", "deploy.yaml")
	git(t, work, "commit", "-m", "seed")
	git(t, work, "push", "origin", "main")

	r := &Repo{Dir: work}

	if clean, err := r.IsClean(context.Background()); err != nil || !clean {
		t.Fatalf("seeded tree should be clean: %v %v", clean, err)
	}
	if b, err := r.CurrentBranch(context.Background()); err != nil || b != "main" {
		t.Fatalf("branch = %q, %v; want main", b, err)
	}
	// origin here is a local path, not a GitHub URL — OriginRepo must say so
	// rather than invent an owner/name that would 404 against the API.
	if _, _, err := r.OriginRepo(context.Background()); err == nil {
		t.Error("a local-path remote is not a GitHub repo; want an error")
	}

	const branch = "kubeloop/rightsize-checkout-api"
	if err := r.CreateBranch(context.Background(), branch); err != nil {
		t.Fatal(err)
	}
	// Simulate pr.Prepare writing the patched manifest.
	if err := os.WriteFile(manifest, []byte("cpu: 576m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.CommitFile(context.Background(), "deploy.yaml", "rightsize checkout-api"); err != nil {
		t.Fatal(err)
	}
	if err := r.Push(context.Background(), branch, "main"); err != nil {
		t.Fatal(err)
	}

	// The branch exists in origin and carries exactly the patched content.
	if got := git(t, origin, "rev-parse", "--verify", branch); got == "" {
		t.Fatal("branch missing from origin")
	}
	if got := git(t, origin, "show", branch+":deploy.yaml"); got != "cpu: 576m" {
		t.Errorf("origin content = %q, want the patched manifest", got)
	}
	// main is untouched: kubeloop never writes to the base branch.
	if got := git(t, origin, "show", "main:deploy.yaml"); got != "cpu: 2000m" {
		t.Errorf("base branch was modified: %q", got)
	}
}

// OriginRepo over a real `git remote get-url`, using the project's own remote
// (whose name ends in '-'). No network: get-url only reads local config.
func TestRealGit_OriginRepoResolvesGitHubRemote(t *testing.T) {
	work := t.TempDir()
	git(t, work, "init", "--initial-branch=main", ".")
	git(t, work, "remote", "add", "origin", "https://github.com/Karthikbangari/kubeloop-.git")

	owner, name, err := (&Repo{Dir: work}).OriginRepo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if owner != "Karthikbangari" || name != "kubeloop-" {
		t.Errorf("OriginRepo = %q/%q, want Karthikbangari/kubeloop- (trailing dash intact)", owner, name)
	}
}

// A dirty tree must be refused by the real implementation too, not just the fake.
func TestRealGit_RefusesDirtyTree(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	git(t, root, "init", "--initial-branch=main", work)
	git(t, work, "config", "user.email", "test@example.com")
	git(t, work, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", "a.txt")
	git(t, work, "commit", "-m", "seed")

	// Unrelated uncommitted work.
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("user edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := (&Repo{Dir: work}).CreateBranch(context.Background(), "kubeloop/x")
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("want refusal on a dirty tree, got %v", err)
	}
	if b := git(t, work, "rev-parse", "--abbrev-ref", "HEAD"); b != "main" {
		t.Errorf("still on %q; refusal must not have switched branches", b)
	}
}
