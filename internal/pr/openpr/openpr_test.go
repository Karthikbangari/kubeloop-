package openpr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pr "github.com/Karthikbangari/kubeloop-/internal/pr"
	"github.com/Karthikbangari/kubeloop-/internal/pr/ghclient"
)

type fakeGit struct {
	steps   []string
	dirty   bool
	origErr error
	pushErr error
	branch  string
	head    string // "" -> "main"
}

func (f *fakeGit) IsClean(context.Context) (bool, error) {
	f.steps = append(f.steps, "isclean")
	return !f.dirty, nil
}
func (f *fakeGit) OriginRepo(context.Context) (string, string, error) {
	f.steps = append(f.steps, "origin")
	if f.origErr != nil {
		return "", "", f.origErr
	}
	return "Karthikbangari", "kubeloop-", nil
}
func (f *fakeGit) CurrentBranch(context.Context) (string, error) {
	f.steps = append(f.steps, "current")
	if f.head != "" {
		return f.head, nil
	}
	return "main", nil
}
func (f *fakeGit) CreateBranch(_ context.Context, b string) error {
	f.steps = append(f.steps, "branch:"+b)
	f.branch = b
	return nil
}
func (f *fakeGit) CommitFile(_ context.Context, p, _ string) error {
	f.steps = append(f.steps, "commit:"+p)
	return nil
}
func (f *fakeGit) Push(_ context.Context, b, base string) error {
	f.steps = append(f.steps, "push:"+b+"->"+base)
	return f.pushErr
}

type fakeGH struct {
	got  ghclient.PullRequest
	repo string
	err  error
	n    int
}

func (f *fakeGH) CreatePR(_ context.Context, owner, repo string, p ghclient.PullRequest) (ghclient.Created, error) {
	f.n++
	f.got, f.repo = p, owner+"/"+repo
	if f.err != nil {
		return ghclient.Created{}, f.err
	}
	return ghclient.Created{Number: 7, HTMLURL: "https://github.com/" + owner + "/" + repo + "/pull/7"}, nil
}

func request(t *testing.T) Request {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte("cpu: 2000m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return Request{
		Prepared: pr.Prepared{Path: "deploy.yaml", Content: []byte("cpu: 576m\n"),
			Title: "kubeloop: rightsize checkout-api", Body: "saves $32/mo"},
		RepoDir: dir,
		Ref:     pr.Ref{Kind: "Deployment", Namespace: "shop", Name: "checkout-api"},
	}
}

func TestOpen_HappyPathOrderAndResult(t *testing.T) {
	g, c := &fakeGit{}, &fakeGH{}
	req := request(t)
	res, err := Open(context.Background(), g, c, req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pushed || res.PRNumber != 7 || !strings.HasSuffix(res.PRURL, "/pull/7") {
		t.Fatalf("result = %+v", res)
	}
	// origin is resolved before anything mutates; push precedes the PR call.
	joined := strings.Join(g.steps, " ")
	if !strings.HasPrefix(joined, "origin") {
		t.Errorf("origin must resolve before any mutation: %v", g.steps)
	}
	for _, want := range []string{"isclean", "branch:", "commit:deploy.yaml", "push:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing step %q in %v", want, g.steps)
		}
	}
	if !strings.Contains(joined, "push:"+res.Branch+"->main") {
		t.Errorf("must push the new branch onto base main: %v", g.steps)
	}
	// The patched content actually landed in the checkout.
	got, err := os.ReadFile(filepath.Join(req.RepoDir, "deploy.yaml"))
	if err != nil || string(got) != "cpu: 576m\n" {
		t.Errorf("file content = %q, %v", got, err)
	}
	// Repo name's trailing dash survives into the API call.
	if c.repo != "Karthikbangari/kubeloop-" {
		t.Errorf("repo = %q", c.repo)
	}
	if c.got.Head != res.Branch || c.got.Base != "main" || c.got.Title == "" || c.got.Body == "" {
		t.Errorf("pull request = %+v", c.got)
	}
}

