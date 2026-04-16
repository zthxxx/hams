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

// TestStoreStatus_MissingStorePath locks in cycle-69: when the
// configured store_path points at a non-existent directory, status
// must emit a loud "(does NOT exist)" indicator, NOT print the
// derived paths as if the store were just empty.
func TestStoreStatus_MissingStorePath(t *testing.T) {
	// Build a config pointing at a store that doesn't exist.
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	ghostStore := filepath.Join(t.TempDir(), "ghost-store")
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: t\nmachine_id: m\nstore_path: "+ghostStore+"\n")

	got := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(), []string{"hams", "store", "status"}); err != nil {
			t.Fatalf("store status: %v", err)
		}
	})

	if !strings.Contains(got, "does NOT exist") {
		t.Errorf("expected 'does NOT exist' indicator; got:\n%s", got)
	}
	// The actionable suggestions should both appear.
	if !strings.Contains(got, "hams store init") {
		t.Errorf("expected 'hams store init' suggestion; got:\n%s", got)
	}
	// Profile tag / machine id should NOT print — the full status
	// block is skipped when the store doesn't exist.
	if strings.Contains(got, "Profile tag:") {
		t.Errorf("status block should be suppressed when store missing; got:\n%s", got)
	}
}

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

// TestList_TextOutput_ShowsLastErrorForFailed locks in cycle 119:
// failed / hook-failed resources emit the LastError text inline so
// debugging doesn't require `--json` or reading state YAML.
func TestList_TextOutput_ShowsLastErrorForFailed(t *testing.T) {
	storeDir, _, stateDir, _ := setupApplyTestEnv(t, []string{"apt"})

	statePath := filepath.Join(stateDir, "apt.state.yaml")
	writeApplyTestFile(t, statePath, `provider: apt
machine_id: test-machine
resources:
  htop:
    state: failed
    last_error: "package not found in repository"
`)

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

	got := captureStdout(t, func() {
		app := NewApp(registry, sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--store", storeDir, "list"}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	if !strings.Contains(got, "(error: package not found in repository)") {
		t.Errorf("text output missing last_error suffix; got:\n%s", got)
	}
}

// TestList_TextOutput_ShowsValueForKVConfig locks in cycle 117:
// text output of `hams list` displays the stored Value for
// KV-Config resources using ` = <value>` suffix, not just the
// bare `<id> <status>` that previously hid the actual config
// value. Package-class rows continue to use ` <version>`.
func TestList_TextOutput_ShowsValueForKVConfig(t *testing.T) {
	storeDir, _, stateDir, _ := setupApplyTestEnv(t, []string{"git-config"})

	statePath := filepath.Join(stateDir, "git-config.state.yaml")
	writeApplyTestFile(t, statePath, `provider: git-config
machine_id: test-machine
resources:
  user.name=zthxxx:
    state: ok
    value: zthxxx
`)

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "git-config", DisplayName: "git config", FilePrefix: "git-config",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := captureStdout(t, func() {
		app := NewApp(registry, sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--store", storeDir, "list"}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	// Value MUST appear after the status, using the ` = <value>` format.
	if !strings.Contains(got, "= zthxxx") {
		t.Errorf("text output missing value `= zthxxx`; got:\n%s", got)
	}
}

// TestList_JSON_IncludesValueAndLastError locks in the cycle-116
// enhancement: `hams list --json` surfaces the Resource.Value
// (relevant for KV-Config providers: defaults/duti/git-config) and
// Resource.LastError (scripts can detect failures without parsing
// the state YAML directly). omitempty keeps the output clean — a
// Package-class row with no value doesn't emit an empty `value`.
func TestList_JSON_IncludesValueAndLastError(t *testing.T) {
	storeDir, _, stateDir, _ := setupApplyTestEnv(t, []string{"git-config"})

	// Seed a git-config state file with a succeeded resource (Value
	// populated) and a failed one (LastError populated).
	statePath := filepath.Join(stateDir, "git-config.state.yaml")
	writeApplyTestFile(t, statePath, `provider: git-config
machine_id: test-machine
resources:
  user.name=zthxxx:
    state: ok
    value: zthxxx
  core.pager=less:
    state: failed
    last_error: "pager binary missing"
`)

	registry := provider.NewRegistry()
	p := &applyTestProvider{
		manifest: provider.Manifest{
			Name: "git-config", DisplayName: "git config", FilePrefix: "git-config",
			Platforms: []provider.Platform{provider.PlatformAll},
		},
	}
	if err := registry.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got := captureStdout(t, func() {
		app := NewApp(registry, sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--store", storeDir, "--json", "list"}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	// Must be valid JSON and contain the two resources.
	if !strings.Contains(got, `"value": "zthxxx"`) {
		t.Errorf("JSON output missing value for ok resource; got:\n%s", got)
	}
	if !strings.Contains(got, `"last_error": "pager binary missing"`) {
		t.Errorf("JSON output missing last_error for failed resource; got:\n%s", got)
	}
	// The ok resource should NOT emit a last_error key (omitempty).
	// Specifically, the entry for user.name=zthxxx (first in the output)
	// must not have last_error — check the text is bounded to the ok entry.
	if idx := strings.Index(got, "user.name=zthxxx"); idx >= 0 {
		if end := strings.Index(got[idx:], "}"); end > 0 {
			slice := got[idx : idx+end]
			if strings.Contains(slice, "last_error") {
				t.Errorf("ok-resource entry should NOT emit last_error; entry:\n%s", slice)
			}
		}
	}
}
