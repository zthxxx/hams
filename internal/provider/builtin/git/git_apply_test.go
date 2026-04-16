package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// requireGit skips the test when the host has no `git` binary on PATH.
// Builder containers (e.g. minimal alpine) may lack git; ConfigProvider's
// real-git tests can only assert their contract when git is present.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
}

// withIsolatedHome forces git to write `--global` config inside a
// throwaway tempdir instead of the developer's real ~/.gitconfig.
//
// Implementation note: setting HOME to tempdir alone is not enough on a
// CI runner that already has GIT_CONFIG_GLOBAL set to a different path.
// And setting GIT_CONFIG_GLOBAL="" (empty string) makes git try to write
// to an empty-filename path and fail with "Invalid cross-device link"
// (exit 4). The robust fix is to point GIT_CONFIG_GLOBAL at an explicit
// file inside our tempdir.
//
// Returns the gitconfig path so the test can read it directly.
func withIsolatedHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitconfigPath := filepath.Join(dir, ".gitconfig")
	t.Setenv("HOME", dir)
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfigPath)
	return gitconfigPath
}

// TestApply_WritesToIsolatedHome asserts that the ConfigProvider's Apply
// path writes only inside the tempdir HOME, never to the real home.
//
// Risk this test gates against: a contributor adding a new provider
// behavior that writes to ~/.gitconfig would silently corrupt the host
// when this test runs. The path assertion catches that immediately.
func TestApply_WritesToIsolatedHome(t *testing.T) {
	requireGit(t)
	configPath := withIsolatedHome(t)
	p := NewConfigProvider(nil)

	// Apply expects the resource ID format "key=value" per git.go:74-76.
	const key = "user.email"
	const value = "test@hams.example"
	action := provider.Action{ID: key + "=" + value}

	if err := p.Apply(context.Background(), action); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected gitconfig at %q: %v", configPath, err)
	}

	if !strings.Contains(string(body), value) {
		t.Errorf("gitconfig missing value %q; body=%q", value, string(body))
	}
}

// TestProbeAndRemove_RoundtripIsolated walks the full lifecycle inside
// the tempdir HOME: Apply writes a value → Probe reads it back → Remove
// drops it. Each step asserts the visible mutation; none touch the host
// developer's real ~/.gitconfig.
func TestProbeAndRemove_RoundtripIsolated(t *testing.T) {
	requireGit(t)
	withIsolatedHome(t)
	p := NewConfigProvider(nil)
	ctx := context.Background()

	const key = "init.defaultBranch"
	const value = "main"

	// Apply.
	if err := p.Apply(ctx, provider.Action{ID: key + "=" + value}); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Probe should observe state.StateOK with the written value.
	sf := state.New("git-config", "test-machine")
	sf.SetResource(key+"=", state.StateOK)
	results, err := p.Probe(ctx, sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1: %+v", len(results), results)
	}
	if results[0].State != state.StateOK {
		t.Errorf("Probe state = %v, want StateOK", results[0].State)
	}
	if results[0].Value != value {
		t.Errorf("Probe value = %q, want %q", results[0].Value, value)
	}

	// Remove drops the key.
	if removeErr := p.Remove(ctx, key+"="+value); removeErr != nil {
		t.Fatalf("Remove error: %v", removeErr)
	}

	// After Remove, Probe SHALL fail to find the key (`git config --get`
	// exits non-zero, surfacing as StateFailed in the probe result).
	results, err = p.Probe(ctx, sf)
	if err != nil {
		t.Fatalf("Probe after Remove error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe after Remove returned %d results, want 1", len(results))
	}
	if results[0].State != state.StateFailed {
		t.Errorf("Probe after Remove state = %v, want StateFailed (key absent)", results[0].State)
	}
}

// TestApply_RejectsMalformedResourceID asserts the Apply path returns
// a clear error rather than silently writing garbage when the resource
// ID is missing the `=value` half. This is enforced because the wider
// system relies on the resource-ID format invariant for state-key
// canonicalization (see git.go:75-78).
func TestApply_RejectsMalformedResourceID(t *testing.T) {
	requireGit(t)
	withIsolatedHome(t)
	p := NewConfigProvider(nil)

	err := p.Apply(context.Background(), provider.Action{ID: "no-equals-sign"})
	if err == nil {
		t.Fatalf("Apply with malformed ID should error, got nil")
	}
	if !strings.Contains(err.Error(), "scope.key=value") {
		t.Errorf("error message should hint at the required format, got: %v", err)
	}
}
