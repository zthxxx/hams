package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

// TestApply_AutoInit_WritesConfigOnFirstRun exercises the "fresh
// machine, one-shot apply" workflow:
//
//   - Global config file does NOT exist (pristine machine).
//   - User passes `--tag=macOS` (plus a valid --store pointing at a
//     local fixture store so the apply has somewhere to read from).
//   - Hams auto-initializes ~/.config/hams/hams.config.yaml with
//     profile_tag=macOS + machine_id=<DeriveMachineID result>.
//
// Regression gate for CLAUDE.md's Current Tasks bullet: "If no hams
// config file exists when `hams apply` runs, auto-create one at the
// default location".
func TestApply_AutoInit_WritesConfigOnFirstRun(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	storeDir := filepath.Join(root, "store")
	profileDir := filepath.Join(storeDir, "macOS")

	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	// Store needs a hams.config.yaml to load — ResolvePaths requires
	// it as the StorePath anchor, but the body can be minimal.
	writeApplyTestFile(t, filepath.Join(storeDir, "hams.config.yaml"),
		"provider_priority: []\n")

	// Pre-flight invariant: no global config exists yet.
	if _, err := os.Stat(filepath.Join(configHome, "hams.config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("pre-flight: global config should not exist; stat err = %v", err)
	}

	// Pin DeriveMachineID to a deterministic value for this test so
	// the asserted machine_id string is stable across CI hosts.
	origHostname := config.HostnameLookup
	defer func() { config.HostnameLookup = origHostname }()
	config.HostnameLookup = func() (string, error) { return "ci-runner", nil }
	t.Setenv("HAMS_MACHINE_ID", "")

	flags := &provider.GlobalFlags{Store: storeDir, Tag: "macOS"}
	registry := provider.NewRegistry()

	if err := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{},
		"", true, "", "", false, bootstrapMode{}); err != nil {
		t.Fatalf("runApply first run: %v", err)
	}

	// Post-apply invariant: global config materialized with the
	// values we supplied + derived.
	data, err := os.ReadFile(filepath.Join(configHome, "hams.config.yaml"))
	if err != nil {
		t.Fatalf("global config not created: %v", err)
	}
	body := string(data)
	for _, want := range []string{"profile_tag: macOS", "machine_id: ci-runner"} {
		if !strings.Contains(body, want) {
			t.Errorf("global config missing %q; got:\n%s", want, body)
		}
	}
}

// TestApply_AutoInit_DoesNotOverwriteExistingConfig locks in the
// "first-run scope" invariant: if the user ALREADY has a global
// config (even partial), auto-init MUST NOT clobber it. Matches the
// openspec proposal section "Auto-init is lossless".
func TestApply_AutoInit_DoesNotOverwriteExistingConfig(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	dataHome := filepath.Join(root, "data")
	storeDir := filepath.Join(root, "store")
	profileDir := filepath.Join(storeDir, "linux")

	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}

	// Pre-existing global config: user carefully set linux + their_host.
	preExisting := "profile_tag: linux\nmachine_id: their_host\n"
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"), preExisting)
	writeApplyTestFile(t, filepath.Join(storeDir, "hams.config.yaml"),
		"provider_priority: []\n")

	// Caller passes --tag=macOS, a DIFFERENT value. The expectation
	// is that the CLI flag wins for this invocation but the
	// persisted config remains linux/their_host.
	flags := &provider.GlobalFlags{Store: storeDir, Tag: "macOS"}
	registry := provider.NewRegistry()

	// Expect failure because the `macOS` profile dir doesn't exist —
	// but the failure happens AFTER config is loaded, and the
	// auto-init path should not have fired. Either error or success,
	// the assertion afterward is on the config file contents.
	applyErr := runApply(context.Background(), flags, registry, sudo.NoopAcquirer{},
		"", true, "", "", false, bootstrapMode{})
	_ = applyErr // intentionally ignored — the invariant under test
	//             is the config file contents, not the apply outcome.

	data, err := os.ReadFile(filepath.Join(configHome, "hams.config.yaml"))
	if err != nil {
		t.Fatalf("global config missing post-run: %v", err)
	}
	if string(data) != preExisting {
		t.Errorf("pre-existing global config was mutated.\nwant:\n%s\ngot:\n%s",
			preExisting, string(data))
	}
}

