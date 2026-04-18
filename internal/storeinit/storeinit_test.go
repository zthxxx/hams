package storeinit_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/zthxxx/hams/internal/storeinit"
)

func TestBootstrap_FreshDirIsValidStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "store")

	if err := storeinit.Bootstrap(target); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if !storeinit.Bootstrapped(target) {
		t.Fatalf("Bootstrapped(%s) = false, want true after Bootstrap", target)
	}

	for _, want := range []string{".gitignore", "hams.config.yaml", "default", ".git"} {
		path := filepath.Join(target, want)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist after Bootstrap, got: %v", path, err)
		}
	}
}

func TestBootstrap_EmptyPathRejected(t *testing.T) {
	t.Parallel()
	if err := storeinit.Bootstrap(""); err == nil {
		t.Fatal("Bootstrap(\"\") returned nil, want error")
	}
}

func TestBootstrap_PreservesExistingFiles(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "store")
	if err := storeinit.Bootstrap(target); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}

	custom := []byte("# user-edited config\nstore_path: /elsewhere\n")
	configPath := filepath.Join(target, "hams.config.yaml")
	if err := os.WriteFile(configPath, custom, 0o600); err != nil {
		t.Fatalf("writing custom config: %v", err)
	}

	if err := storeinit.Bootstrap(target); err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading after second Bootstrap: %v", err)
	}
	if string(got) != string(custom) {
		t.Errorf("Bootstrap clobbered hand-edited config\nwant: %q\ngot:  %q", custom, got)
	}
}

// TestBootstrap_PropertyIdempotent encodes the invariant: re-running Bootstrap
// repeatedly on a fresh temp dir always produces the same valid store and
// never errors. Property-based per CLAUDE.md testing convention.
func TestBootstrap_PropertyIdempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		repeats := rapid.IntRange(1, 6).Draw(t, "repeats")
		dir := filepath.Join(testTempDir(t), "store")
		for i := range repeats {
			if err := storeinit.Bootstrap(dir); err != nil {
				t.Fatalf("Bootstrap iteration %d: %v", i, err)
			}
			if !storeinit.Bootstrapped(dir) {
				t.Fatalf("Bootstrapped(%s) = false after iteration %d", dir, i)
			}
		}
	})
}

func TestBootstrap_NestedNonExistentParents(t *testing.T) {
	t.Parallel()
	deep := filepath.Join(t.TempDir(), "a", "b", "c", "store")
	if err := storeinit.Bootstrap(deep); err != nil {
		t.Fatalf("Bootstrap on deeply nested path: %v", err)
	}
	if !storeinit.Bootstrapped(deep) {
		t.Fatalf("Bootstrapped(%s) = false", deep)
	}
}

func testTempDir(t *rapid.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "storeinit-rapid-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			t.Errorf("temp dir cleanup: %v", rmErr)
		}
	})
	return dir
}

// TestBootstrap_ContextTimeoutStopsHungGitHook pins the
// `initGitRepo` timeout contract: when the external `git init`
// invocation (real or simulated) takes longer than [GitInitTimeout],
// [BootstrapContext] MUST return with a wrapped deadline-exceeded
// error rather than block indefinitely.
//
// The test swaps both DI seams — [LookPathGit] to pretend `git` exists
// on the expected path, [ExecCommandContext] to return a real `exec.Cmd`
// pointing at `/usr/bin/sleep <long>` — and shrinks [GitInitTimeout]
// to 150 ms so the whole test terminates in well under a second.
//
// Regression gate for the CLAUDE.md task-list bullet
// "auto-init-ux-hardening" / "wrap git init in context.WithTimeout(…,
// 30*time.Second)" — without the timeout the test would hang for the
// full sleep duration and only fail on the go-test-run-time deadline.
func TestBootstrap_ContextTimeoutStopsHungGitHook(t *testing.T) {
	// Save + restore every seam we touch so parallel test runs (or
	// follow-up tests in this package) see pristine state.
	origLookPath := storeinit.LookPathGit
	origExec := storeinit.ExecCommandContext
	origTimeout := storeinit.GitInitTimeout
	t.Cleanup(func() {
		storeinit.LookPathGit = origLookPath
		storeinit.ExecCommandContext = origExec
		storeinit.GitInitTimeout = origTimeout
	})

	// Pretend `git` lives at a well-known path; the value is passed
	// straight to ExecCommandContext below where the test's fake
	// ignores it.
	storeinit.LookPathGit = func() (string, error) {
		return "/fake/git", nil
	}

	// Swap the exec seam for a real `sleep 5` process. The test
	// timeout (150 ms) is the trigger — ExecCommandContext receives
	// the timed-out context so `CombinedOutput` returns early with
	// the context's deadline-exceeded error.
	storeinit.ExecCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/usr/bin/sleep", "5")
	}

	// Shrink the timeout so the assertion completes in <1 s.
	storeinit.GitInitTimeout = 150 * time.Millisecond

	dir := t.TempDir()
	target := filepath.Join(dir, "store")

	start := time.Now()
	err := storeinit.BootstrapContext(context.Background(), target)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("BootstrapContext returned nil, want deadline-exceeded error")
	}
	// Envelope: the exec.Cmd reports its own deadline via err.Error();
	// the outer wrapping includes "git init failed" + "storeinit: git
	// init:" — either surfacing of the deadline string passes.
	msg := err.Error()
	if !errors.Is(err, context.DeadlineExceeded) &&
		!strings.Contains(msg, "signal: killed") &&
		!strings.Contains(msg, "deadline") {
		t.Errorf("error did not reflect context deadline: %v", err)
	}
	// Upper bound: with a 150 ms timeout we MUST be back well before
	// the 5 s sleep completes. 3 s gives plenty of slack for slow
	// CI runners without masking a genuine hang.
	if elapsed > 3*time.Second {
		t.Errorf("BootstrapContext took %v, want <3s (timeout did not fire)", elapsed)
	}
}
