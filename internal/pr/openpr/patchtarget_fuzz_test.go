package openpr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostileRepo builds a checkout laced with every escape patchTarget must resist:
// a .git dir, a symlinked leaf and a symlinked directory pointing outside, a
// hard link to an outside file, plus legitimate files that must stay writable.
// Returns the repo root and the "outside" directory that must never be written.
func hostileRepo(t *testing.T) (repo, outside string) {
	t.Helper()
	root := t.TempDir()
	repo = filepath.Join(root, "repo")
	outside = filepath.Join(root, "outside")
	for _, d := range []string{
		filepath.Join(repo, "deploy", "prod"),
		filepath.Join(repo, ".git", "hooks"),
		outside,
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Legit, writable targets.
	write(filepath.Join(repo, "deploy.yaml"), "SENTINEL")
	write(filepath.Join(repo, "deploy", "prod", "app.yaml"), "SENTINEL")
	write(filepath.Join(repo, ".gitignore"), "SENTINEL")
	// Off-limits: git internals, and files outside the repo.
	write(filepath.Join(repo, ".git", "config"), "GIT-ORIGINAL")
	write(filepath.Join(repo, ".git", "hooks", "pre-commit"), "HOOK-ORIGINAL")
	write(filepath.Join(outside, "secret.yaml"), "OUTSIDE-ORIGINAL")
	write(filepath.Join(outside, "hl.yaml"), "OUTSIDE-ORIGINAL")
	// Aliases that resolve out of the repo.
	_ = os.Symlink(filepath.Join(outside, "secret.yaml"), filepath.Join(repo, "evil-leaf.yaml"))
	_ = os.Symlink(outside, filepath.Join(repo, "evil-dir"))
	_ = os.Link(filepath.Join(outside, "hl.yaml"), filepath.Join(repo, "hardlink.yaml"))
	return repo, outside
}

// hasDotGitSegment reports whether any path segment normalizes to ".git" the
// way Windows would (case-insensitive, trailing dots/spaces stripped).
func hasDotGitSegment(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.EqualFold(strings.TrimRight(seg, ". "), ".git") {
			return true
		}
	}
	return false
}

// FuzzPatchTarget asserts the whole safety contract at once: if patchTarget
// returns a path with no error, that path — with symlinks resolved — is a
// regular file that genuinely lives inside the repository and not inside .git.
// Anything it cannot guarantee, it must reject; an error is always acceptable.
//
// The seed corpus runs on every `go test` (a permanent regression suite for all
// six escapes found by hand); `go test -run x -fuzz FuzzPatchTarget` searches
// for a seventh.
func FuzzPatchTarget(f *testing.F) {
	for _, seed := range []string{
		"deploy.yaml", "deploy/prod/app.yaml", ".gitignore", // legit
		"", ".", "..", "../outside/secret.yaml", "a/../../outside/secret.yaml",
		"/etc/passwd", "/outside/secret.yaml",
		".git/config", ".git/hooks/pre-commit", "sub/.git/config",
		".GIT/config", ".git./config", ".git /config", ".git../config",
		"evil-leaf.yaml", "evil-dir/secret.yaml", "hardlink.yaml",
		"deploy/../.git/config", "./deploy.yaml", "deploy//app.yaml",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rel string) {
		repo, outside := hostileRepo(t)

		// Snapshot every off-limits file so we can prove none was disturbed.
		offLimits := map[string]string{
			filepath.Join(repo, ".git", "config"):              "GIT-ORIGINAL",
			filepath.Join(repo, ".git", "hooks", "pre-commit"): "HOOK-ORIGINAL",
			filepath.Join(outside, "secret.yaml"):              "OUTSIDE-ORIGINAL",
			filepath.Join(outside, "hl.yaml"):                  "OUTSIDE-ORIGINAL",
		}

		full, err := patchTarget(repo, rel)
		if err != nil {
			return // rejecting is always safe
		}

		// On success, simulate the write openpr would perform, then verify it
		// stayed within contract.
		if err := os.WriteFile(full, []byte("PATCHED"), 0o644); err != nil {
			return // couldn't write; nothing escaped
		}

		// 1. No off-limits file may have changed.
		for p, want := range offLimits {
			if got, _ := os.ReadFile(p); string(got) != want {
				t.Fatalf("rel=%q: wrote through to off-limits %q (now %q)", rel, p, got)
			}
		}
		// 2. The real target must live inside the real repo root.
		realRoot, err := filepath.EvalSymlinks(repo)
		if err != nil {
			t.Fatal(err)
		}
		realTarget, err := filepath.EvalSymlinks(full)
		if err != nil {
			t.Fatalf("rel=%q: returned path does not resolve: %v", rel, err)
		}
		inside, err := filepath.Rel(realRoot, realTarget)
		if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
			t.Fatalf("rel=%q: target %q resolves OUTSIDE repo %q", rel, realTarget, realRoot)
		}
		// 3. The real target must not be inside .git.
		if hasDotGitSegment(inside) {
			t.Fatalf("rel=%q: target resolves into .git (%q)", rel, inside)
		}
	})
}
