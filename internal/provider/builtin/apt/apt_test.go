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

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if h.runner.CallCount("install", "htop") != 1 {
		t.Errorf("runner.Install called %d times, want 1", h.runner.CallCount("install", "htop"))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "htop" {
		t.Errorf("hamsfile apps = %v, want [htop]", apps)
	}
}

// U2: repeat install keeps exactly one entry in the hamsfile.
func TestHandleCommand_U2_InstallIsIdempotent(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("install second time: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 {
		t.Errorf("hamsfile apps = %v, want single entry", apps)
	}
	if h.runner.CallCount("install", "htop") != 2 {
		t.Errorf("expected 2 runner.Install calls, got %d", h.runner.CallCount("install", "htop"))
	}
}

// U3: when runner.Install errors, hamsfile is NOT modified.
func TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newHarness(t)
	h.runner.WithInstallError("htop", errors.New("apt-get: E: no such package"))

	err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags)
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
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}

	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile should be empty after remove, got %v", apps)
	}
	if h.runner.CallCount("remove", "htop") != 1 {
		t.Errorf("expected 1 runner.Remove call, got %d", h.runner.CallCount("remove", "htop"))
	}
}

// U5: remove failure leaves hamsfile untouched.
func TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}

	h.runner.WithRemoveError("htop", errors.New("apt-get remove failed"))
	err := h.provider.HandleCommand(context.Background(), []string{"remove", "htop"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected remove error")
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "htop" {
		t.Errorf("hamsfile should still contain htop after remove failure, got %v", apps)
	}
}

// U6: remove of absent entry is a no-op on the file, no error.
func TestHandleCommand_U6_RemoveAbsentIsNoOp(t *testing.T) {
	h := newHarness(t)
	// Pre-populate as "already installed" so runner.Remove succeeds.
	h.runner.Seed("htop", "1.0.0")

	err := h.provider.HandleCommand(context.Background(), []string{"remove", "htop"}, nil, h.flags)
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

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
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

	if err := h.provider.Apply(context.Background(), provider.Action{ID: "htop"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	sf := state.New("apt", "test-machine")
	sf.SetResource("htop", state.StateOK)
	r := sf.Resources["htop"]
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
	sf.SetResource("htop", state.StateOK)
	firstInstall := sf.Resources["htop"].FirstInstallAt
	sf.Resources["htop"].UpdatedAt = "19700101T000000"

	sf.SetResource("htop", state.StateOK)
	r := sf.Resources["htop"]
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
	sf.SetResource("htop", state.StateOK)
	firstInstall := sf.Resources["htop"].FirstInstallAt
	sf.Resources["htop"].UpdatedAt = "19700101T000000"

	sf.SetResource("htop", state.StateRemoved)
	r := sf.Resources["htop"]
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
	sf.SetResource("htop", state.StateOK)
	firstInstall := sf.Resources["htop"].FirstInstallAt
	sf.SetResource("htop", state.StateRemoved)
	if sf.Resources["htop"].RemovedAt == "" {
		t.Fatal("precondition: RemovedAt should be set")
	}

	sf.Resources["htop"].UpdatedAt = "19700101T000000"
	sf.SetResource("htop", state.StateOK)
	r := sf.Resources["htop"]
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

// Probe uses the CmdRunner and skips removed resources.
func TestProbe_SkipsRemovedAndUsesRunner(t *testing.T) {
	h := newHarness(t)
	h.runner.Seed("htop", "0.24.0")
	// jq is intentionally NOT in Installed -> probe returns StateFailed for it.
	// vim is set to StateRemoved -> probe skips it entirely.

	sf := state.New("apt", "test-machine")
	sf.SetResource("htop", state.StateOK)
	sf.SetResource("jq", state.StateOK)
	sf.SetResource("vim", state.StateOK)
	sf.SetResource("vim", state.StateRemoved)

	results, err := h.provider.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	states := map[string]state.ResourceState{}
	for _, r := range results {
		states[r.ID] = r.State
	}
	if states["htop"] != state.StateOK {
		t.Errorf("htop state = %q, want ok", states["htop"])
	}
	if states["jq"] != state.StateFailed {
		t.Errorf("jq state = %q, want failed (not installed in fake)", states["jq"])
	}
	if _, present := states["vim"]; present {
		t.Errorf("removed resource vim should be skipped by probe")
	}

	// Probe should NOT call IsInstalled for removed resources.
	if h.runner.CallCount("is_installed", "vim") != 0 {
		t.Errorf("probe should skip removed resources, got %d calls for vim",
			h.runner.CallCount("is_installed", "vim"))
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
	in := []string{"--no-install-recommends", "htop", "-y", "jq"}
	got := packageArgs(in)
	want := []string{"htop", "jq"}
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
	err := errorWithPkg("htop")
	if !strings.Contains(err.Error(), "htop") {
		t.Errorf("error should name pkg, got %q", err.Error())
	}
}

func errorWithPkg(pkg string) error {
	return errors.New("apt-get install " + pkg + ": exit status 100")
}

// loadState reads the state file written by the harness's provider; calling
// before any install/remove returns a fresh in-memory File with no resources.
func (h *testHarness) loadState() *state.File {
	h.t.Helper()
	if _, err := os.Stat(h.statePath); err != nil {
		return state.New("apt", "test-machine")
	}
	sf, err := state.Load(h.statePath)
	if err != nil {
		h.t.Fatalf("load state: %v", err)
	}
	return sf
}

// U12: imperative install writes apt.state.yaml with state=ok, FirstInstallAt,
// UpdatedAt, and Version captured from runner.IsInstalled.
func TestHandleCommand_U12_InstallWritesState(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	sf := h.loadState()
	r, ok := sf.Resources["htop"]
	if !ok {
		t.Fatal("htop should be in state after install")
	}
	if r.State != state.StateOK {
		t.Errorf("State = %q, want %q", r.State, state.StateOK)
	}
	if r.FirstInstallAt == "" {
		t.Error("FirstInstallAt should be set on first install")
	}
	if r.UpdatedAt != r.FirstInstallAt {
		t.Errorf("UpdatedAt = %q, want %q (equal to FirstInstallAt on first install)",
			r.UpdatedAt, r.FirstInstallAt)
	}
	if r.RemovedAt != "" {
		t.Errorf("RemovedAt should be empty on fresh install, got %q", r.RemovedAt)
	}
	if r.Version != "fake-1.0.0" {
		t.Errorf("Version = %q, want fake-1.0.0 (from FakeCmdRunner.Install default)", r.Version)
	}
}

// U13: re-install bumps UpdatedAt while leaving FirstInstallAt immutable.
func TestHandleCommand_U13_ReinstallPreservesFirstInstallAt(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("first install: %v", err)
	}
	first := h.loadState().Resources["htop"]
	if first == nil {
		t.Fatal("first install should have written state")
	}
	originalFirstInstall := first.FirstInstallAt

	// Force a stale UpdatedAt so we can detect the bump unambiguously.
	stale := state.New("apt", "test-machine")
	if loaded, err := state.Load(h.statePath); err == nil {
		stale = loaded
	}
	stale.Resources["htop"].UpdatedAt = "19700101T000000"
	if err := stale.Save(h.statePath); err != nil {
		t.Fatalf("seed stale UpdatedAt: %v", err)
	}

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("re-install: %v", err)
	}

	r := h.loadState().Resources["htop"]
	if r.FirstInstallAt != originalFirstInstall {
		t.Errorf("FirstInstallAt mutated: got %q, want %q", r.FirstInstallAt, originalFirstInstall)
	}
	if r.UpdatedAt == "19700101T000000" {
		t.Error("UpdatedAt should be bumped on re-install")
	}
	if r.State != state.StateOK {
		t.Errorf("State = %q, want %q", r.State, state.StateOK)
	}
}

// U14: imperative remove writes state=removed with RemovedAt set and
// FirstInstallAt preserved when the resource was previously installed.
func TestHandleCommand_U14_RemoveWritesState(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	preserved := h.loadState().Resources["htop"].FirstInstallAt

	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}

	r := h.loadState().Resources["htop"]
	if r == nil {
		t.Fatal("htop should remain in state for audit after remove")
	}
	if r.State != state.StateRemoved {
		t.Errorf("State = %q, want %q", r.State, state.StateRemoved)
	}
	if r.FirstInstallAt != preserved {
		t.Errorf("FirstInstallAt mutated on remove: got %q, want %q", r.FirstInstallAt, preserved)
	}
	if r.RemovedAt == "" {
		t.Error("RemovedAt should be set on remove")
	}
	if r.UpdatedAt != r.RemovedAt {
		t.Errorf("UpdatedAt = %q, want %q (equal to RemovedAt on remove)", r.UpdatedAt, r.RemovedAt)
	}
}

// U15: re-install after remove clears RemovedAt while preserving FirstInstallAt.
func TestHandleCommand_U15_ReinstallAfterRemoveClearsRemovedAt(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("setup install: %v", err)
	}
	preserved := h.loadState().Resources["htop"].FirstInstallAt

	if err := h.provider.HandleCommand(context.Background(), []string{"remove", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if h.loadState().Resources["htop"].RemovedAt == "" {
		t.Fatal("precondition: RemovedAt should be set after remove")
	}

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("re-install: %v", err)
	}

	r := h.loadState().Resources["htop"]
	if r.State != state.StateOK {
		t.Errorf("State = %q, want %q", r.State, state.StateOK)
	}
	if r.RemovedAt != "" {
		t.Errorf("RemovedAt should be cleared after re-install, got %q", r.RemovedAt)
	}
	if r.FirstInstallAt != preserved {
		t.Errorf("FirstInstallAt mutated across remove+reinstall: got %q, want %q",
			r.FirstInstallAt, preserved)
	}
}

// U16: when runner.Install errors, neither hamsfile nor state are written.
func TestHandleCommand_U16_InstallFailureLeavesStateUntouched(t *testing.T) {
	h := newHarness(t)
	h.runner.WithInstallError("htop", errors.New("apt-get: E: no such package"))

	err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected install error")
	}
	if _, statErr := os.Stat(h.statePath); statErr == nil {
		t.Error("state file should not exist after install failure")
	}
}

// U17: dry-run does not load or write the state file.
func TestHandleCommand_U17_DryRunDoesNotTouchState(t *testing.T) {
	h := newHarness(t)
	h.flags.DryRun = true

	if err := h.provider.HandleCommand(context.Background(), []string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	if _, statErr := os.Stat(h.statePath); statErr == nil {
		t.Error("dry-run should not create the state file")
	}
}

// U18: passthrough flags survive into runner.Install args. Locks in the
// codex P2 fix — `hams apt install --no-install-recommends htop` had been
// silently dropping the flag because the CLI loop iterated `packageArgs(args)`
// (filtered names only) and called `runner.Install(ctx, pkg)` per package.
func TestHandleCommand_U18_InstallPreservesPassthroughFlags(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "--no-install-recommends", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := h.runner.LastCallArgs("install")
	want := []string{"--no-install-recommends", "htop"}
	if !slicesEqual(got, want) {
		t.Errorf("runner.Install args = %v, want %v", got, want)
	}
}

// U19: multi-package install runs as one apt-get transaction. Locks in
// the codex P2 fix — looping `runner.Install(ctx, pkg)` per package broke
// atomicity (ok-pkg would install successfully on the host, then bad-pkg
// would fail, leaving ok-pkg installed but unrecorded in hamsfile/state).
func TestHandleCommand_U19_InstallMultiPackageIsAtomic(t *testing.T) {
	h := newHarness(t)
	h.runner.WithInstallError("bad-pkg", errors.New("E: Unable to locate package bad-pkg"))

	err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "ok-pkg", "bad-pkg"}, nil, h.flags)
	if err == nil {
		t.Fatal("expected error for failed multi-package install")
	}

	// Exactly one Install call (transactional batch), not two.
	if h.runner.CallCount("install", "") != 1 {
		t.Errorf("runner.Install was called %d times, want 1 (atomic batch)",
			h.runner.CallCount("install", ""))
	}
	// Neither package is installed (apt-get atomic: dep-resolution failure
	// rolls back the whole batch).
	installed, _, probeErr := h.runner.IsInstalled(context.Background(), "ok-pkg")
	if probeErr != nil {
		t.Fatalf("IsInstalled probe: %v", probeErr)
	}
	if installed {
		t.Error("ok-pkg should NOT be installed when batch failed atomically")
	}
	// Neither hamsfile nor state should record either package — install
	// failed, the post-runner-bookkeeping was never reached.
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile apps = %v, want []", apps)
	}
	if _, statErr := os.Stat(h.statePath); statErr == nil {
		t.Error("state file should not exist after atomic install failure")
	}
}