// If the PR call fails after the push, the user must be told the branch exists —
// otherwise they hunt for a PR that isn't there and a branch they didn't know about.
func TestOpen_PushSucceededButPRFailed(t *testing.T) {
	g := &fakeGit{}
	c := &fakeGH{err: errors.New("422 validation failed")}
	res, err := Open(context.Background(), g, c, request(t))
	if err == nil {
		t.Fatal("want an error")
	}
	if !res.Pushed || res.Branch == "" {
		t.Errorf("result must report the pushed branch, got %+v", res)
	}
	if !strings.Contains(err.Error(), "was pushed") || !strings.Contains(err.Error(), "422") {
		t.Errorf("error must say the branch was pushed and why the PR failed: %v", err)
	}
}

// A non-GitHub or missing origin must fail before the tree is touched.
func TestOpen_BadOriginFailsBeforeMutating(t *testing.T) {
	g := &fakeGit{origErr: errors.New("unrecognized git remote")}
	req := request(t)
	if _, err := Open(context.Background(), g, &fakeGH{}, req); err == nil {
		t.Fatal("want an error")
	}
	for _, s := range g.steps {
		if strings.HasPrefix(s, "branch:") || strings.HasPrefix(s, "commit:") || strings.HasPrefix(s, "push:") {
			t.Errorf("mutated before resolving origin: %v", g.steps)
		}
	}
	// The original file is untouched.
	got, _ := os.ReadFile(filepath.Join(req.RepoDir, "deploy.yaml"))
	if string(got) != "cpu: 2000m\n" {
		t.Errorf("file was modified despite failure: %q", got)
	}
}

func TestOpen_RefusesDirtyTree(t *testing.T) {
	g := &fakeGit{dirty: true}
	c := &fakeGH{}
	req := request(t)
	if _, err := Open(context.Background(), g, c, req); err == nil ||
		!strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("want dirty-tree refusal, got %v", err)
	}
	if c.n != 0 {
		t.Error("must not call GitHub when refusing")
	}
	got, _ := os.ReadFile(filepath.Join(req.RepoDir, "deploy.yaml"))
	if string(got) != "cpu: 2000m\n" {
		t.Errorf("file was modified on a dirty tree: %q", got)
	}
}

