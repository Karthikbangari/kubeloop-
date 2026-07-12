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

	pr "github.com/Karthikbangari/kubeloop-/internal/pr"
	"github.com/Karthikbangari/kubeloop-/internal/pr/ghclient"
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
	// In a detached HEAD, `rev-parse --abbrev-ref HEAD` reports the literal
	// "HEAD". Opening a PR against a branch named HEAD would 422 from GitHub
	// after the branch was already pushed; say so before anything is mutated.
	if base == "HEAD" {
		return Result{}, fmt.Errorf("repository is in a detached HEAD state: pass --base with the target branch")
	}
	branch := req.Branch
	if branch == "" {
		branch = BranchName(req.Ref, req.Prepared.Content)
	}
	if branch == base {
		return Result{}, fmt.Errorf("refusing to open a pull request from %q onto itself", base)
	}
	// Resolve the write target before touching git, so a traversing path or
	// symlink can never escape the checkout.
	abs, err := patchTarget(req.RepoDir, req.Prepared.Path)
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

// patchTarget returns the regular file to overwrite.
//
// safeJoin only rules out *lexical* traversal, and os.WriteFile follows
// symlinks, so filesystem aliases need their own checks:
//
//   - the leaf is a symlink: "deploy.yaml" -> /etc/passwd. Caught by Lstat,
//     which does not follow the link.
//   - an intermediate directory is a symlink: "sub" -> /etc, so "sub/passwd"
//     is lexically inside the repo and its leaf is a genuine regular file.
//     Lstat on the leaf sees nothing wrong. Only resolving the parent
//     directory's symlinks and re-checking containment catches this.
//   - a hard-linked manifest is a regular file inside the repo and another
//     path outside it. Writing the repo path mutates both names.
//
// Both are refused before any branch/commit/push/PR happens, so a hostile
// manifest path cannot make kubeloop write outside the checkout it was given.
func patchTarget(root, rel string) (string, error) {
	full, err := safeJoin(root, rel)
	if err != nil {
		return "", err
	}
	// Never write inside .git. The path is lexically inside the repo and the
	// leaf is a regular file, so the checks below would happily allow it — but
	// the patch is written *before* the commit, so a clobbered
	// .git/hooks/pre-commit would then execute, and .git/config controls where
	// `push` sends data.
	//
	// The comparison is case-insensitive (macOS/Windows filesystems and git
	// itself treat ".GIT" as the git directory) AND ignores trailing dots and
	// spaces: Windows silently strips those when opening a file, so ".git." and
	// ".git " both resolve to ".git" while dodging a plain equality check — the
	// same normalization git had CVEs over. Residual: the 8.3 short name
	// "GIT~1" also aliases ".git" on Windows volumes with short-name generation
	// on; that is repo- and volume-specific and not handled here.
	for _, seg := range strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/") {
		if strings.EqualFold(strings.TrimRight(seg, ". "), ".git") {
			return "", fmt.Errorf("manifest path %q is inside the git directory; refusing to write there", rel)
		}
	}
	// Resolve the root too: on macOS a temp dir under /var is itself reached
	// through a /var -> /private/var symlink, so comparing an unresolved root
	// against a resolved child would reject every legitimate path. Absolute
	// first, since --repo-dir is often ".".
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve repository directory: %w", err)
	}
	parentReal, err := filepath.EvalSymlinks(filepath.Dir(full))
	if err != nil {
		return "", fmt.Errorf("resolve manifest directory for %q: %w", rel, err)
	}
	inside, err := filepath.Rel(rootReal, parentReal)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("manifest path %q resolves outside the repository (a parent directory is a symlink)", rel)
	}
	info, err := os.Lstat(full)
	if err != nil {
		return "", fmt.Errorf("stat manifest path %q: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("manifest path %q is a symlink; refusing to write through it", rel)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("manifest path %q is not a regular file", rel)
	}
	// Fail closed: if the link count cannot be determined, refuse. "I could not
	// tell" must never be treated as "it is safe".
	linked, err := hasMultipleHardLinks(full, info)
	if err != nil {
		return "", fmt.Errorf("manifest path %q: %w", rel, err)
	}
	if linked {
		return "", fmt.Errorf("manifest path %q has multiple hard links; refusing to overwrite it", rel)
	}
	return full, nil
}
