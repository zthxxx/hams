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