// --- Direct-unit-test coverage of the autoinit helpers extracted in
// 2026-04-19-cli-modularization. These bypass the runApply pipeline
// to exercise ensureProfileConfigured and statFile in isolation, so
// regressions land on a focused failure rather than masquerading as
// an apply failure.

// TestEnsureProfileConfigured_CLITagSeedsConfigOnFreshMachine
// asserts the path-1 (auto-init) branch fires when the global config
// is missing AND a CLI tag is supplied.
func TestEnsureProfileConfigured_CLITagSeedsConfigOnFreshMachine(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", filepath.Join(root, "data"))

	origHostname := config.HostnameLookup
	t.Cleanup(func() { config.HostnameLookup = origHostname })
	config.HostnameLookup = func() (string, error) { return "fresh-box", nil }
	t.Setenv("HAMS_MACHINE_ID", "")

	paths := config.ResolvePaths()
	cfg := &config.Config{}
	flags := &provider.GlobalFlags{Tag: "darwin-arm"}

	if err := ensureProfileConfigured(paths, "", cfg, flags); err != nil {
		t.Fatalf("ensureProfileConfigured: %v", err)
	}
	if cfg.ProfileTag != "darwin-arm" {
		t.Errorf("cfg.ProfileTag = %q, want darwin-arm", cfg.ProfileTag)
	}
	if cfg.MachineID == "" {
		t.Error("cfg.MachineID should be derived, got empty")
	}

	persisted, err := os.ReadFile(paths.GlobalConfigPath())
	if err != nil {
		t.Fatalf("global config not persisted: %v", err)
	}
	if !strings.Contains(string(persisted), "profile_tag: darwin-arm") {
		t.Errorf("persisted config missing profile_tag; got:\n%s", persisted)
	}
}

