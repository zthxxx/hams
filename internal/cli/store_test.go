package cli

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hamserr "github.com/zthxxx/hams/internal/error"
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

// TestStoreStatus_HonorsProfileOverride — cycle 218 guard. Pre-cycle-218
// `hams --profile=X store status` silently showed the config-file's
// profile_tag, contradicting what apply/refresh/list would use for
// the same invocation. The test seeds TWO profile dirs (config-file
// profile "fromfile" and overridden profile "override") and asserts
// that passing --profile=override makes the output show the override
// value. Regression against forgetting the overlay step.
func TestStoreStatus_HonorsProfileOverride(t *testing.T) {
	storeDir := t.TempDir()
	// Seed both profile dirs so neither triggers a "dir missing" path.
	for _, p := range []string{"fromfile", "override"} {
		if err := os.MkdirAll(filepath.Join(storeDir, p), 0o750); err != nil {
			t.Fatalf("mkdir profile %s: %v", p, err)
		}
	}
	t.Setenv("HAMS_CONFIG_HOME", t.TempDir())
	t.Setenv("HAMS_DATA_HOME", t.TempDir())

	got := captureStdout(t, func() {
		registry := provider.NewRegistry()
		app := NewApp(registry, sudo.NoopAcquirer{})
		args := []string{"hams", "--store", storeDir, "--profile", "override", "store", "status"}
		if err := app.Run(context.Background(), args); err != nil {
			t.Fatalf("store status: %v", err)
		}
	})

	if !strings.Contains(got, "override") {
		t.Errorf("store status output missing overridden profile %q; full:\n%s", "override", got)
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

// TestStoreStatus_CanceledContextAbortsPromptly locks in cycle 157:
// `hams store status` on a git repo previously used context.Background()
// for the inner `git status --short` probe (with a 5s timeout), so
// SIGINT/SIGTERM during the probe was ignored and the user had to
// wait up to 5s for the timeout. Now: derives from the request ctx
// so cancellation propagates immediately. Asserts: pre-canceling
// the context returns from the action much faster than the 5s
// timeout (using a 2s upper bound for noisy CI environments).
func TestStoreStatus_CanceledContextAbortsPromptly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	storeDir, _, _, _ := setupApplyTestEnv(t, []string{"apt"})

	if err := exec.CommandContext(context.Background(), "git", "-C", storeDir, "init", "-q").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	// Pre-cancel the context. exec.CommandContext fires the kill
	// signal on cancel, so the git status invocation should abort
	// nearly immediately instead of running the full ~ms timeout
	// budget. Even with timing noise on CI, 2s should be plenty.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})
	// Errors from the captureStdout body are intentionally swallowed
	// here — we only care about cancellation timing for this test.
	captureStdout(t, func() {
		if err := app.Run(ctx, []string{"hams", "--store", storeDir, "store", "status"}); err != nil {
			t.Logf("store status (expected possibly-error on cancel): %v", err)
		}
	})
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Errorf("store status with canceled ctx took %v; want < 2s (5s timeout was being honored, ignoring ctx)", elapsed)
	}
}

