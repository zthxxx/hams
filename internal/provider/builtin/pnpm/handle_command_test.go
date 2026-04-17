package pnpm

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

type pnpmHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newPnpmHarness(t *testing.T) *pnpmHarness {
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
	return &pnpmHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "pnpm.hams.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *pnpmHarness) hamsfileApps() []string {
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
func (h *pnpmHarness) stateResource(id string) state.ResourceState {
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

func TestHandleCommand_U1_InstallAddsPackageToHamsfile(t *testing.T) {
	h := newPnpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "typescript"}, nil, h.flags); err != nil {
		t.Fatalf("add: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "typescript") != 1 {
		t.Errorf("runner.Install calls = %d", h.runner.CallCount(fakeOpInstall, "typescript"))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "typescript" {
		t.Errorf("hamsfile apps = %v", apps)
	}
}

func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newPnpmHarness(t)
	for range 2 {
		if err := h.provider.HandleCommand(context.Background(), []string{"add", "prettier"}, nil, h.flags); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if apps := h.hamsfileApps(); len(apps) != 1 {
		t.Errorf("want single entry, got %v", apps)
	}
	if h.runner.CallCount(fakeOpInstall, "prettier") != 2 {
		t.Errorf("runner calls = %d, want 2", h.runner.CallCount(fakeOpInstall, "prettier"))
	}
}

func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newPnpmHarness(t)
	h.runner.WithInstallError("nope", errors.New("pnpm ERR! 404"))
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "nope"}, nil, h.flags); err == nil {
		t.Fatal("expected install error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty, got %v", apps)
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("hamsfile should not be created")
	}
}

func TestHandleCommand_U4_RemoveDeletesFromHamsfile(t *testing.T) {
	h := newPnpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "eslint"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "eslint"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty, got %v", apps)
	}
	if h.runner.CallCount(fakeOpUninstall, "eslint") != 1 {
		t.Errorf("runner.Uninstall calls = %d", h.runner.CallCount(fakeOpUninstall, "eslint"))
	}
}

func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newPnpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "yarn"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	h.runner.WithUninstallError("yarn", errors.New("permission denied"))
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "yarn"}, nil, h.flags); err == nil {
		t.Fatal("expected error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "yarn" {
		t.Errorf("hamsfile should still contain yarn, got %v", apps)
	}
}

func TestHandleCommand_U6_DryRunSkipsRunnerAndHamsfile(t *testing.T) {
	h := newPnpmHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "pnpm"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called on dry-run")
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}
}

func TestHandleCommand_U7_MultiPackageInstallRecordsAll(t *testing.T) {
	h := newPnpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "typescript", "prettier"}, nil, h.flags); err != nil {
		t.Fatalf("multi add: %v", err)
	}
	apps := h.hamsfileApps()
	want := map[string]bool{"typescript": true, "prettier": true}
	for _, a := range apps {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("missing pkgs: %v", want)
	}
}

func TestHandleCommand_U8_MultiPackageInstallAtomicOnFailure(t *testing.T) {
	h := newPnpmHarness(t)
	h.runner.WithInstallError("broken", errors.New("404"))
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "ok-pkg", "broken"}, nil, h.flags); err == nil {
		t.Fatal("expected error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on atomic failure, got %v", apps)
	}
}

func TestHandleCommand_U9_FlagsNotRecordedAsPackages(t *testing.T) {
	h := newPnpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "--save-exact", "typescript"}, nil, h.flags); err != nil {
		t.Fatalf("add: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "typescript" {
		t.Errorf("apps = %v, want [typescript]", apps)
	}
}

func TestHandleCommand_U10_FlagsOnlyReturnsUsage(t *testing.T) {
	h := newPnpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "--save-exact"}, nil, h.flags); err == nil {
		t.Fatal("expected usage error")
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called")
	}
}

// TestHandleCommand_U11_InstallWritesStateFile — cycle 205.
// `hams pnpm install <pkg>` now writes StateOK to pnpm.state.yaml
// so `hams list --only=pnpm` returns the package immediately.
// Same auto-record gap as cycle 96/202/203/204.
func TestHandleCommand_U11_InstallWritesStateFile(t *testing.T) {
	h := newPnpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "serve"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := h.stateResource("serve"); got != state.StateOK {
		t.Errorf("state[serve] = %q, want %q", got, state.StateOK)
	}
}

// TestHandleCommand_U12_RemoveMarksStateRemoved asserts symmetric
// StateRemoved tombstone on successful uninstall.
func TestHandleCommand_U12_RemoveMarksStateRemoved(t *testing.T) {
	h := newPnpmHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "typescript"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "typescript"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := h.stateResource("typescript"); got != state.StateRemoved {
		t.Errorf("state[typescript] = %q, want %q", got, state.StateRemoved)
	}
}

// TestHandleCommand_U13_InstallFailureLeavesStateUntouched: a runner
// rejection must NOT produce a spurious StateOK entry.
func TestHandleCommand_U13_InstallFailureLeavesStateUntouched(t *testing.T) {
	h := newPnpmHarness(t)
	h.runner.WithInstallError("bogus-pkg", errors.New("pnpm: not found"))
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "bogus-pkg"}, nil, h.flags); err == nil {
		t.Fatal("expected install error")
	}
	if got := h.stateResource("bogus-pkg"); got != "" {
		t.Errorf("state should not have an entry, got %q", got)
	}
}

// TestHandleCommand_U14_DryRunSkipsStateWrite: --dry-run must not
// create an on-disk state file.
func TestHandleCommand_U14_DryRunSkipsStateWrite(t *testing.T) {
	h := newPnpmHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"add", "serve"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(h.provider.statePath(h.flags)); err == nil {
		t.Error("dry-run should not create state file")
	}
}
