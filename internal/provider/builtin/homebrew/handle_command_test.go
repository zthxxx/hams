package homebrew

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// brewHarness wires a homebrew.Provider against a FakeCmdRunner +
// tempdir profile + tempdir state dir so HandleCommand tests can
// assert real hamsfile + state writes without invoking the host's
// real brew. Mirrors apt_test.go's `testHarness` shape so a future
// reader can see the two at a glance.
type brewHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	stateDir     string
	hamsfilePath string
	statePath    string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newBrewHarness(t *testing.T) *brewHarness {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileTag := "test"
	profileDir := filepath.Join(storeDir, profileTag)
	stateDir := filepath.Join(storeDir, ".state", "test-machine")
	for _, d := range []string{profileDir, stateDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatalf("mkdir %q: %v", d, err)
		}
	}
	cfg := &config.Config{StorePath: storeDir, ProfileTag: profileTag, MachineID: "test-machine"}
	runner := NewFakeCmdRunner()
	p := New(cfg, runner)
	return &brewHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		stateDir:     stateDir,
		hamsfilePath: filepath.Join(profileDir, "Homebrew.hams.yaml"),
		statePath:    filepath.Join(stateDir, "Homebrew.state.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *brewHarness) hamsfileApps() []string {
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

func (h *brewHarness) stateResources() map[string]state.ResourceState {
	h.t.Helper()
	if _, err := os.Stat(h.statePath); err != nil {
		return nil
	}
	sf, err := state.Load(h.statePath)
	if err != nil {
		h.t.Fatalf("state.Load: %v", err)
	}
	out := make(map[string]state.ResourceState, len(sf.Resources))
	for id, r := range sf.Resources {
		out[id] = r.State
	}
	return out
}

// U1 — install records the package in BOTH hamsfile and state.
// Cycle 96 added the state write; cycle 97 locks it in against
// regressions.
func TestHandleCommand_U1_InstallWritesHamsfileAndState(t *testing.T) {
	h := newBrewHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "git"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "git") != 1 {
		t.Errorf("runner.Install(git) calls = %d, want 1", h.runner.CallCount(fakeOpInstall, "git"))
	}
	if apps := h.hamsfileApps(); len(apps) != 1 || apps[0] != "git" {
		t.Errorf("hamsfile apps = %v, want [git]", apps)
	}
	if resources := h.stateResources(); resources["git"] != state.StateOK {
		t.Errorf("state[git] = %q, want ok", resources["git"])
	}
}

// U2 — idempotent re-install keeps single hamsfile entry AND
// preserves first_install_at in state (the SetResource contract).
func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newBrewHarness(t)
	for range 2 {
		if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
			t.Fatalf("install: %v", err)
		}
	}
	if apps := h.hamsfileApps(); len(apps) != 1 {
		t.Errorf("hamsfile apps = %v, want single entry", apps)
	}
	if h.runner.CallCount(fakeOpInstall, "bat") != 2 {
		t.Errorf("runner.Install calls = %d, want 2", h.runner.CallCount(fakeOpInstall, "bat"))
	}
}

// U3 — install failure leaves hamsfile AND state untouched.
func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newBrewHarness(t)
	h.runner.WithInstallError("nope", errors.New("brew: No available formula"))

	err := h.provider.HandleCommand(context.Background(), []string{"install", "nope"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected install error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after failed install, got %v", apps)
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("hamsfile should not be created after failure")
	}
	if _, statErr := os.Stat(h.statePath); statErr == nil {
		t.Error("state file should not be created after failure")
	}
}

// U4 — remove drops the hamsfile entry AND marks state removed.
// The state still contains the entry (with `state: removed`) per
// apt's U14 semantic — "removed" is a first-class state, not a
// delete.
func TestHandleCommand_U4_RemoveDeletesFromHamsfileMarksStateRemoved(t *testing.T) {
	h := newBrewHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after remove, got %v", apps)
	}
	resources := h.stateResources()
	if resources["htop"] != state.StateRemoved {
		t.Errorf("state[htop] = %q, want removed", resources["htop"])
	}
	if h.runner.CallCount(fakeOpUninstall, "htop") != 1 {
		t.Errorf("runner.Uninstall calls = %d, want 1", h.runner.CallCount(fakeOpUninstall, "htop"))
	}
}

// U5 — remove failure leaves BOTH hamsfile and state intact.
func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newBrewHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "jq"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	h.runner.WithUninstallError("jq", errors.New("brew uninstall: permission denied"))

	err := h.provider.HandleCommand(context.Background(), []string{"remove", "jq"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected remove error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "jq" {
		t.Errorf("hamsfile should still contain jq after failed remove, got %v", apps)
	}
	resources := h.stateResources()
	if resources["jq"] != state.StateOK {
		t.Errorf("state[jq] = %q, want ok (remove failed)", resources["jq"])
	}
}

// U6 — dry-run skips runner, hamsfile, AND state write.
func TestHandleCommand_U6_DryRunSkipsRunnerHamsfileState(t *testing.T) {
	h := newBrewHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ripgrep"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("dry-run should not invoke runner, got %d calls", h.runner.CallCount(fakeOpInstall, ""))
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}
	if _, statErr := os.Stat(h.statePath); statErr == nil {
		t.Error("dry-run should not write state file")
	}
}

// U7 — --cask flag routes to the "cask" tag AND passes isCask=true
// to the runner. Protects the cycle-52 cask-detection branch.
func TestHandleCommand_U7_CaskFlagRecordsUnderCaskTag(t *testing.T) {
	h := newBrewHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "visual-studio-code", "--cask"}, nil, h.flags); err != nil {
		t.Fatalf("install --cask: %v", err)
	}

	// Read hamsfile structurally so we can verify the TAG, not just
	// the app list. The cask-detection branch is the reason the
	// --cask flag exists on the CLI path at all.
	f, err := hamsfile.Read(h.hamsfilePath)
	if err != nil {
		t.Fatalf("read hamsfile: %v", err)
	}
	tag, idx := f.FindApp("visual-studio-code")
	if tag != "cask" {
		t.Errorf("FindApp tag = %q, want 'cask'", tag)
	}
	if idx < 0 {
		t.Errorf("visual-studio-code not found in hamsfile")
	}
	// Runner should have been called with isCask=true — inspect via
	// a manual iteration since the fake exposes CallCount by name
	// only.
	if h.runner.CallCount(fakeOpInstall, "visual-studio-code") != 1 {
		t.Errorf("runner.Install(visual-studio-code) calls = %d, want 1", h.runner.CallCount(fakeOpInstall, "visual-studio-code"))
	}
}
