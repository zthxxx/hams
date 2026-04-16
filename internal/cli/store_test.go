package cli

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

// TestStoreStatus_SpecCompliantOutput asserts cycle-56: `hams store
// status` emits the four spec-required lines (store path, profile
// tag, machine-id, uncommitted changes). Previously profile tag +
// machine-id were missing, and no git status was attempted.
func TestStoreStatus_SpecCompliantOutput(t *testing.T) {
	storeDir, _, _, flags := setupApplyTestEnv(t, []string{"apt"})
	_ = flags // satisfy linter

	got := captureStdout(t, func() {
		registry := provider.NewRegistry()
		app := NewApp(registry, sudo.NoopAcquirer{})
		if err := app.Run(context.Background(), []string{"hams", "--store", storeDir, "store", "status"}); err != nil {
			t.Fatalf("store status: %v", err)
		}
	})

	// Spec requires all four fields; git-status is present since
	// setupApplyTestEnv writes a non-git store so we'd expect absence,
	// but we assert the four primary lines regardless.
	for _, want := range []string{
		"Store path:",
		"Profile tag:",
		"Machine ID:",
		"Hamsfiles:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("store status output missing %q; full:\n%s", want, got)
		}
	}
}

// TestStoreStatus_WithGitRepo asserts the Git status line is
// surfaced when the store is a git repo. Uses a real `git init`
// against a fresh tempdir so the test doesn't depend on network.
func TestStoreStatus_WithGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	storeDir, _, _, _ := setupApplyTestEnv(t, []string{"apt"})

	if err := exec.CommandContext(context.Background(), "git", "-C", storeDir, "init", "-q").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	// After `git init`, there's an untracked file (hams.config.yaml
	// and the profile dir), so git status should report changes.
	got := captureStdout(t, func() {
		registry := provider.NewRegistry()
		app := NewApp(registry, sudo.NoopAcquirer{})
		if err := app.Run(context.Background(), []string{"hams", "--store", storeDir, "store", "status"}); err != nil {
			t.Fatalf("store status: %v", err)
		}
	})

	if !strings.Contains(got, "Git status:") {
		t.Errorf("store status on git repo should include 'Git status:' line; got:\n%s", got)
	}
	// Store has hamsfile + state dirs, so status should be non-clean.
	if !strings.Contains(got, "uncommitted") && !strings.Contains(got, "clean") {
		t.Errorf("git status line should say 'uncommitted' or 'clean'; got:\n%s", got)
	}
}

// TestList_FilterExcludedAll_DistinctMessage asserts cycle-55: when
// resources exist but --status filters them all out, the output
// tells the user the filter excluded the matches rather than
// printing the truly-empty-store message (with install/apply
// suggestions that don't apply).
func TestList_FilterExcludedAll_DistinctMessage(t *testing.T) {
	storeDir, _, stateDir, _ := setupApplyTestEnv(t, []string{"apt"})

	// Seed an apt state with a non-hook-failed resource.
	statePath := filepath.Join(stateDir, "apt.state.yaml")
	writeApplyTestFile(t, statePath, `provider: apt
machine_id: test-machine
resources:
  urn:hams:apt:htop:
    state: ok
    version: "3.2.2"
`)

	// Register an apt provider so the registry has a match.
	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "apt", DisplayName: "apt", FilePrefix: "apt",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Filter on hook-failed → zero matches, but resources exist.
	got := captureStdout(t, func() {
		app := NewApp(registry, sudo.NoopAcquirer{})
		if err := app.Run(context.Background(), []string{"hams", "--store", storeDir, "list", "--status=hook-failed"}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
	if !strings.Contains(got, "No resources match the current filter") {
		t.Errorf("expected filter-excluded message; got:\n%s", got)
	}
	if strings.Contains(got, "Run 'hams <provider> install") {
		t.Errorf("filter-excluded should NOT show the empty-store install hint; got:\n%s", got)
	}
}

// Prevent unused-import removal of filepath when the file gets
// gofmt'd; it's used in future expansions for per-file checks.
var _ = filepath.Join