// TestList_NonexistentStorePathEmitsUserError locks in cycle 211:
// `hams list --store=/ghost` (where the path doesn't exist) previously
// printed "No managed resources found. Run 'hams <provider> install
// <package>' ..." — misleading because the user's real issue was a
// misaimed store_path, not an empty store. Now: surface an
// ExitUsageError naming the bad path with the same recovery hints as
// apply (cycle 87) / refresh (cycle 88).
func TestList_NonexistentStorePathEmitsUserError(t *testing.T) {
	ghostStore := filepath.Join(t.TempDir(), "ghost-store-does-not-exist")

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

	// Redirect config dirs so the test doesn't pick up the user's real config.
	t.Setenv("HAMS_CONFIG_HOME", t.TempDir())
	t.Setenv("HAMS_DATA_HOME", t.TempDir())

	app := NewApp(registry, sudo.NoopAcquirer{})
	err := app.Run(context.Background(), []string{"hams", "--store", ghostStore, "list"})
	if err == nil {
		t.Fatal("expected ExitUsageError for missing store_path")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitUsageError {
		t.Fatalf("expected ExitUsageError, got %v (%T)", err, err)
	}
	if !strings.Contains(ufe.Message, ghostStore) {
		t.Errorf("error should name the bad path; got %q", ufe.Message)
	}
	// Regression: the bad surface message is "No managed resources
	// found" — assert it's NOT in the error.
	if strings.Contains(ufe.Error(), "No managed resources found") {
		t.Errorf("error should NOT use the misleading empty-store text; got %q", ufe.Error())
	}
}

// TestList_NonexistentProfileEmitsUserError — cycle 217 guard.
// `hams --profile=<typo> list` used to be a silent no-op: the list
// Action never applied flags.Profile to cfg.ProfileTag, so the
// override was dropped and the "No managed resources found" fallback
// fired against whatever profile_tag the config file specified.
// Apply (cycle 92) and refresh (cycle 93) already validate the
// overridden profile dir; cycle 217 adds the same check to list.
func TestList_NonexistentProfileEmitsUserError(t *testing.T) {
	storeDir := t.TempDir()
	// Seed a valid profile so cfg.Load finds store_path OK. The
	// typo'd --profile value is the focus of this test.
	if err := os.MkdirAll(filepath.Join(storeDir, "macOS"), 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}

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

	t.Setenv("HAMS_CONFIG_HOME", t.TempDir())
	t.Setenv("HAMS_DATA_HOME", t.TempDir())

	app := NewApp(registry, sudo.NoopAcquirer{})
	err := app.Run(context.Background(), []string{"hams", "--store", storeDir, "--profile", "Typo", "list"})
	if err == nil {
		t.Fatal("expected ExitUsageError for typo'd profile")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitUsageError {
		t.Fatalf("expected ExitUsageError, got %v (%T)", err, err)
	}
	if !strings.Contains(ufe.Message, "Typo") {
		t.Errorf("error should name the bad profile; got %q", ufe.Message)
	}
	if strings.Contains(ufe.Error(), "No managed resources found") {
		t.Errorf("error should NOT use the misleading empty-store text; got %q", ufe.Error())
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

// TestConfigList_IncludesStoreRepo locks in cycle 124: both the
// text and JSON outputs of `hams config list` include store_repo.
// Previously store_repo was omitted — users who set it via
// `hams config set store_repo ...` couldn't see it in `list`
// (only via `config get store_repo`).
func TestConfigList_IncludesStoreRepo(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", t.TempDir())
	// Seed a global config with store_repo set.
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: t\nmachine_id: m\nstore_repo: github.com/zthxxx/hams-store\n")

	// Text output should include the Store repo line.
	textOut := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(), []string{"hams", "config", "list"}); err != nil {
			t.Fatalf("config list text: %v", err)
		}
	})
	if !strings.Contains(textOut, "Store repo:") || !strings.Contains(textOut, "github.com/zthxxx/hams-store") {
		t.Errorf("text output missing store_repo; got:\n%s", textOut)
	}

	// JSON output should have the store_repo key.
	jsonOut := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(), []string{"hams", "--json", "config", "list"}); err != nil {
			t.Fatalf("config list json: %v", err)
		}
	})
	if !strings.Contains(jsonOut, `"store_repo": "github.com/zthxxx/hams-store"`) {
		t.Errorf("JSON output missing store_repo; got:\n%s", jsonOut)
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

// TestList_DeterministicOrderAcrossRuns locks in cycle 149: per-provider
// rows in `hams list` (text and JSON) must be in stable, alphabetical
// order across invocations. Previously the IDs were collected by
// iterating sf.Resources (a Go map) — non-deterministic, so each
// `hams list` shuffled the rows. Broke grep/diff/snapshot workflows.
// Symmetric with cycle 148's fix in DiffDesiredVsState (covers a
// different code path: the listCmd flow does not go through
// DiffDesiredVsState — it iterates state directly).
func TestList_DeterministicOrderAcrossRuns(t *testing.T) {
	storeDir, _, stateDir, _ := setupApplyTestEnv(t, []string{"apt"})

	// Seed an apt state file with 6 resources whose alphabetical order
	// is unrelated to insertion order — increases the chance Go's map
	// iteration would shuffle them differently across invocations.
	statePath := filepath.Join(stateDir, "apt.state.yaml")
	writeApplyTestFile(t, statePath, `provider: apt
machine_id: test-machine
resources:
  zsh:
    state: ok
    version: "5.9"
  htop:
    state: ok
    version: "3.2.2"
  curl:
    state: ok
    version: "7.81"
  ack:
    state: ok
    version: "3.5"
  jq:
    state: ok
    version: "1.6"
  vim:
    state: ok
    version: "8.2"
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

	runOnce := func() string {
		return captureStdout(t, func() {
			app := NewApp(registry, sudo.NoopAcquirer{})
			if err := app.Run(context.Background(),
				[]string{"hams", "--store", storeDir, "list"}); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	}

	first := runOnce()
	for range 20 {
		got := runOnce()
		if got != first {
			t.Errorf("list output differs across runs:\nfirst:\n%s\n\nlater:\n%s", first, got)
			break
		}
	}

	// Assert alphabetical ordering: ack < curl < htop < jq < vim < zsh.
	want := []string{"ack", "curl", "htop", "jq", "vim", "zsh"}
	last := -1
	for _, name := range want {
		idx := strings.Index(first, name)
		if idx < 0 {
			t.Errorf("output missing %q; got:\n%s", name, first)
			continue
		}
		if idx <= last {
			t.Errorf("output not alphabetical: %q at idx %d should come after previous (idx %d)", name, idx, last)
		}
		last = idx
	}
}

// TestList_JSON_DeterministicOrder asserts the same per-provider
// determinism for the --json output path. Scripts consuming
// `hams list --json` would otherwise see the array elements
// shuffled across runs.
func TestList_JSON_DeterministicOrder(t *testing.T) {
	storeDir, _, stateDir, _ := setupApplyTestEnv(t, []string{"apt"})

	statePath := filepath.Join(stateDir, "apt.state.yaml")
	writeApplyTestFile(t, statePath, `provider: apt
machine_id: test-machine
resources:
  zsh: {state: ok, version: "5.9"}
  htop: {state: ok, version: "3.2.2"}
  curl: {state: ok, version: "7.81"}
  ack: {state: ok, version: "3.5"}
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

	runOnce := func() string {
		return captureStdout(t, func() {
			app := NewApp(registry, sudo.NoopAcquirer{})
			if err := app.Run(context.Background(),
				[]string{"hams", "--store", storeDir, "--json", "list"}); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	}

	first := runOnce()
	for range 20 {
		if got := runOnce(); got != first {
			t.Errorf("list --json output differs across runs:\nfirst:\n%s\n\nlater:\n%s", first, got)
			break
		}
	}

	// Validate alphabetical ordering by checking ID positions.
	want := []string{`"id": "ack"`, `"id": "curl"`, `"id": "htop"`, `"id": "zsh"`}
	last := -1
	for _, frag := range want {
		idx := strings.Index(first, frag)
		if idx < 0 {
			t.Errorf("JSON missing %s; got:\n%s", frag, first)
			continue
		}
		if idx <= last {
			t.Errorf("JSON not alphabetical: %s at idx %d should come after previous (idx %d)", frag, idx, last)
		}
		last = idx
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

// TestList_CorruptStateFileEmitsWarning locks in cycle 236: a state
// file that exists but fails to parse (corrupt YAML, permission
// denied) previously caused `hams list` to silently skip the
// provider and print "No managed resources found" — indistinguishable
// from a fresh empty store to the user whose state had been
// corrupted mid-write or by an editor crash. Now: slog.Warn names
// the provider + path + underlying error before the loop continues.
// Healthy providers in the same store still surface in the list.
func TestList_CorruptStateFileEmitsWarning(t *testing.T) {
	storeDir, _, stateDir, _ := setupApplyTestEnv(t, []string{"apt"})

	// setupApplyTestEnv doesn't create stateDir (apply/refresh's lazy
	// mkdir does that on first write). For this test we need the dir
	// up front so we can drop a corrupt state file in place.
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	// Write garbage YAML to the apt state path. state.Load will error.
	statePath := filepath.Join(stateDir, "apt.state.yaml")
	if err := os.WriteFile(statePath, []byte("not: valid: yaml: here\n"), 0o600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

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

	// Redirect slog to capture the warn line.
	origDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origDefault) })
	var buf strings.Builder
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	app := NewApp(registry, sudo.NoopAcquirer{})
	if err := app.Run(context.Background(), []string{"hams", "--store", storeDir, "list"}); err != nil {
		t.Fatalf("list: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "state file unreadable") {
		t.Errorf("expected 'state file unreadable' warning; got:\n%s", got)
	}
	if !strings.Contains(got, "apt") {
		t.Errorf("warning should name the provider; got:\n%s", got)
	}
	if !strings.Contains(got, statePath) {
		t.Errorf("warning should name the state file path; got:\n%s", got)
	}
}
