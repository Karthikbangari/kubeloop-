package openpr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pr "github.com/kubeloop/kubeloop/internal/pr"
	ghclient "github.com/kubeloop/kubeloop/playground/slice-33-ghclient"
)

type fakeGit struct {
	steps   []string
	dirty   bool
	origErr error
	pushErr error
	branch  string
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
