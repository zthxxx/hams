package vscodeext

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

type vscodeextHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newVscodeextHarness(t *testing.T) *vscodeextHarness {
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
	return &vscodeextHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "code.hams.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *vscodeextHarness) hamsfileApps() []string {
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

func (h *vscodeextHarness) stateResource(id string) state.ResourceState {
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

func TestHandleCommand_U1_InstallAddsExtensionToHamsfile(t *testing.T) {
	h := newVscodeextHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "golang.go"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "golang.go") != 1 {
		t.Errorf("runner.Install calls = %d", h.runner.CallCount(fakeOpInstall, "golang.go"))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "golang.go" {
		t.Errorf("hamsfile apps = %v", apps)
	}
}

func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newVscodeextHarness(t)
	for range 2 {
		if err := h.provider.HandleCommand(context.Background(),
			[]string{"install", "esbenp.prettier-vscode"}, nil, h.flags); err != nil {
			t.Fatalf("install: %v", err)
		}
	}
	if apps := h.hamsfileApps(); len(apps) != 1 {
		t.Errorf("want single entry, got %v", apps)
	}
}

func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newVscodeextHarness(t)
	h.runner.WithInstallError("bogus.extension", errors.New("VS Code: extension not found"))
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "bogus.extension"}, nil, h.flags); err == nil {
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
	h := newVscodeextHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "golang.go"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"remove", "golang.go"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty, got %v", apps)
	}
	if h.runner.CallCount(fakeOpUninstall, "golang.go") != 1 {
		t.Errorf("runner.Uninstall calls = %d", h.runner.CallCount(fakeOpUninstall, "golang.go"))
	}
}

func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newVscodeextHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "golang.go"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	h.runner.WithUninstallError("golang.go", errors.New("VS Code: uninstall failed"))
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"remove", "golang.go"}, nil, h.flags); err == nil {
		t.Fatal("expected remove error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "golang.go" {
		t.Errorf("hamsfile should still contain extension, got %v", apps)
	}
}

func TestHandleCommand_U6_DryRunSkipsRunnerAndHamsfile(t *testing.T) {
	h := newVscodeextHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "golang.go"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called on dry-run")
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}
}

func TestHandleCommand_U7_MultiExtensionInstallRecordsAll(t *testing.T) {
	h := newVscodeextHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "golang.go", "esbenp.prettier-vscode", "ms-python.python"}, nil, h.flags); err != nil {
		t.Fatalf("multi install: %v", err)
	}
	apps := h.hamsfileApps()
	want := map[string]bool{
		"golang.go":              true,
		"esbenp.prettier-vscode": true,
		"ms-python.python":       true,
	}
	for _, a := range apps {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("missing extensions: %v", want)
	}
}

func TestHandleCommand_U8_MultiExtensionInstallAtomicOnFailure(t *testing.T) {
	h := newVscodeextHarness(t)
	h.runner.WithInstallError("bogus.ext", errors.New("fail"))
	err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "golang.go", "bogus.ext"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on atomic failure, got %v", apps)
	}
}

func TestHandleCommand_U9_FlagsNotRecordedAsExtensions(t *testing.T) {
	h := newVscodeextHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "--force", "golang.go"}, nil, h.flags); err != nil {
		t.Fatalf("flagged install: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "golang.go" {
		t.Errorf("hamsfile apps = %v, want [golang.go]", apps)
	}
}

func TestHandleCommand_U10_FlagsOnlyReturnsUsage(t *testing.T) {
	h := newVscodeextHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "--force"}, nil, h.flags); err == nil {
		t.Fatal("expected usage error")
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called")
	}
}

// TestHandleCommand_U11_InstallWritesStateFile — cycle 208.
// Last Package-class provider to gain state-write parity with apt/brew.
func TestHandleCommand_U11_InstallWritesStateFile(t *testing.T) {
	h := newVscodeextHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "golang.go"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := h.stateResource("golang.go"); got != state.StateOK {
		t.Errorf("state[golang.go] = %q, want %q", got, state.StateOK)
	}
}

// TestHandleCommand_U12_RemoveMarksStateRemoved — tombstone on uninstall.
func TestHandleCommand_U12_RemoveMarksStateRemoved(t *testing.T) {
	h := newVscodeextHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "esbenp.prettier-vscode"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "esbenp.prettier-vscode"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := h.stateResource("esbenp.prettier-vscode"); got != state.StateRemoved {
		t.Errorf("state[esbenp.prettier-vscode] = %q, want %q", got, state.StateRemoved)
	}
}

// TestHandleCommand_U13_InstallFailureLeavesStateUntouched.
func TestHandleCommand_U13_InstallFailureLeavesStateUntouched(t *testing.T) {
	h := newVscodeextHarness(t)
	h.runner.WithInstallError("bogus.ext", errors.New("code: ext not found"))
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bogus.ext"}, nil, h.flags); err == nil {
		t.Fatal("expected install error")
	}
	if got := h.stateResource("bogus.ext"); got != "" {
		t.Errorf("state should not have an entry, got %q", got)
	}
}

// TestHandleCommand_U14_DryRunSkipsStateWrite.
func TestHandleCommand_U14_DryRunSkipsStateWrite(t *testing.T) {
	h := newVscodeextHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "golang.go"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(h.provider.statePath(h.flags)); err == nil {
		t.Error("dry-run should not create state file")
	}
}
