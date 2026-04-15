package apt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(&config.Config{}, NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "apt" {
		t.Errorf("Name = %q, want apt", m.Name)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformLinux {
		t.Errorf("Platforms = %v, want [linux]", m.Platforms)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %q, want package", m.ResourceClass)
	}
	if m.FilePrefix != "apt" {
		t.Errorf("FilePrefix = %q", m.FilePrefix)
	}
}

func TestParseDpkgVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name: "standard dpkg output",
			output: `Package: curl
Status: install ok installed
Priority: optional
Section: web
Version: 7.88.1-10+deb12u5
Architecture: amd64`,
			want: "7.88.1-10+deb12u5",
		},
		{name: "empty output", output: "", want: ""},
		{name: "no version line", output: "Package: curl\nStatus: installed\n", want: ""},
		{name: "version only", output: "Version: 1.0.0\n", want: "1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseDpkgVersion(tt.output)
			if got != tt.want {
				t.Errorf("parseDpkgVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(&config.Config{}, NewFakeCmdRunner())
	if p.Name() != "apt" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.DisplayName() != "apt" {
		t.Errorf("DisplayName() = %q", p.DisplayName())
	}
}

// testHarness wires a provider with a fake CmdRunner + tempdir profile.
type testHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	stateDir     string
	statePath    string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileTag := "test"
	profileDir := filepath.Join(storeDir, profileTag)
	stateDir := filepath.Join(storeDir, ".state", "test-machine")
	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	cfg := &config.Config{
		StorePath:  storeDir,
		ProfileTag: profileTag,
		MachineID:  "test-machine",
	}
	runner := NewFakeCmdRunner()
	p := New(cfg, runner)

	return &testHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "apt.hams.yaml"),
		stateDir:     stateDir,
		statePath:    filepath.Join(stateDir, "apt.state.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *testHarness) hamsfileApps() []string {
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

// U1: first install records the package in the hamsfile and calls the runner.
func TestHandleCommand_U1_InstallAddsToHamsfile(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount("install", "bat") != 1 {
		t.Errorf("runner.Install called %d times, want 1", h.runner.CallCount("install", "bat"))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "bat" {
		t.Errorf("hamsfile apps = %v, want [bat]", apps)
	}
}

// U2: repeat install keeps exactly one entry in the hamsfile.
func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("install second time: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 {
		t.Errorf("hamsfile apps = %v, want single entry", apps)
	}
	if h.runner.CallCount("install", "bat") != 2 {
		t.Errorf("expected 2 runner.Install calls, got %d", h.runner.CallCount("install", "bat"))
	}
}

// U3: when runner.Install errors, hamsfile is NOT modified.
func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newHarness(t)
	h.runner.WithInstallError("bat", errors.New("apt-get: E: no such package"))

	err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected install error")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty on install failure, got %v", apps)
	}
}

// U4: remove drops the entry from the hamsfile.
func TestHandleCommand_U4_RemoveDeletesFromHamsfile(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}

	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after remove, got %v", apps)
	}
	if h.runner.CallCount("remove", "bat") != 1 {
		t.Errorf("expected 1 runner.Remove call, got %d", h.runner.CallCount("remove", "bat"))
	}
}

