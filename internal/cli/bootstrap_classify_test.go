package cli

import (
	"os"
	"path/filepath"
	"testing"
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
