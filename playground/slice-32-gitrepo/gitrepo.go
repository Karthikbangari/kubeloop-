// Package gitrepo performs the local git half of opening a pull request:
// resolve the origin, branch, commit one patched file, push. The GitHub API
// half is ghclient's job.
//
// Shells out to `git` rather than taking a go-git dependency, for the same
// reason kubeclient shells out to kubectl: the user's credentials, helpers, and
// signing config already work, and the project stays on its single dependency.
//
// Every mutating step is guarded. This package is the *only* place kubeloop
// writes anything, and it writes to a git branch — never to a cluster.
package gitrepo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes git in dir and returns stdout. Injectable for tests.
type Runner func(ctx context.Context, dir string, args ...string) ([]byte, error)

// ExecRunner runs git for real, surfacing stderr in the error (git puts
// "rejected", "permission denied" and hook output there).
func ExecRunner(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// Repo is a local git working tree.
type Repo struct {
	Dir string
	Run Runner // nil → ExecRunner
}

func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	run := r.Run
	if run == nil {
		run = ExecRunner
	}
	out, err := run(ctx, r.Dir, args...)
	return strings.TrimSpace(string(out)), err
}

// OriginRepo returns the owner and repository name of `origin`.
func (r *Repo) OriginRepo(ctx context.Context) (owner, name string, err error) {
	url, err := r.run(ctx, "remote", "get-url", "origin")
	if err != nil {
		return "", "", err
	}
	return ParseOrigin(url)
}

// ParseOrigin extracts owner/name from the HTTPS, SSH, or scp-style forms of a
// GitHub remote URL.
//
// The ".git" suffix is removed with TrimSuffix, not by trimming a cutset: a
// repository legitimately named "kubeloop-" would otherwise lose its trailing
// dash and every subsequent API call would 404 against a repo that doesn't
// exist. Cutset trimming on suffixes is a classic silent corrupter.
func ParseOrigin(rawURL string) (owner, name string, err error) {
	s := strings.TrimSpace(rawURL)
	switch {
	case strings.HasPrefix(s, "git@"): // git@github.com:owner/repo.git
		_, after, ok := strings.Cut(s, ":")
		if !ok {
			return "", "", fmt.Errorf("unrecognized git remote %q", rawURL)
		}
		s = after
	case strings.Contains(s, "://"): // https:// or ssh://
		_, after, _ := strings.Cut(s, "://")
		_, after, ok := strings.Cut(after, "/") // drop host
		if !ok {
			return "", "", fmt.Errorf("unrecognized git remote %q", rawURL)
		}
		s = after
	default:
		return "", "", fmt.Errorf("unrecognized git remote %q", rawURL)
	}
	s = strings.TrimSuffix(strings.Trim(s, "/"), ".git")
	owner, name, ok := strings.Cut(s, "/")
	if !ok || owner == "" || name == "" {
		return "", "", fmt.Errorf("unrecognized git remote %q", rawURL)
	}
	if strings.Contains(name, "/") {
		return "", "", fmt.Errorf("unrecognized git remote %q", rawURL)
	}
	return owner, name, nil
}

// IsClean reports whether the working tree has no uncommitted changes.
func (r *Repo) IsClean(ctx context.Context) (bool, error) {
	out, err := r.run(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

// CurrentBranch returns the checked-out branch name.
func (r *Repo) CurrentBranch(ctx context.Context) (string, error) {
	return r.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// CreateBranch creates and checks out branch, refusing to run on a dirty tree.
//
// Branching on a dirty tree would sweep the user's unrelated uncommitted work
// into kubeloop's pull request. Refuse rather than surprise them.
func (r *Repo) CreateBranch(ctx context.Context, branch string) error {
	if err := validRef(branch); err != nil {
		return err
	}
	clean, err := r.IsClean(ctx)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("working tree has uncommitted changes: commit or stash them before kubeloop opens a PR")
	}
	_, err = r.run(ctx, "checkout", "-b", branch)
	return err
}

// CommitFile stages one already-written path and commits it. Only that path is
// staged: `git commit -a` or `git add .` could sweep in unrelated files.
func (r *Repo) CommitFile(ctx context.Context, path, message string) error {
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("invalid path %q: must not begin with '-'", path)
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("empty commit message")
	}
	if _, err := r.run(ctx, "add", "--", path); err != nil {
		return err
	}
	// Refuse an empty commit: a PR that changes nothing must never be opened,
	// the same guarantee pr.Prepare enforces at the rounding layer.
	staged, err := r.run(ctx, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if staged == "" {
		return fmt.Errorf("nothing staged for %s: the patch changed no bytes", path)
	}
	_, err = r.run(ctx, "commit", "-m", message, "--", path)
	return err
}

// Push publishes branch to origin. It refuses to push the base branch, so a
// misconfigured caller cannot push straight to main.
func (r *Repo) Push(ctx context.Context, branch, base string) error {
	if err := validRef(branch); err != nil {
		return err
	}
	if branch == base {
		return fmt.Errorf("refusing to push directly to the base branch %q", base)
	}
	_, err := r.run(ctx, "push", "--set-upstream", "origin", branch)
	return err
}

// validRef rejects names git would read as flags, or that aren't refs.
func validRef(name string) error {
	if name == "" {
		return fmt.Errorf("empty branch name")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid branch %q: must not begin with '-'", name)
	}
	if strings.ContainsAny(name, " \t~^:?*[\\") {
		return fmt.Errorf("invalid branch %q", name)
	}
	return nil
}