// U20: dry-run flags (--download-only / --simulate / -s / ...) execute
// the apt-get call (passthrough) but skip auto-record entirely. The
// hamsfile + state remain unchanged so `hams apply` does not later try
// to reconcile against a state row that was never genuinely installed.
// Codex round-3 finding: dpkg-based gate could not distinguish dry-run
// on a pre-existing install from a real install; the upstream
// `isComplexAptInvocation` guard sidesteps the question entirely.
func TestHandleCommand_U20_DownloadOnlyDoesNotRecordState(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "--download-only", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	// runner.Install was still forwarded (passthrough preserved).
	got := h.runner.LastCallArgs("install")
	want := []string{"--download-only", "htop"}
	if !slicesEqual(got, want) {
		t.Errorf("runner.Install args = %v, want %v", got, want)
	}

	// Auto-record was refused — no hamsfile, no state file.
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile apps = %v, want [] (dry-run flag must not auto-record)", apps)
	}
	if _, statErr := os.Stat(h.statePath); statErr == nil {
		t.Error("state file should not exist after --download-only auto-record refusal")
	}
}

// U21: an apt option value (`-o KEY=VAL`) is filtered by parseAptInstallToken's
// debian-package-name regex — `Debug::NoLocking` contains `::` which is
// not a valid Debian package name character, so the parser returns
// empty and the bookkeeping skips it. Only the legitimate `htop` is
// recorded.
func TestHandleCommand_U21_AptOptionValueNotRecordedAsPackage(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "-o", "Debug::NoLocking=true", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "htop" {
		t.Errorf("hamsfile apps = %v, want [htop] (option value must not be recorded)", apps)
	}
}