// Dry-run must produce no branch, no commit, no push, no PR, and no file write.
func TestOpen_DryRunTouchesNothing(t *testing.T) {
	g, c := &fakeGit{}, &fakeGH{}
	req := request(t)
	req.DryRun = true
	res, err := Open(context.Background(), g, c, req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || res.Pushed || res.Branch == "" {
		t.Errorf("dry run result = %+v, want a planned branch and nothing done", res)
	}
	if c.n != 0 {
		t.Error("dry run must not call GitHub")
	}
	for _, s := range g.steps {
		if strings.HasPrefix(s, "branch:") || strings.HasPrefix(s, "commit:") || strings.HasPrefix(s, "push:") {
			t.Errorf("dry run mutated git: %v", g.steps)
		}
	}
	got, _ := os.ReadFile(filepath.Join(req.RepoDir, "deploy.yaml"))
	if string(got) != "cpu: 2000m\n" {
		t.Errorf("dry run wrote the file: %q", got)
	}
}

// Prepared.Path comes from a user-supplied --manifest. It must not escape the
// repository directory.
func TestOpen_RefusesPathEscape(t *testing.T) {
	for _, bad := range []string{"../outside.yaml", "a/../../outside.yaml", "/etc/passwd", ""} {
		g, c := &fakeGit{}, &fakeGH{}
		req := request(t)
		req.Prepared.Path = bad
		_, err := Open(context.Background(), g, c, req)
		if err == nil {
			t.Errorf("path %q must be refused", bad)
			continue
		}
		for _, s := range g.steps {
			if strings.HasPrefix(s, "branch:") {
				t.Errorf("path %q: mutated git before refusing", bad)
			}
		}
	}
}

func TestOpen_RefusesSymlinkTarget(t *testing.T) {
	g, c := &fakeGit{}, &fakeGH{}
	req := request(t)
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(outside, []byte("outside: original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(req.RepoDir, req.Prepared.Path)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}

	_, err := Open(context.Background(), g, c, req)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("want symlink refusal, got %v", err)
	}
	for _, s := range g.steps {
		if strings.HasPrefix(s, "branch:") || strings.HasPrefix(s, "commit:") || strings.HasPrefix(s, "push:") {
			t.Errorf("symlink target: mutated git before refusing: %v", g.steps)
		}
	}
	if c.n != 0 {
		t.Error("must not call GitHub when refusing a symlink target")
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != "outside: original\n" {
		t.Errorf("outside file = %q, %v", got, err)
	}
}

func TestOpen_RefusesBranchEqualsBase(t *testing.T) {
	g, c := &fakeGit{}, &fakeGH{}
	req := request(t)
	req.Branch, req.Base = "main", "main"
	if _, err := Open(context.Background(), g, c, req); err == nil ||
		!strings.Contains(err.Error(), "onto itself") {
		t.Fatalf("want refusal, got %v", err)
	}
}

func TestBranchName(t *testing.T) {
	ref := pr.Ref{Namespace: "shop", Name: "checkout-api"}
	a := BranchName(ref, []byte("cpu: 576m"))
	b := BranchName(ref, []byte("cpu: 576m"))
	c := BranchName(ref, []byte("cpu: 600m"))
	if a != b {
		t.Errorf("same proposal must give a stable branch: %q vs %q", a, b)
	}
	if a == c {
		t.Error("a different proposal must get its own branch")
	}
	if !strings.HasPrefix(a, "kubeloop/rightsize-shop-checkout-api-") {
		t.Errorf("branch = %q", a)
	}
	// Names that would be invalid refs are sanitized.
	weird := BranchName(pr.Ref{Namespace: "a b", Name: "x~y^z"}, []byte("x"))
	if strings.ContainsAny(weird, " ~^:?*[\\") {
		t.Errorf("unsanitized branch name: %q", weird)
	}
}

func TestCommitMessage(t *testing.T) {
	if got := commitMessage("subject\n\nbody"); got != "subject" {
		t.Errorf("got %q, want just the subject", got)
	}
	if got := commitMessage("   "); got == "" {
		t.Error("empty title must fall back to a real message")
	}
}

// A symlinked *intermediate directory* is the escape that a leaf-only Lstat
// check misses: repo/sub -> /outside means "sub/secret.yaml" is lexically
// inside the repo and its leaf is a genuine regular file. Only resolving the
// parent directory catches it. Regression for a hole found while verifying #82.
func TestOpen_RefusesSymlinkedParentDir(t *testing.T) {
	repo, outsideDir := t.TempDir(), t.TempDir()
	outside := filepath.Join(outsideDir, "secret.yaml")
	if err := os.WriteFile(outside, []byte("ORIGINAL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(repo, "sub")); err != nil {
		t.Fatal(err)
	}

	g, c := &fakeGit{}, &fakeGH{}
	_, err := Open(context.Background(), g, c, Request{
		Prepared: pr.Prepared{Path: filepath.Join("sub", "secret.yaml"),
			Content: []byte("PATCHED\n"), Title: "t", Body: "b"},
		RepoDir: repo,
		Ref:     pr.Ref{Namespace: "shop", Name: "checkout-api"},
	})
	if err == nil {
		t.Fatal("must refuse a path whose parent directory is a symlink")
	}
	if got, _ := os.ReadFile(outside); string(got) != "ORIGINAL\n" {
		t.Fatalf("SECURITY: wrote outside the repository: %q", got)
	}
	if c.n != 0 {
		t.Error("must not call GitHub")
	}
	for _, s := range g.steps {
		if strings.HasPrefix(s, "branch:") || strings.HasPrefix(s, "commit:") || strings.HasPrefix(s, "push:") {
			t.Errorf("mutated git before refusing: %v", g.steps)
		}
	}
}

// A legitimate manifest in a real subdirectory must still work — the symlink
// guard must not reject ordinary nested paths.
func TestOpen_AllowsRealSubdirectory(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "deploy", "prod"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("deploy", "prod", "app.yaml")
	if err := os.WriteFile(filepath.Join(repo, rel), []byte("cpu: 2000m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Open(context.Background(), &fakeGit{}, &fakeGH{}, Request{
		Prepared: pr.Prepared{Path: rel, Content: []byte("cpu: 576m\n"), Title: "t", Body: "b"},
		RepoDir:  repo,
		Ref:      pr.Ref{Namespace: "shop", Name: "checkout-api"},
	})
	if err != nil {
		t.Fatalf("a real nested path must be allowed: %v", err)
	}
	if !res.Pushed {
		t.Error("expected a completed flow")
	}
	got, _ := os.ReadFile(filepath.Join(repo, rel))
	if string(got) != "cpu: 576m\n" {
		t.Errorf("patch not applied: %q", got)
	}
}

// Nothing may be written inside .git. The path is lexically inside the repo and
// the leaf is a regular file, so the traversal and symlink guards both pass —
// but the patch is written before the commit, so a clobbered pre-commit hook
// would then execute, and .git/config controls where `push` sends data.
func TestOpen_RefusesWriteInsideGitDir(t *testing.T) {
	for _, rel := range []string{
		filepath.Join(".git", "hooks", "pre-commit"),
		filepath.Join(".git", "config"),
		filepath.Join("sub", ".git", "config"),
		filepath.Join(".GIT", "config"), // macOS/Windows are case-insensitive
	} {
		repo := t.TempDir()
		target := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("ORIGINAL\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		g, c := &fakeGit{}, &fakeGH{}
		_, err := Open(context.Background(), g, c, Request{
			Prepared: pr.Prepared{Path: rel, Content: []byte("PWNED\n"), Title: "t", Body: "b"},
			RepoDir:  repo, Ref: pr.Ref{Namespace: "shop", Name: "x"},
		})
		if err == nil || !strings.Contains(err.Error(), "git directory") {
			t.Errorf("%s: want a git-directory refusal, got %v", rel, err)
		}
		if got, _ := os.ReadFile(target); string(got) != "ORIGINAL\n" {
			t.Errorf("%s: CLOBBERED git internals: %q", rel, got)
		}
		if c.n != 0 {
			t.Errorf("%s: must not call GitHub", rel)
		}
		for _, s := range g.steps {
			if strings.HasPrefix(s, "branch:") || strings.HasPrefix(s, "commit:") || strings.HasPrefix(s, "push:") {
				t.Errorf("%s: mutated git before refusing: %v", rel, g.steps)
			}
		}
	}
}

// The .git guard must match a path segment, not a substring: these are ordinary
// files with legitimate names.
func TestOpen_AllowsGitLikeFilenames(t *testing.T) {
	for _, rel := range []string{".gitignore", "deploy/.gitkeep", "gitops/app.yaml"} {
		repo := t.TempDir()
		target := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("cpu: 2000m\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := Open(context.Background(), &fakeGit{}, &fakeGH{}, Request{
			Prepared: pr.Prepared{Path: filepath.FromSlash(rel), Content: []byte("cpu: 576m\n"), Title: "t", Body: "b"},
			RepoDir:  repo, Ref: pr.Ref{Namespace: "shop", Name: "x"},
		})
		if err != nil {
			t.Errorf("%s must be allowed, got %v", rel, err)
		}
	}
}

// A detached HEAD reports the literal "HEAD" as the current branch. Opening a
// PR onto "HEAD" would 422 only after the branch was already pushed.
func TestOpen_RefusesDetachedHead(t *testing.T) {
	g := &fakeGit{head: "HEAD"}
	c := &fakeGH{}
	_, err := Open(context.Background(), g, c, request(t))
	if err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("want a detached-HEAD refusal, got %v", err)
	}
	if c.n != 0 {
		t.Error("must not call GitHub")
	}
	for _, s := range g.steps {
		if strings.HasPrefix(s, "push:") {
			t.Errorf("pushed despite detached HEAD: %v", g.steps)
		}
	}
}

// The patched write must not widen a private manifest's permissions.
func TestOpen_PreservesFileMode(t *testing.T) {
	repo := t.TempDir()
	p := filepath.Join(repo, "deploy.yaml")
	if err := os.WriteFile(p, []byte("cpu: 2000m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), &fakeGit{}, &fakeGH{}, Request{
		Prepared: pr.Prepared{Path: "deploy.yaml", Content: []byte("cpu: 576m\n"), Title: "t", Body: "b"},
		RepoDir:  repo, Ref: pr.Ref{Namespace: "shop", Name: "x"},
	}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("manifest mode widened 0600 -> %v", fi.Mode().Perm())
	}
}