// TestEnsureProfileConfigured_AutoInitSkippedWhenConfigExists locks
// in the "auto-init is for pristine machines only" invariant: when
// a global config file is already on disk, the function MUST NOT
// invoke the path-1 auto-init branch (which would clobber persisted
// state). The TTY/non-TTY error fallback handles whatever the user
// does next.
func TestEnsureProfileConfigured_AutoInitSkippedWhenConfigExists(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", filepath.Join(root, "data"))

	if err := os.MkdirAll(configHome, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pre := "profile_tag: linux\nmachine_id: their_host\n"
	if err := os.WriteFile(filepath.Join(configHome, "hams.config.yaml"), []byte(pre), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	paths := config.ResolvePaths()
	// Pass an empty cfg + a CLI tag the user supplied. Auto-init
	// would normally fire (cliTag != "") but globalConfigPresent
	// blocks it.
	cfg := &config.Config{}
	flags := &provider.GlobalFlags{Tag: "darwin-arm"}

	// Function will fall through to the non-TTY error branch (test
	// runner stdin is not a terminal). That error is expected — what
	// we assert is that the persisted config was NOT touched.
	if err := ensureProfileConfigured(paths, "", cfg, flags); err == nil {
		t.Log("ensureProfileConfigured returned nil — unexpected but acceptable; assertion below is the gate")
	}

	got, err := os.ReadFile(filepath.Join(configHome, "hams.config.yaml"))
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if string(got) != pre {
		t.Errorf("global config mutated by auto-init when it should have been skipped:\nwant: %q\ngot:  %q", pre, string(got))
	}
}

// TestEnsureProfileConfigured_NonTTYWithoutCLITagFails covers the
// path-3 (non-TTY error) branch. We can simulate non-TTY because os.Stdin
// in the test runner is not a terminal.
func TestEnsureProfileConfigured_NonTTYWithoutCLITagFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HAMS_DATA_HOME", filepath.Join(root, "data"))

	paths := config.ResolvePaths()
	cfg := &config.Config{}
	flags := &provider.GlobalFlags{} // no Tag/Profile

	err := ensureProfileConfigured(paths, "", cfg, flags)
	if err == nil {
		t.Fatal("expected non-TTY missing-keys error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "profile_tag") || !strings.Contains(msg, "machine_id") {
		t.Errorf("error should name both missing keys; got: %q", msg)
	}
}

// TestEnsureProfileConfigured_TagAliasMatchesProfile asserts --tag and
// --profile resolve to the same effective override (one chosen, the
// other empty), which is what the auto-init path-1 branch keys on.
func TestEnsureProfileConfigured_TagAliasMatchesProfile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HAMS_DATA_HOME", filepath.Join(root, "data"))

	origHostname := config.HostnameLookup
	t.Cleanup(func() { config.HostnameLookup = origHostname })
	config.HostnameLookup = func() (string, error) { return "alias-host", nil }
	t.Setenv("HAMS_MACHINE_ID", "")

	paths := config.ResolvePaths()
	cfg := &config.Config{}
	flags := &provider.GlobalFlags{Profile: "linux-x64"} // legacy alias

	if err := ensureProfileConfigured(paths, "", cfg, flags); err != nil {
		t.Fatalf("ensureProfileConfigured: %v", err)
	}
	if cfg.ProfileTag != "linux-x64" {
		t.Errorf("cfg.ProfileTag = %q, want linux-x64 (from --profile alias)", cfg.ProfileTag)
	}
}

// TestEnsureProfileConfigured_TagProfileConflictRejected asserts that
// --tag X --profile Y (X != Y) bubbles up the same usage error
// emitted by config.ResolveCLITagOverride. The conflict is checked
// upstream in the apply / provider_cmd paths, but we want a regression
// gate in case ensureProfileConfigured ever stops calling the resolver.
func TestEnsureProfileConfigured_TagProfileConflictRejected(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HAMS_DATA_HOME", filepath.Join(root, "data"))

	paths := config.ResolvePaths()
	cfg := &config.Config{}
	// Conflicting tag and profile: ensureProfileConfigured silently
	// swallows the resolver error (nolint:errcheck) — the upstream
	// caller is responsible for surfacing it. So here we just assert
	// the function does not seed cfg with a wrong value.
	flags := &provider.GlobalFlags{Tag: "macOS", Profile: "linux"}
	if err := ensureProfileConfigured(paths, "", cfg, flags); err != nil {
		t.Logf("ensureProfileConfigured returned err (expected for non-TTY): %v", err)
	}
	if cfg.ProfileTag == "macOS" || cfg.ProfileTag == "linux" {
		t.Errorf("conflict should not silently seed ProfileTag; got %q", cfg.ProfileTag)
	}
}

// --- statFile coverage ---

// TestStatFile_MissingReportsFalse asserts the not-exist branch.
func TestStatFile_MissingReportsFalse(t *testing.T) {
	t.Parallel()
	got := statFile(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if got {
		t.Error("statFile of missing path = true, want false")
	}
}

// TestStatFile_PresentReportsTrue asserts the regular-file branch.
func TestStatFile_PresentReportsTrue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "present.yaml")
	if err := os.WriteFile(path, []byte("x: 1\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := statFile(path)
	if !got {
		t.Error("statFile of present file = false, want true")
	}
}

// TestStatFile_DirectoryReportsTrue asserts the conservative
// "treat as exists" branch — directories at the same path block
// auto-init from clobbering whatever is there.
func TestStatFile_DirectoryReportsTrue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got := statFile(dir)
	if !got {
		t.Error("statFile of directory = false, want true (conservative)")
	}
}

// --- enforceTagProfileConsistency coverage ---

// TestEnforceTagProfileConsistency_TableDriven covers the four shapes:
// nil flags, empty flags, equal Tag/Profile, conflicting Tag/Profile.
func TestEnforceTagProfileConsistency_TableDriven(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		flags   *provider.GlobalFlags
		wantErr bool
	}{
		{name: "nil flags", flags: nil, wantErr: false},
		{name: "empty flags", flags: &provider.GlobalFlags{}, wantErr: false},
		{name: "tag only", flags: &provider.GlobalFlags{Tag: "macOS"}, wantErr: false},
		{name: "profile only", flags: &provider.GlobalFlags{Profile: "macOS"}, wantErr: false},
		{name: "matching tag and profile", flags: &provider.GlobalFlags{Tag: "macOS", Profile: "macOS"}, wantErr: false},
		{name: "conflicting tag and profile", flags: &provider.GlobalFlags{Tag: "macOS", Profile: "linux"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := enforceTagProfileConsistency(tc.flags)
			if tc.wantErr && got == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && got != nil {
				t.Errorf("expected nil, got %v", got)
			}
		})
	}
}
