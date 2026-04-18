package provider

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// fakePackageInstaller records Install/Uninstall calls and optionally
// returns a preconfigured error on a named package. Shared across the
// AutoRecordInstall / AutoRecordRemove tests — same contract every
// provider's runner.Install already exposes.
type fakePackageInstaller struct {
	installErrors   map[string]error
	uninstallErrors map[string]error
	installed       []string
	uninstalled     []string
}

func (f *fakePackageInstaller) Install(_ context.Context, pkg string) error {
	if err, ok := f.installErrors[pkg]; ok {
		return err
	}
	f.installed = append(f.installed, pkg)
	return nil
}

func (f *fakePackageInstaller) Uninstall(_ context.Context, pkg string) error {
	if err, ok := f.uninstallErrors[pkg]; ok {
		return err
	}
	f.uninstalled = append(f.uninstalled, pkg)
	return nil
}

// packageDispatcherHarness wires an on-disk hamsfile + state dir and
// returns typed paths. Keeps the per-test setup terse.
type packageDispatcherHarness struct {
	t         *testing.T
	cfg       *config.Config
	flags     *GlobalFlags
	hfPath    string
	statePath string
	opts      PackageDispatchOpts
	installer *fakePackageInstaller
}

func newPackageDispatcherHarness(t *testing.T) *packageDispatcherHarness {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	stateDir := filepath.Join(root, "state")
	if err := os.MkdirAll(filepath.Join(storeDir, "test"), 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	cfg := &config.Config{StorePath: storeDir, ProfileTag: "test", MachineID: "m1"}
	return &packageDispatcherHarness{
		t:         t,
		cfg:       cfg,
		flags:     &GlobalFlags{Store: storeDir},
		hfPath:    filepath.Join(storeDir, "test", "pkg.hams.yaml"),
		statePath: filepath.Join(stateDir, "pkg.state.yaml"),
		opts: PackageDispatchOpts{
			CLIName:     "pkg",
			InstallVerb: "install",
			RemoveVerb:  "remove",
			HamsTag:     "cli",
		},
		installer: &fakePackageInstaller{},
	}
}

func (h *packageDispatcherHarness) hamsfileApps() []string {
	h.t.Helper()
	if _, err := os.Stat(h.hfPath); err != nil {
		return nil
	}
	hf, err := hamsfile.Read(h.hfPath)
	if err != nil {
		h.t.Fatalf("read hamsfile: %v", err)
	}
	return hf.ListApps()
}

func (h *packageDispatcherHarness) stateResource(id string) state.ResourceState {
	h.t.Helper()
	sf, err := state.Load(h.statePath)
	if err != nil {
		return ""
	}
	if r, ok := sf.Resources[id]; ok {
		return r.State
	}
	return ""
}

// TestAutoRecordInstall_HappyPath: multi-package install records every
// entry in hamsfile + state, and invokes the runner once per package.
func TestAutoRecordInstall_HappyPath(t *testing.T) {
	t.Parallel()
	h := newPackageDispatcherHarness(t)
	err := AutoRecordInstall(context.Background(), h.installer,
		[]string{"foo", "bar"}, h.cfg, h.flags, h.hfPath, h.statePath, h.opts)
	if err != nil {
		t.Fatalf("AutoRecordInstall: %v", err)
	}
	if len(h.installer.installed) != 2 {
		t.Errorf("installed count = %d, want 2", len(h.installer.installed))
	}
	apps := h.hamsfileApps()
	if len(apps) != 2 {
		t.Errorf("hamsfile apps = %v, want [foo bar]", apps)
	}
	if got := h.stateResource("foo"); got != state.StateOK {
		t.Errorf("state[foo] = %q, want ok", got)
	}
}

// TestAutoRecordInstall_EmptyUsageError covers the zero-args branch.
func TestAutoRecordInstall_EmptyUsageError(t *testing.T) {
	t.Parallel()
	h := newPackageDispatcherHarness(t)
	err := AutoRecordInstall(context.Background(), h.installer,
		nil, h.cfg, h.flags, h.hfPath, h.statePath, h.opts)
	if err == nil {
		t.Fatal("expected usage error on empty package list")
	}
	if len(h.installer.installed) != 0 {
		t.Errorf("runner should not be invoked, got %d installs", len(h.installer.installed))
	}
}

// TestAutoRecordInstall_DryRunShortCircuits locks in: DryRun prints
// preview and touches nothing (no runner calls, no hamsfile, no state).
func TestAutoRecordInstall_DryRunShortCircuits(t *testing.T) {
	t.Parallel()
	h := newPackageDispatcherHarness(t)
	var buf bytes.Buffer
	h.flags.DryRun = true
	h.flags.Out = &buf
	err := AutoRecordInstall(context.Background(), h.installer,
		[]string{"foo"}, h.cfg, h.flags, h.hfPath, h.statePath, h.opts)
	if err != nil {
		t.Fatalf("AutoRecordInstall: %v", err)
	}
	if len(h.installer.installed) != 0 {
		t.Errorf("runner called under dry-run")
	}
	if _, err := os.Stat(h.hfPath); err == nil {
		t.Error("hamsfile must not exist after dry-run")
	}
	if _, err := os.Stat(h.statePath); err == nil {
		t.Error("state file must not exist after dry-run")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Would install")) {
		t.Errorf("preview missing; got %q", buf.String())
	}
}

// TestAutoRecordInstall_RunnerErrorLeavesHamsfileUntouched — atomic
// auto-record: a runner failure MUST NOT write the hamsfile/state.
func TestAutoRecordInstall_RunnerErrorLeavesHamsfileUntouched(t *testing.T) {
	t.Parallel()
	h := newPackageDispatcherHarness(t)
	h.installer.installErrors = map[string]error{"bad": errors.New("nope")}
	err := AutoRecordInstall(context.Background(), h.installer,
		[]string{"good", "bad"}, h.cfg, h.flags, h.hfPath, h.statePath, h.opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(h.hfPath); err == nil {
		t.Error("hamsfile should not exist after failure")
	}
}

// TestAutoRecordRemove_HappyPath: remove tombstones the state entry
// and drops the hamsfile record.
func TestAutoRecordRemove_HappyPath(t *testing.T) {
	t.Parallel()
	h := newPackageDispatcherHarness(t)
	if err := AutoRecordInstall(context.Background(), h.installer,
		[]string{"foo"}, h.cfg, h.flags, h.hfPath, h.statePath, h.opts); err != nil {
		t.Fatalf("seed install: %v", err)
	}
	if err := AutoRecordRemove(context.Background(), h.installer,
		[]string{"foo"}, h.cfg, h.flags, h.hfPath, h.statePath, h.opts); err != nil {
		t.Fatalf("AutoRecordRemove: %v", err)
	}
	if got := h.stateResource("foo"); got != state.StateRemoved {
		t.Errorf("state[foo] = %q, want removed", got)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile apps = %v, want empty", apps)
	}
}

// TestAutoRecordRemove_EmptyUsageError covers the zero-args branch.
func TestAutoRecordRemove_EmptyUsageError(t *testing.T) {
	t.Parallel()
	h := newPackageDispatcherHarness(t)
	err := AutoRecordRemove(context.Background(), h.installer,
		nil, h.cfg, h.flags, h.hfPath, h.statePath, h.opts)
	if err == nil {
		t.Fatal("expected usage error")
	}
}

// TestAutoRecordRemove_DryRunShortCircuits mirrors the install branch.
func TestAutoRecordRemove_DryRunShortCircuits(t *testing.T) {
	t.Parallel()
	h := newPackageDispatcherHarness(t)
	var buf bytes.Buffer
	h.flags.DryRun = true
	h.flags.Out = &buf
	err := AutoRecordRemove(context.Background(), h.installer,
		[]string{"foo"}, h.cfg, h.flags, h.hfPath, h.statePath, h.opts)
	if err != nil {
		t.Fatalf("AutoRecordRemove: %v", err)
	}
	if len(h.installer.uninstalled) != 0 {
		t.Errorf("runner called under dry-run")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Would remove")) {
		t.Errorf("preview missing; got %q", buf.String())
	}
}