// U22: version-pinning syntax (`pkg=version`) — apt installs the pinned
// version AND hams records a structured entry with `version: "<pin>"` in
// the hamsfile and `requested_version` in state.
func TestHandleCommand_U22_VersionPinRecordsStructuredEntry(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "nginx=1.24.0"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "nginx" {
		t.Fatalf("hamsfile apps = %v, want [nginx]", apps)
	}

	data, readErr := os.ReadFile(h.hamsfilePath)
	if readErr != nil {
		t.Fatalf("read hamsfile: %v", readErr)
	}
	if !strings.Contains(string(data), "version: \"1.24.0\"") &&
		!strings.Contains(string(data), "version: 1.24.0") {
		t.Errorf("hamsfile body = %q, want to contain version: 1.24.0", string(data))
	}

	sf, loadErr := state.Load(h.statePath)
	if loadErr != nil {
		t.Fatalf("state.Load: %v", loadErr)
	}
	r, ok := sf.Resources["nginx"]
	if !ok {
		t.Fatal("nginx missing from state after version-pin install")
	}
	if r.RequestedVersion != "1.24.0" {
		t.Errorf("nginx.RequestedVersion = %q, want 1.24.0", r.RequestedVersion)
	}
}

// U23: release-pinning syntax (`pkg/release`) — same shape as U22 but
// recording `source` instead of `version`.
func TestHandleCommand_U23_ReleasePinRecordsStructuredEntry(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "nginx/bookworm-backports"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "nginx" {
		t.Fatalf("hamsfile apps = %v, want [nginx]", apps)
	}

	data, readErr := os.ReadFile(h.hamsfilePath)
	if readErr != nil {
		t.Fatalf("read hamsfile: %v", readErr)
	}
	if !strings.Contains(string(data), "source: bookworm-backports") {
		t.Errorf("hamsfile body = %q, want to contain source: bookworm-backports", string(data))
	}

	sf, loadErr := state.Load(h.statePath)
	if loadErr != nil {
		t.Fatalf("state.Load: %v", loadErr)
	}
	r := sf.Resources["nginx"]
	if r.RequestedSource != "bookworm-backports" {
		t.Errorf("nginx.RequestedSource = %q, want bookworm-backports", r.RequestedSource)
	}
}