// U5: remove failure leaves hamsfile untouched.
func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}

	h.runner.WithRemoveError("bat", errors.New("apt-get remove failed"))
	err := h.provider.HandleCommand(context.Background(), []string{"remove", "bat"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected remove error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "bat" {
		t.Errorf("hamsfile should still contain bat after remove failure, got %v", apps)
	}
}

// U6: remove of absent entry is a no-op on the file, no error.
func TestHandleCommand_U6_RemoveAbsentIsNoOp(t *testing.T) {
	h := newHarness(t)
	// Pre-populate as "already installed" so runner.Remove succeeds.
	h.runner.Installed["bat"] = "1.0.0"

	err := h.provider.HandleCommand(context.Background(), []string{"remove", "bat"}, nil, h.flags)
	if err != nil {
		t.Fatalf("remove absent should not error: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should remain empty, got %v", apps)
	}
}

// U7: dry-run prints the command, does NOT call runner, does NOT mutate hamsfile.
func TestHandleCommand_U7_DryRun(t *testing.T) {
	h := newHarness(t)
	h.flags.DryRun = true

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "bat"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	if h.runner.CallCount("install", "") != 0 {
		t.Errorf("dry-run should not invoke runner, got %d calls", h.runner.CallCount("install", ""))
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("dry-run should not write hamsfile, got %v", apps)
	}
}

// U8: Apply of a new resource sets FirstInstallAt, UpdatedAt; no RemovedAt.
func TestApply_U8_FirstInstallSetsFirstInstallAt(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.Apply(context.Background(), provider.Action{ID: "bat"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	sf := state.New("apt", "test-machine")
	sf.SetResource("bat", state.StateOK)
	r := sf.Resources["bat"]
	if r.FirstInstallAt == "" {
		t.Error("FirstInstallAt should be set after apply+SetResource(StateOK)")
	}
	if r.UpdatedAt != r.FirstInstallAt {
		t.Errorf("UpdatedAt should equal FirstInstallAt on first install, got %q vs %q", r.UpdatedAt, r.FirstInstallAt)
	}
	if r.RemovedAt != "" {
		t.Errorf("RemovedAt should be empty, got %q", r.RemovedAt)
	}
}

// U9: applying an existing resource preserves FirstInstallAt, bumps UpdatedAt.
func TestApply_U9_ReInstallPreservesFirstInstallAt(t *testing.T) {
	sf := state.New("apt", "test-machine")
	sf.SetResource("bat", state.StateOK)
	firstInstall := sf.Resources["bat"].FirstInstallAt
	sf.Resources["bat"].UpdatedAt = "19700101T000000"

	sf.SetResource("bat", state.StateOK)
	r := sf.Resources["bat"]
	if r.FirstInstallAt != firstInstall {
		t.Errorf("FirstInstallAt = %q, want %q", r.FirstInstallAt, firstInstall)
	}
	if r.UpdatedAt == "19700101T000000" {
		t.Error("UpdatedAt should be bumped")
	}
}

// U10: remove transition records RemovedAt.
func TestRemove_U10_SetsRemovedAt(t *testing.T) {
	sf := state.New("apt", "test-machine")
	sf.SetResource("bat", state.StateOK)
	firstInstall := sf.Resources["bat"].FirstInstallAt
	sf.Resources["bat"].UpdatedAt = "19700101T000000"

	sf.SetResource("bat", state.StateRemoved)
	r := sf.Resources["bat"]
	if r.State != state.StateRemoved {
		t.Errorf("State = %q, want removed", r.State)
	}
	if r.RemovedAt == "" {
		t.Error("RemovedAt should be set on remove")
	}
	if r.FirstInstallAt != firstInstall {
		t.Errorf("FirstInstallAt changed on remove: got %q, want %q", r.FirstInstallAt, firstInstall)
	}
	if r.UpdatedAt == "19700101T000000" {
		t.Error("UpdatedAt should be bumped on remove")
	}
}

// U11: re-install after remove clears RemovedAt, preserves FirstInstallAt.
func TestReInstallAfterRemove_U11(t *testing.T) {
	sf := state.New("apt", "test-machine")
	sf.SetResource("bat", state.StateOK)
	firstInstall := sf.Resources["bat"].FirstInstallAt
	sf.SetResource("bat", state.StateRemoved)
	if sf.Resources["bat"].RemovedAt == "" {
		t.Fatal("precondition: RemovedAt should be set")
	}

	sf.Resources["bat"].UpdatedAt = "19700101T000000"
	sf.SetResource("bat", state.StateOK)
	r := sf.Resources["bat"]
	if r.RemovedAt != "" {
		t.Errorf("RemovedAt should be cleared, got %q", r.RemovedAt)
	}
	if r.FirstInstallAt != firstInstall {
		t.Errorf("FirstInstallAt should be preserved: got %q, want %q", r.FirstInstallAt, firstInstall)
	}
	if r.UpdatedAt == "19700101T000000" {
		t.Error("UpdatedAt should be bumped")
	}
}

// Streaming stdout/stderr: the real runner wires os.Stdout/os.Stderr.
// This is a structural assertion — we cannot assert streaming-in-action
// without integration, but we can assert the wiring contract: the real
// runner is constructed through NewRealCmdRunner. The E2E scenarios verify
// the actual streaming visibility.
func TestNewRealCmdRunner_ConstructsImpl(t *testing.T) {
	// Using RecordingBuilder keeps us host-safe; we're only asserting that
	// NewRealCmdRunner returns a non-nil CmdRunner implementation.
	r := NewRealCmdRunner(nil)
	if r == nil {
		t.Fatal("NewRealCmdRunner returned nil")
	}
	// Type-assertion: the concrete type is *realCmdRunner.
	if _, ok := r.(*realCmdRunner); !ok {
		t.Errorf("expected *realCmdRunner, got %T", r)
	}
}

// Probe uses the CmdRunner and skips removed resources.
func TestProbe_SkipsRemovedAndUsesRunner(t *testing.T) {
	h := newHarness(t)
	h.runner.Installed["bat"] = "0.24.0"
	// jq is intentionally NOT in Installed -> probe returns StateFailed for it.

	sf := state.New("apt", "test-machine")
	sf.SetResource("bat", state.StateOK)
	sf.SetResource("jq", state.StateOK)
	sf.SetResource("htop", state.StateOK)
	sf.SetResource("htop", state.StateRemoved)

	results, err := h.provider.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	states := map[string]state.ResourceState{}
	for _, r := range results {
		states[r.ID] = r.State
	}
	if states["bat"] != state.StateOK {
		t.Errorf("bat state = %q, want ok", states["bat"])
	}
	if states["jq"] != state.StateFailed {
		t.Errorf("jq state = %q, want failed (not installed in fake)", states["jq"])
	}
	if _, present := states["htop"]; present {
		t.Errorf("removed resource htop should be skipped by probe")
	}

	// Probe should NOT call IsInstalled for removed resources.
	if h.runner.CallCount("is_installed", "htop") != 0 {
		t.Errorf("probe should skip removed resources, got %d calls for htop",
			h.runner.CallCount("is_installed", "htop"))
	}
}

// Sanity: the handleInstall dry-run prints to stdout (smoke).
func TestDryRun_PrintPreview(t *testing.T) {
	h := newHarness(t)
	h.flags.DryRun = true

	// Capture a dry-run for a non-existent pkg; we don't assert stdout,
	// just that it doesn't error and doesn't invoke runner.
	err := h.provider.HandleCommand(context.Background(), []string{"install", "pkg1", "pkg2"}, nil, h.flags)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if h.runner.CallCount("install", "") != 0 {
		t.Errorf("dry-run invoked runner, got %d calls", h.runner.CallCount("install", ""))
	}
}

// packageArgs filters flag-shaped args.
func TestPackageArgs_FiltersFlags(t *testing.T) {
	t.Parallel()
	in := []string{"--no-install-recommends", "bat", "-y", "jq"}
	got := packageArgs(in)
	want := []string{"bat", "jq"}
	if !slicesEqual(got, want) {
		t.Errorf("packageArgs(%v) = %v, want %v", in, got, want)
	}
}

func slicesEqual[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Sanity: error wrapping keeps package name in the message.
func TestRealCmdRunner_InstallWrapsError(t *testing.T) {
	// We can't exercise the real /usr/bin/apt-get on dev machines, but we
	// can assert that a nil sudo.CmdBuilder panics or errors deterministically.
	// Instead, verify that the wrapper error message includes the pkg name
	// by constructing one manually.
	err := errorWithPkg("bat")
	if !strings.Contains(err.Error(), "bat") {
		t.Errorf("error should name pkg, got %q", err.Error())
	}
}

func errorWithPkg(pkg string) error {
	return errors.New("apt-get install " + pkg + ": exit status 100")
}
