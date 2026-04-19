package storeinit_test

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/zthxxx/hams/internal/storeinit"
)

// TestLookPathGit_ForcesFallbackWhenGitMissing simulates a machine
// without git on PATH and asserts the default ExecGitInit returns an
// error wrapping exec.ErrNotFound, so ensureGitRepo's switch routes
// to the GoGitInit fallback (covered separately by the existing
// Bootstrap_GoGitFallback test).
func TestLookPathGit_ForcesFallbackWhenGitMissing(t *testing.T) {
	// Cannot run in parallel — swaps package-level seam.
	orig := storeinit.LookPathGit
	t.Cleanup(func() { storeinit.LookPathGit = orig })
	storeinit.LookPathGit = func() (string, error) {
		return "", exec.ErrNotFound
	}

	err := storeinit.ExecGitInit(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when LookPathGit reports git missing")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("err = %v, want it to wrap exec.ErrNotFound", err)
	}
}

// TestExecCommandContext_RecordsInvocation asserts the seam is hit
// with the resolved git binary path and the canonical `init --quiet`
// argv. Production runs the real git; here we record the call.
func TestExecCommandContext_RecordsInvocation(t *testing.T) {
	// Cannot run in parallel — swaps package-level seams.
	origLook := storeinit.LookPathGit
	origCmd := storeinit.ExecCommandContext
	t.Cleanup(func() {
		storeinit.LookPathGit = origLook
		storeinit.ExecCommandContext = origCmd
	})
	storeinit.LookPathGit = func() (string, error) {
		return "/usr/bin/git", nil
	}

	var gotName string
	var gotArgs []string
	storeinit.ExecCommandContext = func(_ context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		// Return a no-op command (`true` is a portable always-success).
		return exec.CommandContext(context.Background(), "true")
	}

	if err := storeinit.ExecGitInit(context.Background(), "/tmp/storeinit-test"); err != nil {
		t.Fatalf("ExecGitInit: %v", err)
	}
	if gotName != "/usr/bin/git" {
		t.Errorf("name = %q, want /usr/bin/git", gotName)
	}
	wantArgs := []string{"init", "--quiet", "/tmp/storeinit-test"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args length = %d, want %d (got %v)", len(gotArgs), len(wantArgs), gotArgs)
	}
	for i, a := range wantArgs {
		if gotArgs[i] != a {
			t.Errorf("args[%d] = %q, want %q", i, gotArgs[i], a)
		}
	}
}

// TestGitInitTimeout_TestablyShrinkable asserts the timeout is a
// var (not const) so tests in callers can shrink it without rebuilding.
// Smoke-test: change it, observe the change, restore.
func TestGitInitTimeout_TestablyShrinkable(t *testing.T) {
	// Cannot run in parallel — mutates package-level var.
	orig := storeinit.GitInitTimeout
	t.Cleanup(func() { storeinit.GitInitTimeout = orig })
	storeinit.GitInitTimeout = 1 * time.Millisecond
	if storeinit.GitInitTimeout != 1*time.Millisecond {
		t.Errorf("GitInitTimeout = %v, want 1ms", storeinit.GitInitTimeout)
	}
}