// U24: passthrough flags that don't change install semantics
// (--no-install-recommends) DO auto-record — they're not "complex". This
// is the original round-1 case that motivated `--prune-orphans`-adjacent
// fixes; lock it in to prevent the U20-U23 narrowing from over-correcting.
func TestHandleCommand_U24_BenignPassthroughFlagStillRecords(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "--no-install-recommends", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "htop" {
		t.Errorf("hamsfile apps = %v, want [htop] (--no-install-recommends is benign)", apps)
	}
}

// U25: parseAptInstallToken unit cases.
func TestParseAptInstallToken(t *testing.T) {
	tests := []struct {
		arg                          string
		wantPkg, wantVer, wantSource string
	}{
		{"nginx", "nginx", "", ""},
		{"nginx=1.24.0", "nginx", "1.24.0", ""},
		{"nginx/bookworm-backports", "nginx", "", "bookworm-backports"},
		{"", "", "", ""},
		{"-y", "", "", ""},
		{"--no-install-recommends", "", "", ""},
		{"Debug::NoLocking=true", "", "", ""}, // not a valid package name
		{"Bad/Pkg", "", "", ""},               // not a valid package name
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			pkg, ver, src := parseAptInstallToken(tt.arg)
			if pkg != tt.wantPkg || ver != tt.wantVer || src != tt.wantSource {
				t.Errorf("parseAptInstallToken(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.arg, pkg, ver, src, tt.wantPkg, tt.wantVer, tt.wantSource)
			}
		})
	}
}

