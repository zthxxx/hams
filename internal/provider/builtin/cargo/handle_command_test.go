package cargo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
)

// cargoHarness wires a cargo provider against a FakeCmdRunner +
// tempdir profile so HandleCommand tests can assert real hamsfile
// writes without ever touching the host's cargo.
type cargoHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newCargoHarness(t *testing.T) *cargoHarness {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileTag := "test"
	profileDir := filepath.Join(storeDir, profileTag)
	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	cfg := &config.Config{StorePath: storeDir, ProfileTag: profileTag, MachineID: "test-machine"}
	runner := NewFakeCmdRunner()
	p := New(cfg, runner)
	return &cargoHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "cargo.hams.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *cargoHarness) hamsfileApps() []string {
	h.t.Helper()
	if _, err := os.Stat(h.hamsfilePath); err != nil {
		return nil
	}
	f, err := hamsfile.Read(h.hamsfilePath)
	if err != nil {
		h.t.Fatalf("read hamsfile: %v", err)
	}
	return f.ListApps()
}

// U1 — first `hams cargo install <crate>` records the crate in the
// hamsfile and calls the runner exactly once.
func TestHandleCommand_U1_InstallAddsCrateToHamsfile(t *testing.T) {
	h := newCargoHarness(t)

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ripgrep"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "ripgrep") != 1 {
		t.Errorf("runner.Install called %d times, want 1", h.runner.CallCount(fakeOpInstall, "ripgrep"))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "ripgrep" {
		t.Errorf("hamsfile apps = %v, want [ripgrep]", apps)
	}
}

// U2 — repeated install of the same crate keeps exactly one hamsfile
// entry (AddApp is idempotent on duplicate names).
func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newCargoHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("second install: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 {
		t.Errorf("hamsfile apps = %v, want exactly one entry", apps)
	}
	if h.runner.CallCount(fakeOpInstall, "bat") != 2 {
		t.Errorf("runner.Install calls = %d, want 2", h.runner.CallCount(fakeOpInstall, "bat"))
	}
}

// U3 — when the runner returns an error, the hamsfile MUST NOT be
// modified. This is the spec's literal promise: exit-nonzero → no
// record so the user's host state matches the hamsfile after a retry.
func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newCargoHarness(t)
	h.runner.WithInstallError("nope", errors.New("cargo install nope: E0601"))

	err := h.provider.HandleCommand(context.Background(), []string{"install", "nope"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected install error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after failed install, got %v", apps)
	}
	// File should not exist on disk either.
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("hamsfile should not be created after install failure")
	}
}

// U4 — `hams cargo remove <crate>` drops the entry from the hamsfile
// and calls runner.Uninstall once.
func TestHandleCommand_U4_RemoveDeletesFromHamsfile(t *testing.T) {
	h := newCargoHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "fd-find"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "fd-find"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after remove, got %v", apps)
	}
	if h.runner.CallCount(fakeOpUninstall, "fd-find") != 1 {
		t.Errorf("runner.Uninstall calls = %d, want 1", h.runner.CallCount(fakeOpUninstall, "fd-find"))
	}
}

// U5 — remove failure MUST NOT de-record the entry. Otherwise the
// user would lose a valid hamsfile row because of a transient
// uninstall error.
func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newCargoHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "tokei"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	h.runner.WithUninstallError("tokei", errors.New("cargo uninstall tokei: permission denied"))

	err := h.provider.HandleCommand(context.Background(), []string{"remove", "tokei"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected remove error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "tokei" {
		t.Errorf("hamsfile should still contain tokei after failed remove, got %v", apps)
	}
}

// U6 — --dry-run prints the intended command but does NOT invoke
// the runner and does NOT write the hamsfile.
func TestHandleCommand_U6_DryRunSkipsRunnerAndHamsfile(t *testing.T) {
	h := newCargoHarness(t)
	h.flags.DryRun = true

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("dry-run should not invoke runner, got %d calls", h.runner.CallCount(fakeOpInstall, ""))
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}

	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run remove: %v", err)
	}
	if h.runner.CallCount(fakeOpUninstall, "") != 0 {
		t.Errorf("dry-run remove should not invoke runner, got %d calls", h.runner.CallCount(fakeOpUninstall, ""))
	}
}

// U7 — multi-crate install records every crate on full success.
// This isn't a scenario in the delta spec, but it locks in the
// intended semantic that crates are handled as a batch (fail
// atomically, record all on full success).
func TestHandleCommand_U7_MultiCrateInstallRecordsAll(t *testing.T) {
	h := newCargoHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ripgrep", "bat", "fd-find"}, nil, h.flags); err != nil {
		t.Fatalf("multi install: %v", err)
	}
	apps := h.hamsfileApps()
	want := map[string]bool{"ripgrep": true, "bat": true, "fd-find": true}
	for _, a := range apps {
		if !want[a] {
			t.Errorf("unexpected crate in hamsfile: %q", a)
		}
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("missing crates from hamsfile: %v", want)
	}
}

// U8 — multi-crate install aborts without recording if any crate
// fails to install, so the hamsfile never drifts from the host.
// This matches apt's all-or-nothing auto-record semantics.
func TestHandleCommand_U8_MultiCrateInstallAtomicOnFailure(t *testing.T) {
	h := newCargoHarness(t)
	h.runner.WithInstallError("broken", errors.New("cargo install broken: fail"))

	err := h.provider.HandleCommand(context.Background(), []string{"install", "bat", "broken"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected error on partial failure")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on atomic failure, got %v", apps)
	}
}

// U9 — `cargo install` flags (--locked, --features, ...) must be
// filtered from the recorded crate names. Otherwise the hamsfile
// would carry bogus entries like "--locked" that a later `hams
// apply` would fail to install.
func TestHandleCommand_U9_FlagsNotRecordedAsCrates(t *testing.T) {
	h := newCargoHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "--locked", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("flagged install: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "bat" {
		t.Errorf("hamsfile apps = %v, want [bat] (flag filtered)", apps)
	}
}

// U10 — empty args after flag filtering returns a clear usage error
// without invoking the runner.
func TestHandleCommand_U10_FlagsOnlyReturnsUsage(t *testing.T) {
	h := newCargoHarness(t)
	err := h.provider.HandleCommand(context.Background(), []string{"install", "--locked"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected usage error when only flags are given")
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called on usage error, got %d", h.runner.CallCount(fakeOpInstall, ""))
	}
}
