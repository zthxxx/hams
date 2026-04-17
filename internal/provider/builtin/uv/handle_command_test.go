package uv

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

type uvHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newUvHarness(t *testing.T) *uvHarness {
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
	return &uvHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "uv.hams.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *uvHarness) hamsfileApps() []string {
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

func (h *uvHarness) stateResource(id string) state.ResourceState {
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

func TestHandleCommand_U1_InstallAddsToolToHamsfile(t *testing.T) {
	h := newUvHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ruff"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "ruff") != 1 {
		t.Errorf("runner.Install calls = %d", h.runner.CallCount(fakeOpInstall, "ruff"))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "ruff" {
		t.Errorf("hamsfile apps = %v", apps)
	}
}

func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newUvHarness(t)
	for range 2 {
		if err := h.provider.HandleCommand(context.Background(), []string{"install", "black"}, nil, h.flags); err != nil {
			t.Fatalf("install: %v", err)
		}
	}
	if apps := h.hamsfileApps(); len(apps) != 1 {
		t.Errorf("want single entry, got %v", apps)
	}
	if h.runner.CallCount(fakeOpInstall, "black") != 2 {
		t.Errorf("runner calls = %d, want 2", h.runner.CallCount(fakeOpInstall, "black"))
	}
}

func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newUvHarness(t)
	h.runner.WithInstallError("nope-tool", errors.New("uv: error"))
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "nope-tool"}, nil, h.flags); err == nil {
		t.Fatal("expected error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty, got %v", apps)
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("hamsfile should not be created")
	}
}

func TestHandleCommand_U4_RemoveDeletesFromHamsfile(t *testing.T) {
	h := newUvHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "mypy"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "mypy"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty, got %v", apps)
	}
	if h.runner.CallCount(fakeOpUninstall, "mypy") != 1 {
		t.Errorf("runner.Uninstall calls = %d", h.runner.CallCount(fakeOpUninstall, "mypy"))
	}
}

func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newUvHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ruff"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	h.runner.WithUninstallError("ruff", errors.New("uv: error"))
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "ruff"}, nil, h.flags); err == nil {
		t.Fatal("expected error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "ruff" {
		t.Errorf("hamsfile should still contain ruff, got %v", apps)
	}
}

func TestHandleCommand_U6_DryRunSkipsRunnerAndHamsfile(t *testing.T) {
	h := newUvHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ruff"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called on dry-run")
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}
}

func TestHandleCommand_U7_MultiToolInstallRecordsAll(t *testing.T) {
	h := newUvHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ruff", "black", "mypy"}, nil, h.flags); err != nil {
		t.Fatalf("multi install: %v", err)
	}
	apps := h.hamsfileApps()
	want := map[string]bool{"ruff": true, "black": true, "mypy": true}
	for _, a := range apps {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("missing tools: %v", want)
	}
}

func TestHandleCommand_U8_MultiToolInstallAtomicOnFailure(t *testing.T) {
	h := newUvHarness(t)
	h.runner.WithInstallError("broken", errors.New("fail"))
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ok-tool", "broken"}, nil, h.flags); err == nil {
		t.Fatal("expected error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on atomic failure, got %v", apps)
	}
}

func TestHandleCommand_U9_FlagsNotRecordedAsTools(t *testing.T) {
	h := newUvHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "--python", "ruff"}, nil, h.flags); err != nil {
		t.Fatalf("flagged install: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "ruff" {
		t.Errorf("hamsfile apps = %v, want [ruff]", apps)
	}
}

func TestHandleCommand_U10_FlagsOnlyReturnsUsage(t *testing.T) {
	h := newUvHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "--python"}, nil, h.flags); err == nil {
		t.Fatal("expected usage error")
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called")
	}
}

// TestHandleCommand_U11_InstallWritesStateFile — cycle 206.
// `hams uv install <tool>` writes StateOK to uv.state.yaml so
// `hams list --only=uv` returns the tool immediately.
func TestHandleCommand_U11_InstallWritesStateFile(t *testing.T) {
	h := newUvHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ruff"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := h.stateResource("ruff"); got != state.StateOK {
		t.Errorf("state[ruff] = %q, want %q", got, state.StateOK)
	}
}

// TestHandleCommand_U12_RemoveMarksStateRemoved — tombstone on uninstall.
func TestHandleCommand_U12_RemoveMarksStateRemoved(t *testing.T) {
	h := newUvHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "black"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "black"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := h.stateResource("black"); got != state.StateRemoved {
		t.Errorf("state[black] = %q, want %q", got, state.StateRemoved)
	}
}

// TestHandleCommand_U13_InstallFailureLeavesStateUntouched.
func TestHandleCommand_U13_InstallFailureLeavesStateUntouched(t *testing.T) {
	h := newUvHarness(t)
	h.runner.WithInstallError("does-not-exist", errors.New("uv: not found"))
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "does-not-exist"}, nil, h.flags); err == nil {
		t.Fatal("expected install error")
	}
	if got := h.stateResource("does-not-exist"); got != "" {
		t.Errorf("state should not have an entry, got %q", got)
	}
}

// TestHandleCommand_U14_DryRunSkipsStateWrite.
func TestHandleCommand_U14_DryRunSkipsStateWrite(t *testing.T) {
	h := newUvHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "ruff"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(h.provider.statePath(h.flags)); err == nil {
		t.Error("dry-run should not create state file")
	}
}