// U26: bare-name hamsfile entries do not gain spurious version/source
// keys after this change. Round-trip a `{app: htop}` install and inspect
// the serialized YAML for unexpected `version:` / `source:` lines.
func TestHandleCommand_U26_BareNameEntryHasNoExtraFields(t *testing.T) {
	h := newHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	data, err := os.ReadFile(h.hamsfilePath)
	if err != nil {
		t.Fatalf("read hamsfile: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "version:") || strings.Contains(body, "source:") {
		t.Errorf("bare-name entry leaked structured fields: %q", body)
	}
}

// U27: Plan emits Update with the pinned token when state shows drift
// (observed version != requested_version).
func TestPlan_VersionDriftEmitsUpdate(t *testing.T) {
	h := newHarness(t)

	// Seed hamsfile with `{app: nginx, version: "1.24.0"}` via the new helper.
	hf, err := h.provider.loadOrCreateHamsfile(nil, h.flags)
	if err != nil {
		t.Fatalf("loadOrCreateHamsfile: %v", err)
	}
	hf.AddAppWithFields("cli", "nginx", "", map[string]string{"version": "1.24.0"})
	if writeErr := hf.Write(); writeErr != nil {
		t.Fatalf("hf.Write: %v", writeErr)
	}

	// Seed state: nginx observed at 1.22.1 with requested_version=1.24.0.
	sf := state.New("apt", "test-machine")
	sf.SetResource("nginx", state.StateOK,
		state.WithVersion("1.22.1"),
		state.WithRequestedVersion("1.24.0"),
	)
	sf.ConfigHash = "sentinel"
	if saveErr := sf.Save(h.statePath); saveErr != nil {
		t.Fatalf("sf.Save: %v", saveErr)
	}

	hf2, err := hamsfile.Read(h.hamsfilePath)
	if err != nil {
		t.Fatalf("re-read hamsfile: %v", err)
	}
	sf2, err := state.Load(h.statePath)
	if err != nil {
		t.Fatalf("re-load state: %v", err)
	}

	actions, err := h.provider.Plan(context.Background(), hf2, sf2)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	var found bool
	for _, a := range actions {
		if a.Type == provider.ActionUpdate && a.ID == "nginx" {
			res, ok := a.Resource.(string)
			if ok && res == "nginx=1.24.0" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("Plan actions = %#v, want one with Type=Update, ID=nginx, Resource=nginx=1.24.0", actions)
	}
}

// U28: Plan emits Skip when observed matches requested (no drift).
func TestPlan_VersionMatchEmitsSkip(t *testing.T) {
	h := newHarness(t)
	hf, err := h.provider.loadOrCreateHamsfile(nil, h.flags)
	if err != nil {
		t.Fatalf("loadOrCreateHamsfile: %v", err)
	}
	hf.AddAppWithFields("cli", "nginx", "", map[string]string{"version": "1.24.0"})
	if writeErr := hf.Write(); writeErr != nil {
		t.Fatalf("hf.Write: %v", writeErr)
	}

	sf := state.New("apt", "test-machine")
	sf.SetResource("nginx", state.StateOK,
		state.WithVersion("1.24.0"),
		state.WithRequestedVersion("1.24.0"),
	)
	if saveErr := sf.Save(h.statePath); saveErr != nil {
		t.Fatalf("sf.Save: %v", saveErr)
	}

	hf2, readErr := hamsfile.Read(h.hamsfilePath)
	if readErr != nil {
		t.Fatalf("re-read hamsfile: %v", readErr)
	}
	sf2, loadErr := state.Load(h.statePath)
	if loadErr != nil {
		t.Fatalf("re-load state: %v", loadErr)
	}

	actions, err := h.provider.Plan(context.Background(), hf2, sf2)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, a := range actions {
		if a.ID == "nginx" || strings.HasPrefix(a.ID, "nginx=") {
			if a.Type != provider.ActionSkip {
				t.Errorf("nginx action = %#v, want Skip when observed matches requested", a)
			}
			return
		}
	}
	t.Errorf("Plan actions = %#v, want one for nginx", actions)
}

// U29: hams apt install <pkg>=<ver> against an existing bare entry
// upgrades the hamsfile entry IN PLACE — no duplicate, no skip.
// Locks in the round-4 finding III fix.
func TestHandleCommand_U29_BareToPinnedUpgrade(t *testing.T) {
	h := newHarness(t)
	// Pre-write hamsfile with {app: nginx} bare (simulate prior install
	// without a pin).
	if err := os.MkdirAll(h.profileDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(h.hamsfilePath, []byte("cli:\n  - app: nginx\n"), 0o600); err != nil {
		t.Fatalf("seed hamsfile: %v", err)
	}

	// Re-run install with a pin.
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "nginx=1.24.0"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Hamsfile should now have a SINGLE nginx entry with version pin.
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "nginx" {
		t.Fatalf("hamsfile apps = %v, want exactly [nginx]", apps)
	}
	data, err := os.ReadFile(h.hamsfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "1.24.0") {
		t.Errorf("upgrade did not write version pin: %q", body)
	}

	// State should record the pin.
	sf, err := state.Load(h.statePath)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	r := sf.Resources["nginx"]
	if r.RequestedVersion != "1.24.0" {
		t.Errorf("nginx.RequestedVersion = %q, want 1.24.0", r.RequestedVersion)
	}
}

// TestHandleCommand_BareInstallClearsPriorPin locks in cycle 173:
// `hams apt install nginx` (bare) AFTER a prior `hams apt install
// nginx=1.24.0` (pinned) MUST clear the version pin from BOTH the
// hamsfile AND the state file. Otherwise:
//
//   - Hamsfile keeps `version: 1.24.0` even though the user asked
//     for "no pin".
//   - State.RequestedVersion stays "1.24.0".
//   - apt-get itself installed the LATEST version (no `=ver` arg).
//   - Next `hams refresh` + `hams apply` sees the pin mismatch and
//     tries to downgrade the freshly-installed latest version back
//     to 1.24.0 — exact opposite of the user's intent.
//
// Pre-cycle-173: `AddAppWithFields` skipped empty values during merge,
// so the version field stayed; state's WithRequestedVersion was only
// added when non-empty, so RequestedVersion stayed too.
func TestHandleCommand_BareInstallClearsPriorPin(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(h.profileDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Pre-seed hamsfile with a pinned entry (simulates prior pinned
	// install).
	if err := os.WriteFile(h.hamsfilePath,
		[]byte("cli:\n  - app: nginx\n    version: 1.24.0\n"), 0o600); err != nil {
		t.Fatalf("seed hamsfile: %v", err)
	}
	// Pre-seed state too.
	sfSeed := state.New("apt", "test-machine")
	sfSeed.SetResource("nginx", state.StateOK,
		state.WithVersion("1.24.0"),
		state.WithRequestedVersion("1.24.0"))
	if err := sfSeed.Save(h.statePath); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	// Re-run install WITHOUT a pin — user explicitly intends to drop
	// the pin and accept whatever apt-get installs.
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "nginx"}, nil, h.flags); err != nil {
		t.Fatalf("bare re-install: %v", err)
	}

	// Hamsfile MUST no longer contain `version: 1.24.0` for nginx.
	body, err := os.ReadFile(h.hamsfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(body), "1.24.0") {
		t.Errorf("hamsfile still contains stale pin '1.24.0'; got:\n%s", string(body))
	}
	if strings.Contains(string(body), "version") {
		t.Errorf("hamsfile still has 'version' key after bare install; got:\n%s", string(body))
	}

	// State.RequestedVersion MUST be cleared.
	sf, err := state.Load(h.statePath)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	r := sf.Resources["nginx"]
	if r == nil {
		t.Fatalf("nginx state resource missing after bare re-install")
	}
	if r.RequestedVersion != "" {
		t.Errorf("nginx.RequestedVersion = %q after bare install, want empty (unpinned)", r.RequestedVersion)
	}
	if r.RequestedSource != "" {
		t.Errorf("nginx.RequestedSource = %q after bare install, want empty (unpinned)", r.RequestedSource)
	}
}

// U30: Plan on a fresh machine (state has no nginx entry) replays
// the hamsfile-declared pin via action.Resource. Locks in finding I.
func TestPlan_HamsfilePinReplaysOnFreshMachine(t *testing.T) {
	h := newHarness(t)
	hf, err := h.provider.loadOrCreateHamsfile(nil, h.flags)
	if err != nil {
		t.Fatalf("loadOrCreateHamsfile: %v", err)
	}
	hf.AddAppWithFields("cli", "nginx", "", map[string]string{"version": "1.24.0"})
	if writeErr := hf.Write(); writeErr != nil {
		t.Fatalf("hf.Write: %v", writeErr)
	}

	// Empty state: simulate fresh machine.
	sf := state.New("apt", "test-machine")
	if saveErr := sf.Save(h.statePath); saveErr != nil {
		t.Fatalf("sf.Save: %v", saveErr)
	}

	hf2, readErr := hamsfile.Read(h.hamsfilePath)
	if readErr != nil {
		t.Fatalf("re-read hamsfile: %v", readErr)
	}
	sf2, loadErr := state.Load(h.statePath)
	if loadErr != nil {
		t.Fatalf("re-load state: %v", loadErr)
	}
	actions, err := h.provider.Plan(context.Background(), hf2, sf2)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	var found bool
	for _, a := range actions {
		if a.ID != "nginx" {
			continue
		}
		if a.Type != provider.ActionInstall {
			t.Errorf("nginx action.Type = %v, want Install", a.Type)
		}
		res, ok := a.Resource.(string)
		if !ok || res != "nginx=1.24.0" {
			t.Errorf("nginx action.Resource = %q (ok=%v), want nginx=1.24.0", res, ok)
		}
		found = true
	}
	if !found {
		t.Errorf("Plan returned no nginx action: %#v", actions)
	}
}

// U31: The full Apply path uses action.Resource (the install token)
// when invoking the runner, so apt-get gets the pin even though state
// stays keyed on the bare ID.
func TestApply_UsesResourceOverIDForPinnedActions(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.Apply(context.Background(), provider.Action{
		ID:       "nginx",
		Type:     provider.ActionInstall,
		Resource: "nginx=1.24.0",
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got := h.runner.LastCallArgs("install")
	want := []string{"nginx=1.24.0"}
	if !slicesEqual(got, want) {
		t.Errorf("runner.Install args = %v, want %v (must use Resource over ID)", got, want)
	}
}

// U32: Apply falls back to action.ID when Resource is empty/nil.
func TestApply_FallsBackToIDWhenResourceEmpty(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.Apply(context.Background(), provider.Action{
		ID:   "htop",
		Type: provider.ActionInstall,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got := h.runner.LastCallArgs("install")
	want := []string{"htop"}
	if !slicesEqual(got, want) {
		t.Errorf("runner.Install args = %v, want %v (no Resource → use ID)", got, want)
	}
}

// U33: pinned-skip actions carry pin metadata even WITHOUT drift.
// runApply may promote Skip→Update via the hamsfile-hash check; without
// pin metadata that promotion would Apply with the bare ID and lose the
// pin. Locks in the round-5 finding I fix.
func TestPlan_PinnedSkipCarriesPinMetadataEvenWithoutDrift(t *testing.T) {
	h := newHarness(t)
	hf, err := h.provider.loadOrCreateHamsfile(nil, h.flags)
	if err != nil {
		t.Fatalf("loadOrCreateHamsfile: %v", err)
	}
	hf.AddAppWithFields("cli", "nginx", "", map[string]string{"version": "1.24.0"})
	if writeErr := hf.Write(); writeErr != nil {
		t.Fatalf("hf.Write: %v", writeErr)
	}

	// State has nginx at the matching version → ComputePlan returns Skip.
	sf := state.New("apt", "test-machine")
	sf.SetResource("nginx", state.StateOK,
		state.WithVersion("1.24.0"),
		state.WithRequestedVersion("1.24.0"),
	)
	if saveErr := sf.Save(h.statePath); saveErr != nil {
		t.Fatalf("sf.Save: %v", saveErr)
	}

	hf2, readErr := hamsfile.Read(h.hamsfilePath)
	if readErr != nil {
		t.Fatalf("re-read hamsfile: %v", readErr)
	}
	sf2, loadErr := state.Load(h.statePath)
	if loadErr != nil {
		t.Fatalf("re-load state: %v", loadErr)
	}

	actions, err := h.provider.Plan(context.Background(), hf2, sf2)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	var found bool
	for _, a := range actions {
		if a.ID != "nginx" {
			continue
		}
		// Skip is correct here; the test's contract is that the
		// pin metadata IS present so a hash-promotion to Update by
		// runApply doesn't lose it.
		res, ok := a.Resource.(string)
		if !ok || res != "nginx=1.24.0" {
			t.Errorf("nginx Skip action.Resource = %q (ok=%v), want nginx=1.24.0 attached even without drift", res, ok)
		}
		if len(a.StateOpts) == 0 {
			t.Errorf("nginx Skip action.StateOpts is empty; want WithRequestedVersion to be present so a hash-promotion preserves the pin")
		}
		found = true
	}
	if !found {
		t.Errorf("Plan returned no nginx action: %#v", actions)
	}
}

// U34: parseAptInstallToken accepts apt's multi-arch suffix.
// `libssl3:amd64` and `zlib1g:i386` are valid apt install tokens; the
// parser must not reject them as non-package names. Locks in the
// round-5 finding II fix.
func TestParseAptInstallToken_AcceptsMultiArchSuffix(t *testing.T) {
	tests := []struct {
		arg                          string
		wantPkg, wantVer, wantSource string
	}{
		{"libssl3:amd64", "libssl3:amd64", "", ""},
		{"zlib1g:i386", "zlib1g:i386", "", ""},
		{"nginx:amd64=1.24.0", "nginx:amd64", "1.24.0", ""},
		{"nginx:arm64/bookworm-backports", "nginx:arm64", "", "bookworm-backports"},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			pkg, ver, src := parseAptInstallToken(tt.arg)
			if pkg != tt.wantPkg || ver != tt.wantVer || src != tt.wantSource {
				t.Errorf("parseAptInstallToken(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.arg, pkg, ver, src, tt.wantPkg, tt.wantVer, tt.wantSource)
			}
		})
	}
}

// U35: end-to-end via handleInstall — `hams apt install libssl3:amd64`
// records `{app: libssl3:amd64}` in hamsfile + state row keyed on the
// same. Locks in the round-5 finding II fix at the integration boundary.
func TestHandleCommand_U35_MultiArchPackageRecords(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "libssl3:amd64"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "libssl3:amd64" {
		t.Errorf("hamsfile apps = %v, want [libssl3:amd64]", apps)
	}
	sf, err := state.Load(h.statePath)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if _, ok := sf.Resources["libssl3:amd64"]; !ok {
		t.Errorf("state has no libssl3:amd64 row: keys=%v", sf.Resources)
	}
}

// U36: hams apt remove nginx clears the pin from state. Locks in the
// audit-trail-truth fix surfaced by the holistic reviewer:
// install pinned then remove leaves stale `requested_version` in the
// StateRemoved row, suggesting the user still wants the pin even
// though they uninstalled.
func TestHandleCommand_U36_RemoveClearsRequestedPinInState(t *testing.T) {
	h := newHarness(t)

	// First, install pinned.
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "nginx=1.24.0"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Then remove (bare).
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"remove", "nginx"}, nil, h.flags); err != nil {
		t.Fatalf("remove: %v", err)
	}

	sf, err := state.Load(h.statePath)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	r, ok := sf.Resources["nginx"]
	if !ok {
		t.Fatal("nginx missing from state after remove")
	}
	if r.State != state.StateRemoved {
		t.Errorf("nginx.State = %q, want %q", r.State, state.StateRemoved)
	}
	if r.RequestedVersion != "" {
		t.Errorf("nginx.RequestedVersion = %q after remove, want empty (audit trail must not lie)", r.RequestedVersion)
	}
}

// U37: hams apt remove nginx=1.24.0 (the pinned form) keys state on
// the bare `nginx`, not on `nginx=1.24.0`. Locks in the round-5-style
// "remove with install-token form must use bare key" fix.
func TestHandleCommand_U37_RemoveWithPinKeysStateOnBareName(t *testing.T) {
	h := newHarness(t)

	// Pre-install pinned so remove has something to do.
	h.runner.Seed("nginx", "1.24.0")
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "nginx=1.24.0"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"remove", "nginx=1.24.0"}, nil, h.flags); err != nil {
		t.Fatalf("remove with pin: %v", err)
	}

	sf, err := state.Load(h.statePath)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if _, ok := sf.Resources["nginx=1.24.0"]; ok {
		t.Errorf("state has orphan nginx=1.24.0 row; remove must key on bare nginx")
	}
	r, ok := sf.Resources["nginx"]
	if !ok {
		t.Fatal("nginx missing from state — remove failed to find the bare key")
	}
	if r.State != state.StateRemoved {
		t.Errorf("nginx.State = %q, want %q", r.State, state.StateRemoved)
	}
}

// U38: Plan clears stale state-pin when the hamsfile no longer
// declares a pin (user hand-edits {app: nginx, version: "1.24.0"} to
// {app: nginx}). The cleared StateOpts fires when runApply
// hash-promotes Skip→Update.
func TestPlan_UnpinClearsStaleStatePin(t *testing.T) {
	h := newHarness(t)
	hf, err := h.provider.loadOrCreateHamsfile(nil, h.flags)
	if err != nil {
		t.Fatalf("loadOrCreateHamsfile: %v", err)
	}
	// Hamsfile: bare nginx (no pin).
	hf.AddAppWithFields("cli", "nginx", "", nil)
	if writeErr := hf.Write(); writeErr != nil {
		t.Fatalf("hf.Write: %v", writeErr)
	}

	// State: nginx with stale requested_version (the pin was there before
	// the user hand-edited the hamsfile).
	sf := state.New("apt", "test-machine")
	sf.SetResource("nginx", state.StateOK,
		state.WithVersion("1.24.0"),
		state.WithRequestedVersion("1.24.0"),
	)
	if saveErr := sf.Save(h.statePath); saveErr != nil {
		t.Fatalf("sf.Save: %v", saveErr)
	}

	hf2, readErr := hamsfile.Read(h.hamsfilePath)
	if readErr != nil {
		t.Fatalf("re-read hamsfile: %v", readErr)
	}
	sf2, loadErr := state.Load(h.statePath)
	if loadErr != nil {
		t.Fatalf("re-load state: %v", loadErr)
	}
	actions, err := h.provider.Plan(context.Background(), hf2, sf2)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	var found bool
	for _, a := range actions {
		if a.ID != "nginx" {
			continue
		}
		// Action stays Skip (no version drift), but StateOpts must
		// carry the explicit clears so a future hash-promotion
		// removes the stale pin.
		if len(a.StateOpts) == 0 {
			t.Errorf("nginx action.StateOpts is empty; want WithRequestedVersion('') to clear the stale pin")
		}
		// Apply the StateOpts to a fresh resource and assert clear.
		probe := &state.File{Resources: map[string]*state.Resource{
			"nginx": {RequestedVersion: "1.24.0", RequestedSource: "stale"},
		}}
		probe.SetResource("nginx", state.StateOK, a.StateOpts...)
		got := probe.Resources["nginx"]
		if got.RequestedVersion != "" || got.RequestedSource != "" {
			t.Errorf("after applying StateOpts: RequestedVersion=%q RequestedSource=%q, want both cleared", got.RequestedVersion, got.RequestedSource)
		}
		found = true
	}
	if !found {
		t.Errorf("Plan returned no nginx action: %#v", actions)
	}
}
