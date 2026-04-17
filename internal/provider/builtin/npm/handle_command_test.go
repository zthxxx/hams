package npm

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

type npmHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newNpmHarness(t *testing.T) *npmHarness {
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
	return &npmHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "npm.hams.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *npmHarness) hamsfileApps() []string {
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

// stateResource reads the on-disk state file and returns the state of
// the named resource, or "" if the file or resource doesn't exist.
func (h *npmHarness) stateResource(id string) state.ResourceState {
	h.t.Helper()
	sf, err := state.Load(h.provider.statePath(h.flags))
	if err != nil {
		return ""
	}
	if r, ok := sf.Resources[id]; ok {
		return r.State
	}
	return ""
}

// U1 — first install records + calls runner once.
func TestHandleCommand_U1_InstallAddsPackageToHamsfile(t *testing.T) {
	h := newNpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "typescript"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "typescript") != 1 {
		t.Errorf("runner.Install calls = %d, want 1", h.runner.CallCount(fakeOpInstall, "typescript"))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "typescript" {
		t.Errorf("hamsfile apps = %v, want [typescript]", apps)
	}
}

// U2 — re-install is idempotent on the hamsfile.
func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newNpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "prettier"}, nil, h.flags); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "prettier"}, nil, h.flags); err != nil {
		t.Fatalf("second install: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 {
		t.Errorf("hamsfile apps = %v, want single entry", apps)
	}
	if h.runner.CallCount(fakeOpInstall, "prettier") != 2 {
		t.Errorf("runner.Install calls = %d, want 2", h.runner.CallCount(fakeOpInstall, "prettier"))
	}
}

// U3 — install failure leaves hamsfile untouched.
func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newNpmHarness(t)
	h.runner.WithInstallError("nonexistent-pkg-xyz", errors.New("npm ERR! 404"))

	err := h.provider.HandleCommand(context.Background(), []string{"install", "nonexistent-pkg-xyz"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected install error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after failed install, got %v", apps)
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("hamsfile should not be created after install failure")
	}
}

// U4 — remove deletes the entry.
func TestHandleCommand_U4_RemoveDeletesFromHamsfile(t *testing.T) {
	h := newNpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "eslint"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "eslint"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after remove, got %v", apps)
	}
	if h.runner.CallCount(fakeOpUninstall, "eslint") != 1 {
		t.Errorf("runner.Uninstall calls = %d, want 1", h.runner.CallCount(fakeOpUninstall, "eslint"))
	}
}

// U5 — remove failure leaves the hamsfile untouched.
func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newNpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "yarn"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	h.runner.WithUninstallError("yarn", errors.New("permission denied"))
	err := h.provider.HandleCommand(context.Background(), []string{"remove", "yarn"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected remove error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "yarn" {
		t.Errorf("hamsfile should still contain yarn after failed remove, got %v", apps)
	}
}

// U6 — dry-run short-circuits the runner and the hamsfile write.
func TestHandleCommand_U6_DryRunSkipsRunnerAndHamsfile(t *testing.T) {
	h := newNpmHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "pnpm"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("dry-run should not invoke runner, got %d calls", h.runner.CallCount(fakeOpInstall, ""))
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}
}

// U7 — multi-pkg install records every pkg on full success.
func TestHandleCommand_U7_MultiPackageInstallRecordsAll(t *testing.T) {
	h := newNpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "typescript", "prettier", "eslint"}, nil, h.flags); err != nil {
		t.Fatalf("multi install: %v", err)
	}
	apps := h.hamsfileApps()
	want := map[string]bool{"typescript": true, "prettier": true, "eslint": true}
	for _, a := range apps {
		if !want[a] {
			t.Errorf("unexpected pkg in hamsfile: %q", a)
		}
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("missing pkgs from hamsfile: %v", want)
	}
}

// U8 — multi-pkg install is atomic on partial failure.
func TestHandleCommand_U8_MultiPackageInstallAtomicOnFailure(t *testing.T) {
	h := newNpmHarness(t)
	h.runner.WithInstallError("broken", errors.New("404"))
	err := h.provider.HandleCommand(context.Background(), []string{"install", "ok-pkg", "broken"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected error on partial failure")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on atomic failure, got %v", apps)
	}
}

// U9 — npm flags must not leak into the recorded entries.
func TestHandleCommand_U9_FlagsNotRecordedAsPackages(t *testing.T) {
	h := newNpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "--save-exact", "typescript"}, nil, h.flags); err != nil {
		t.Fatalf("flagged install: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "typescript" {
		t.Errorf("hamsfile apps = %v, want [typescript]", apps)
	}
}

// U10 — flags-only install returns a usage error without calling the runner.
func TestHandleCommand_U10_FlagsOnlyReturnsUsage(t *testing.T) {
	h := newNpmHarness(t)
	err := h.provider.HandleCommand(context.Background(), []string{"install", "--save-exact"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected usage error when only flags are given")
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called on usage error, got %d", h.runner.CallCount(fakeOpInstall, ""))
	}
}

// TestHandleCommand_U11_InstallWritesStateFile locks in cycle 204:
// `hams npm install <pkg>` now writes a StateOK entry to npm.state.yaml
// so `hams list --only=npm` returns the package immediately after
// install. Same auto-record gap as cycle 96/202/203.
func TestHandleCommand_U11_InstallWritesStateFile(t *testing.T) {
	h := newNpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "typescript"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := h.stateResource("typescript"); got != state.StateOK {
		t.Errorf("state[typescript] = %q, want %q", got, state.StateOK)
	}
}

// TestHandleCommand_U12_RemoveMarksStateRemoved asserts the symmetric
// contract: a successful uninstall writes a StateRemoved tombstone.
func TestHandleCommand_U12_RemoveMarksStateRemoved(t *testing.T) {
	h := newNpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "prettier"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "prettier"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := h.stateResource("prettier"); got != state.StateRemoved {
		t.Errorf("state[prettier] = %q, want %q", got, state.StateRemoved)
	}
}

// TestHandleCommand_U13_InstallFailureLeavesStateUntouched: a runner
// rejection must NOT produce a spurious StateOK entry.
func TestHandleCommand_U13_InstallFailureLeavesStateUntouched(t *testing.T) {
	h := newNpmHarness(t)
	h.runner.WithInstallError("nonexistent-pkg", errors.New("npm: not found"))
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "nonexistent-pkg"}, nil, h.flags); err == nil {
		t.Fatal("expected install error")
	}
	if got := h.stateResource("nonexistent-pkg"); got != "" {
		t.Errorf("state should not have an entry, got %q", got)
	}
}

// TestHandleCommand_U14_DryRunSkipsStateWrite: --dry-run must not
// create an on-disk state file.
func TestHandleCommand_U14_DryRunSkipsStateWrite(t *testing.T) {
	h := newNpmHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "typescript"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(h.provider.statePath(h.flags)); err == nil {
		t.Error("dry-run should not create state file")
	}
}
