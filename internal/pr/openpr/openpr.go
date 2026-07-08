// Package openpr composes the three halves of a real pull request: the offline
// patch (pr.Prepare), the local git work (gitrepo), and the GitHub API call
// (ghclient). It is the only path in kubeloop that produces an outward-facing
// side effect, so every step it takes is guarded and ordered deliberately.
//
// The cluster is never touched. The only writes are: one file in the user's
// own checkout, one branch, one commit, one push, one pull request.
package openpr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pr "github.com/kubeloop/kubeloop/internal/pr"
	"github.com/kubeloop/kubeloop/internal/pr/ghclient"
)

// Git is the local git half. gitrepo.Repo implements it.
type Git interface {
	IsClean(ctx context.Context) (bool, error)
	OriginRepo(ctx context.Context) (owner, name string, err error)
	CurrentBranch(ctx context.Context) (string, error)
	CreateBranch(ctx context.Context, branch string) error
	CommitFile(ctx context.Context, path, message string) error
	Push(ctx context.Context, branch, base string) error
}

// PRCreator is the GitHub half. ghclient.Client implements it.
type PRCreator interface {
	CreatePR(ctx context.Context, owner, repo string, p ghclient.PullRequest) (ghclient.Created, error)
}

// Request is one prepared proposal, ready to become a pull request.
type Request struct {
	Prepared pr.Prepared // from pr.Prepare: path, patched content, title, body
	RepoDir  string      // the git checkout containing Prepared.Path
	Ref      pr.Ref      // workload identity, used to name the branch
	Base     string      // base branch; "" → the currently checked-out branch
	Branch   string      // "" → derived from Ref + a hash of the patch
	DryRun   bool        // plan only: no branch, no commit, no push, no PR
}

// Result reports what happened. Branch is set once the branch was pushed, even
// if opening the pull request then failed — otherwise a user whose PR call 422s
// has no idea a branch is now sitting on their remote.
type Result struct {
	Branch   string
	Pushed   bool
	PRNumber int
	PRURL    string
	DryRun   bool
}

// Open runs the whole flow. Ordering is load-bearing:
//
//  1. refuse a dirty tree (gitrepo) — never sweep the user's work into our PR
//  2. resolve owner/repo *before* mutating anything, so a non-GitHub remote
//     fails before a branch exists rather than after
//  3. branch → write → commit → push
//  4. only then ask GitHub to open the PR (it 422s on an unpushed branch)
func Open(ctx context.Context, g Git, c PRCreator, req Request) (Result, error) {
	if req.RepoDir == "" {
		return Result{}, fmt.Errorf("repo directory is required")
	}
	// Resolve the destination before writing anything: a local-path remote or a
	// missing origin should fail while the tree is still untouched.
	owner, repo, err := g.OriginRepo(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("resolve origin: %w", err)
	}
	base := req.Base
	if base == "" {
		if base, err = g.CurrentBranch(ctx); err != nil {
			return Result{}, err
		}
	}
	branch := req.Branch
	if branch == "" {
		branch = BranchName(req.Ref, req.Prepared.Content)
	}
	if branch == base {
		return Result{}, fmt.Errorf("refusing to open a pull request from %q onto itself", base)
	}
	// Resolve the write target before touching git, so a traversing path can
	// never escape the checkout.
	abs, err := safeJoin(req.RepoDir, req.Prepared.Path)
	if err != nil {
		return Result{}, err
	}
	if req.DryRun {
		return Result{Branch: branch, DryRun: true}, nil
	}

	clean, err := g.IsClean(ctx)
	if err != nil {
		return Result{}, err
	}
	if !clean {
		return Result{}, fmt.Errorf("working tree has uncommitted changes: commit or stash them before kubeloop opens a PR")
	}
	if err := g.CreateBranch(ctx, branch); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(abs, req.Prepared.Content, 0o644); err != nil {
		return Result{}, err
	}
	if err := g.CommitFile(ctx, req.Prepared.Path, commitMessage(req.Prepared.Title)); err != nil {
		return Result{}, err
	}
	if err := g.Push(ctx, branch, base); err != nil {
		return Result{}, err
	}
	res := Result{Branch: branch, Pushed: true}

	created, err := c.CreatePR(ctx, owner, repo, ghclient.PullRequest{
		Title: req.Prepared.Title,
		Body:  req.Prepared.Body,
		Head:  branch,
		Base:  base,
	})
	if err != nil {
		// The branch is already on the remote. Say so, or the user goes looking
		// for a PR that doesn't exist and a branch they didn't know they pushed.
		return res, fmt.Errorf("branch %q was pushed, but opening the pull request failed: %w", branch, err)
	}
	res.PRNumber, res.PRURL = created.Number, created.HTMLURL
	return res, nil
}

// commitMessage takes the PR title's first line. The PR body carries the
// evidence; a commit subject should stay a subject.
func commitMessage(title string) string {
	if i := strings.IndexByte(title, '\n'); i >= 0 {
		title = title[:i]
	}
	if title = strings.TrimSpace(title); title == "" {
		return "kubeloop: rightsize workload requests"
	}
	return title
}

var unsafeRef = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// BranchName derives a stable branch from the workload identity plus a short
// hash of the patched content. Stable so re-running the same proposal collides
// with the existing branch (an honest "a PR may already be open") rather than
// littering the remote; content-keyed so a *different* proposal for the same
// workload gets its own branch.
func BranchName(ref pr.Ref, content []byte) string {
	sum := sha256.Sum256(content)
	part := func(s string) string { return strings.Trim(unsafeRef.ReplaceAllString(s, "-"), "-") }
	name := part(ref.Name)
	if ns := part(ref.Namespace); ns != "" {
		name = ns + "-" + name
	}
	if name == "" {
		name = "workload"
	}
	return fmt.Sprintf("kubeloop/rightsize-%s-%s", name, hex.EncodeToString(sum[:])[:7])
}

// safeJoin resolves rel inside root and refuses to escape it. Prepared.Path
// comes from a user-supplied --manifest argument; an absolute path or one
// containing ".." would otherwise let kubeloop write outside the checkout it
// was pointed at.
func safeJoin(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty manifest path")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("manifest path %q must be relative to the repository", rel)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(rootAbs, rel)
	inside, err := filepath.Rel(rootAbs, full)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("manifest path %q escapes the repository directory", rel)
	}
	return full, nil
}
