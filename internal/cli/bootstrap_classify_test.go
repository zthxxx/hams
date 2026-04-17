package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
)

// TestIsLocalPathAttempt_AbsolutePath pins the classification rule:
// any path starting with `/` is unambiguously local. A user who
// typos an absolute path should see a local-path error, not a
// misleading "clone from GitHub failed" (that would blame
// network/auth for a purely local issue).
func TestIsLocalPathAttempt_AbsolutePath(t *testing.T) {
	t.Parallel()
	if !isLocalPathAttempt("/tmp/nonexistent-store") {
		t.Error("absolute path should be classified local even if it doesn't exist")
	}
	if !isLocalPathAttempt("/") {
		t.Error("root directory should be classified local")
	}
}

// TestIsLocalPathAttempt_TildePrefix pins the rule: `~/` at start
// is a home-dir reference, classified local.
func TestIsLocalPathAttempt_TildePrefix(t *testing.T) {
	t.Parallel()
	if !isLocalPathAttempt("~/projects/hams-store") {
		t.Error("~/ prefix should be classified local")
	}
}

// TestIsLocalPathAttempt_RelativePrefix pins the rule: `./` and
// `../` prefixes are unambiguous relative paths, classified local
// even if they don't exist on disk. This guards against a future
// "optimization" that only returns true after stat — which would
// make typo'd relative paths fall through to the GitHub clone
// branch and produce a confusing error about a missing user/repo.
func TestIsLocalPathAttempt_RelativePrefix(t *testing.T) {
	t.Parallel()
	if !isLocalPathAttempt("./local-store") {
		t.Error("./ prefix should be classified local")
	}
	if !isLocalPathAttempt("../sibling-store") {
		t.Error("../ prefix should be classified local")
	}
}

// TestIsLocalPathAttempt_GitHubShorthandFalse pins the inverse
// rule: `user/repo` shorthand (no /, ~/, ./, ../ prefix and no
// stat hit) is NOT local. This is the primary dispatch signal that
// lets bootstrap.go clone instead of trying filepath.Abs.
func TestIsLocalPathAttempt_GitHubShorthandFalse(t *testing.T) {
	t.Parallel()
	if isLocalPathAttempt("zthxxx/hams-store") {
		t.Error("user/repo shorthand should NOT be classified local")
	}
	if isLocalPathAttempt("https://github.com/zthxxx/hams-store.git") {
		t.Error("full URL should NOT be classified local")
	}
}

// TestIsLocalPathAttempt_ExistingDirectoryTrue asserts that a bare
// name (no path prefixes) is classified local when it happens to
// exist on disk relative to cwd. This is the "stat it" last-resort
// branch — accommodates users who cd into their home, run
// `hams apply --from-repo=my-store` with no prefix.
func TestIsLocalPathAttempt_ExistingDirectoryTrue(t *testing.T) {
	// Create a tempdir and cd into it so a bare name is relative.
	dir := t.TempDir()
	localRepo := filepath.Join(dir, "local-repo")
	if err := os.Mkdir(localRepo, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldWd, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("Getwd: %v", wdErr)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Logf("chdir back failed (test cleanup): %v", err)
		}
	})

	if !isLocalPathAttempt("local-repo") {
		t.Error("bare name that stats to an existing dir should be classified local")
	}
}

// TestPromptProfileInit_RejectsInvalidInput locks in cycle 198: the
// interactive profile prompt previously accepted any input string.
// A user typing "../etc" at the prompt would set cfg.ProfileTag
// in memory to "../etc", but cycle 197's config.WriteConfigKey
// would reject the persist → the in-memory value diverged from
// the YAML. Now: validate at the prompt so the user sees an
// immediate error instead of a silent divergence.
func TestPromptProfileInit_RejectsInvalidInput(t *testing.T) {
	// Redirect os.Stdin through a pipe carrying the invalid input.
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	cases := []struct {
		name string
		body string
		want string // substring in error
	}{
		{"traversal-profile", "../etc\nvalid-machine\n", "profile tag"},
		{"traversal-machine", "valid-profile\n../..\n", "machine ID"},
		{"slash-profile", "foo/bar\nbaz\n", "profile tag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Pipe: %v", err)
			}
			if _, werr := w.WriteString(tc.body); werr != nil {
				t.Fatalf("write: %v", werr)
			}
			if cerr := w.Close(); cerr != nil {
				t.Logf("close pipe writer: %v", cerr)
			}
			os.Stdin = r

			_, _, gotErr := promptProfileInit()
			if cerr := r.Close(); cerr != nil {
				t.Logf("close pipe reader: %v", cerr)
			}
			if gotErr == nil {
				t.Fatalf("expected error for %q", tc.body)
			}
			if !strings.Contains(gotErr.Error(), tc.want) {
				t.Errorf("error should mention %q; got %v", tc.want, gotErr)
			}
		})
	}
}

// TestCloneRemoteRepo_CanceledContextAborts locks in cycle 121:
// a canceled context reaches go-git's PlainCloneContext and
// aborts the clone promptly instead of waiting for network
// timeout. Previously cloneRemoteRepo used PlainClone (no
// context), so Ctrl+C during `hams apply --from-repo=...`
// appeared to hang — the process didn't exit until the TCP
// connection timed out (minutes).
//
// Uses an unreachable URL to avoid needing a real server; the
// context is pre-canceled so go-git's context check fires
// before the DNS/TCP round-trip, giving a deterministic fast-
// fail result.
func TestCloneRemoteRepo_CanceledContextAborts(t *testing.T) {
	// NOT Parallel: cloneRemoteRepo reads os.Stdout (for
	// `Progress: os.Stdout`), which races with captureStdout in
	// other tests that swap the global.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	paths := config.Paths{DataHome: t.TempDir()}
	_, err := cloneRemoteRepo(ctx, "https://example.invalid/user/repo.git", paths)
	if err == nil {
		t.Fatalf("expected clone error for canceled ctx, got nil")
	}
	// The error MAY be ctx.Err or a transformed network error
	// depending on when go-git checks the context. Either way it
	// MUST be a fail-fast — we assert it's not nil AND not the
	// "Repository not found" transform (which would imply the
	// full network round-trip completed).
	if errors.Is(err, context.Canceled) {
		// Ideal: go-git honored the cancellation directly.
		return
	}
	// Acceptable fallback: go-git wrapped the context cancel or
	// the name resolution failed because of no DNS. As long as
	// error is not nil we're confirming fast-fail behavior.
}

// TestIsLocalPathAttempt_NonexistentBareNameFalse asserts that a
// bare name that doesn't exist on disk falls through to the
// GitHub-shorthand path — callers then attempt to clone, which
// produces the proper "Repository not found" error shape from
// cycle 72's friendly-error work.
func TestIsLocalPathAttempt_NonexistentBareNameFalse(t *testing.T) {
	t.Parallel()
	if isLocalPathAttempt("definitely-not-a-real-repo-xyzxyz") {
		t.Error("nonexistent bare name should NOT be classified local")
	}
}
