package mas

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

type masHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newMasHarness(t *testing.T) *masHarness {
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
	return &masHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "mas.hams.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *masHarness) hamsfileApps() []string {
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

func TestHandleCommand_U1_InstallAddsAppIDToHamsfile(t *testing.T) {
	h := newMasHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "497799835"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "497799835") != 1 {
		t.Errorf("runner.Install calls = %d", h.runner.CallCount(fakeOpInstall, "497799835"))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "497799835" {
		t.Errorf("hamsfile apps = %v", apps)
	}
}

func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newMasHarness(t)
	for range 2 {
		if err := h.provider.HandleCommand(context.Background(), []string{"install", "1444383602"}, nil, h.flags); err != nil {
			t.Fatalf("install: %v", err)
		}
	}
	if apps := h.hamsfileApps(); len(apps) != 1 {
		t.Errorf("want single entry, got %v", apps)
	}
}

func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newMasHarness(t)
	h.runner.WithInstallError("999999999", errors.New("mas: app not found"))
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "999999999"}, nil, h.flags); err == nil {
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
	h := newMasHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "497799835"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "497799835"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty, got %v", apps)
	}
	if h.runner.CallCount(fakeOpUninstall, "497799835") != 1 {
		t.Errorf("runner.Uninstall calls = %d", h.runner.CallCount(fakeOpUninstall, "497799835"))
	}
}

func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newMasHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "1444383602"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	h.runner.WithUninstallError("1444383602", errors.New("mas: permission denied"))
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "1444383602"}, nil, h.flags); err == nil {
		t.Fatal("expected remove error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "1444383602" {
		t.Errorf("hamsfile should still contain app, got %v", apps)
	}
}

func TestHandleCommand_U6_DryRunSkipsRunnerAndHamsfile(t *testing.T) {
	h := newMasHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "497799835"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called on dry-run")
	}
	if _, statErr := os.Stat(h.hamsfilePath); statErr == nil {
		t.Error("dry-run should not write hamsfile")
	}
}

func TestHandleCommand_U7_MultiAppInstallRecordsAll(t *testing.T) {
	h := newMasHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "497799835", "1444383602"}, nil, h.flags); err != nil {
		t.Fatalf("multi install: %v", err)
	}
	apps := h.hamsfileApps()
	want := map[string]bool{"497799835": true, "1444383602": true}
	for _, a := range apps {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("missing apps: %v", want)
	}
}

func TestHandleCommand_U8_MultiAppInstallAtomicOnFailure(t *testing.T) {
	h := newMasHarness(t)
	h.runner.WithInstallError("999999999", errors.New("fail"))
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "497799835", "999999999"}, nil, h.flags); err == nil {
		t.Fatal("expected error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on atomic failure, got %v", apps)
	}
}

func TestHandleCommand_U9_FlagsNotRecordedAsAppIDs(t *testing.T) {
	h := newMasHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "--verbose", "497799835"}, nil, h.flags); err != nil {
		t.Fatalf("flagged install: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "497799835" {
		t.Errorf("hamsfile apps = %v, want [497799835]", apps)
	}
}

func TestHandleCommand_U10_FlagsOnlyReturnsUsage(t *testing.T) {
	h := newMasHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "--verbose"}, nil, h.flags); err == nil {
		t.Fatal("expected usage error")
	}
	if h.runner.CallCount(fakeOpInstall, "") != 0 {
		t.Errorf("runner should not be called")
	}
}
