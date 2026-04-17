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
	"github.com/zthxxx/hams/internal/state"
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

// stateResource returns the state of a resource in the on-disk state file,
// or "" if the state file or resource doesn't exist.
func (h *masHarness) stateResource(id string) state.ResourceState {
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

// TestHandleCommand_U11_InstallWritesStateFile locks in cycle 201:
// after a successful `hams mas install <id>`, the state file MUST
// contain an OK entry for that ID. Without this, `hams list --only=mas`
// silently returned empty immediately after install (list reads state
// only) — the user had to run `hams refresh` first. Same
// auto-record-gap class as cycle 96 (homebrew) / cycle 97 refactor.
func TestHandleCommand_U11_InstallWritesStateFile(t *testing.T) {
	h := newMasHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "497799835"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := h.stateResource("497799835"); got != state.StateOK {
		t.Errorf("state[497799835] = %q, want %q", got, state.StateOK)
	}
}

// TestHandleCommand_U12_RemoveMarksStateRemoved locks in the symmetric
// contract: a successful `hams mas remove <id>` MUST mark the state
// resource as Removed (tombstone) so Probe/apply skip it on the next
// cycle. Pre-cycle-201 the state file was never updated by the CLI
// handler, so Probe would re-dispatch the removed ID as "desired but
// missing" until a subsequent `hams refresh`.
func TestHandleCommand_U12_RemoveMarksStateRemoved(t *testing.T) {
	h := newMasHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "1444383602"}, nil, h.flags); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "1444383602"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := h.stateResource("1444383602"); got != state.StateRemoved {
		t.Errorf("state[1444383602] = %q, want %q", got, state.StateRemoved)
	}
}

// TestHandleCommand_U13_InstallFailureLeavesStateUntouched ensures the
// atomic-on-failure contract also holds at the state layer: if the
// runner rejects an install, the state file MUST NOT gain a spurious
// OK entry. Matches the existing U3 contract for hamsfile.
func TestHandleCommand_U13_InstallFailureLeavesStateUntouched(t *testing.T) {
	h := newMasHarness(t)
	h.runner.WithInstallError("999999999", errors.New("mas: not found"))
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "999999999"}, nil, h.flags); err == nil {
		t.Fatal("expected install error")
	}
	if got := h.stateResource("999999999"); got != "" {
		t.Errorf("state should not have an entry, got %q", got)
	}
}

// TestHandleCommand_U14_DryRunSkipsStateWrite mirrors U6 at the state
// layer: a `--dry-run` install must not produce an on-disk state file.
// A silent state write during dry-run would violate the "--dry-run has
// zero side effects" contract (cycles 39/41/84/86/118).
func TestHandleCommand_U14_DryRunSkipsStateWrite(t *testing.T) {
	h := newMasHarness(t)
	h.flags.DryRun = true
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "497799835"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(h.provider.statePath(h.flags)); err == nil {
		t.Error("dry-run should not create state file")
	}
}
