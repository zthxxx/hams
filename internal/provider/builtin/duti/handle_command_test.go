package duti

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// dutiHarness wires a duti Provider against a FakeCmdRunner +
// tempdir profile so HandleCommand tests can assert real
// hamsfile/state writes without ever exec-ing the host's `duti` binary.
type dutiHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	statePath    string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newDutiHarness(t *testing.T) *dutiHarness {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileTag := "test"
	profileDir := filepath.Join(storeDir, profileTag)
	machineID := "test-machine"
	stateDir := filepath.Join(storeDir, ".state", machineID)
	if err := os.MkdirAll(profileDir, 0o750); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	cfg := &config.Config{StorePath: storeDir, ProfileTag: profileTag, MachineID: machineID}
	runner := NewFakeCmdRunner()
	return &dutiHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "duti.hams.yaml"),
		statePath:    filepath.Join(stateDir, "duti.state.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     New(cfg, runner),
	}
}

func (h *dutiHarness) hamsfileApps() []string {
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

func (h *dutiHarness) stateFile() *state.File {
	h.t.Helper()
	if _, err := os.Stat(h.statePath); err != nil {
		return nil
	}
	f, err := state.Load(h.statePath)
	if err != nil {
		h.t.Fatalf("load state: %v", err)
	}
	return f
}

// U1 — empty args returns a usage error, no runner call.
func TestHandleCommand_U1_NoArgsReturnsUsageError(t *testing.T) {
	h := newDutiHarness(t)

	err := h.provider.HandleCommand(context.Background(), []string{}, nil, h.flags)
	if err == nil {
		t.Fatalf("expected usage error, got nil")
	}
	if h.runner.CallCount(fakeOpSet, "") != 0 {
		t.Errorf("runner.SetDefault called %d times, want 0", h.runner.CallCount(fakeOpSet, ""))
	}
}

// U2 — canonical `<ext>=<bundle-id>` form in dry-run: prints but
// neither calls the runner nor writes to hamsfile/state.
func TestHandleCommand_U2_DryRunSkipsAllSideEffects(t *testing.T) {
	h := newDutiHarness(t)
	h.flags.DryRun = true

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"pdf=com.adobe.acrobat.pdf"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if h.runner.CallCount(fakeOpSet, "") != 0 {
		t.Errorf("runner.SetDefault called %d times in dry-run, want 0", h.runner.CallCount(fakeOpSet, ""))
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("dry-run wrote hamsfile apps = %v", apps)
	}
	if sf := h.stateFile(); sf != nil {
		t.Errorf("dry-run wrote state file")
	}
}

// U3 — `hams duti pdf=com.adobe.acrobat.pdf` auto-records to
// hamsfile + state AND calls runner.SetDefault exactly once.
func TestHandleCommand_U3_SetAutoRecordsToHamsfileAndState(t *testing.T) {
	h := newDutiHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"pdf=com.adobe.acrobat.pdf"}, nil, h.flags); err != nil {
		t.Fatalf("set: %v", err)
	}
	if h.runner.CallCount(fakeOpSet, "") != 1 {
		t.Fatalf("runner.SetDefault called %d times, want 1", h.runner.CallCount(fakeOpSet, ""))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "pdf=com.adobe.acrobat.pdf" {
		t.Errorf("hamsfile apps = %v, want [pdf=com.adobe.acrobat.pdf]", apps)
	}
	sf := h.stateFile()
	if sf == nil {
		t.Fatalf("state file missing")
	}
	r, ok := sf.Resources["pdf=com.adobe.acrobat.pdf"]
	if !ok {
		t.Fatalf("state missing pdf=com.adobe.acrobat.pdf")
	}
	if r.State != state.StateOK || r.Value != "com.adobe.acrobat.pdf" {
		t.Errorf("state = {State:%v Value:%q}, want {ok com.adobe.acrobat.pdf}", r.State, r.Value)
	}
}

// U4 — re-run with the SAME ext but a DIFFERENT bundle-id replaces
// the old hamsfile entry in place. Mirrors git-config U6 / defaults U4.
func TestHandleCommand_U4_ResetNewBundleReplacesEntry(t *testing.T) {
	h := newDutiHarness(t)

	for _, b := range []string{"com.adobe.acrobat.pdf", "com.apple.Preview"} {
		if err := h.provider.HandleCommand(context.Background(),
			[]string{"pdf=" + b}, nil, h.flags); err != nil {
			t.Fatalf("set %s: %v", b, err)
		}
	}

	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "pdf=com.apple.Preview" {
		t.Errorf("hamsfile apps = %v, want [pdf=com.apple.Preview]", apps)
	}

	sf := h.stateFile()
	if sf == nil {
		t.Fatalf("state file missing")
	}
	if oldR, ok := sf.Resources["pdf=com.adobe.acrobat.pdf"]; !ok || oldR.State != state.StateRemoved {
		t.Errorf("old entry state = %v, want StateRemoved (present=%v)", oldR, ok)
	}
	if newR, ok := sf.Resources["pdf=com.apple.Preview"]; !ok || newR.State != state.StateOK {
		t.Errorf("new entry state = %v, want StateOK (present=%v)", newR, ok)
	}
}

// U5 — runner.SetDefault error short-circuits the record path.
func TestHandleCommand_U5_RunnerErrorShortCircuitsRecord(t *testing.T) {
	h := newDutiHarness(t)
	h.runner.WithSetError("pdf", errors.New("boom"))

	err := h.provider.HandleCommand(context.Background(),
		[]string{"pdf=com.adobe.acrobat.pdf"}, nil, h.flags)
	if err == nil {
		t.Fatalf("expected runner error, got nil")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("failed set still wrote hamsfile apps = %v", apps)
	}
	if sf := h.stateFile(); sf != nil {
		t.Errorf("failed set still wrote state file")
	}
}

// U6 — different extensions coexist as independent entries.
func TestHandleCommand_U6_IndependentExtsCoexist(t *testing.T) {
	h := newDutiHarness(t)

	for _, kv := range [][2]string{
		{"pdf", "com.adobe.acrobat.pdf"},
		{"html", "com.google.Chrome"},
	} {
		arg := kv[0] + "=" + kv[1]
		if err := h.provider.HandleCommand(context.Background(),
			[]string{arg}, nil, h.flags); err != nil {
			t.Fatalf("set %s: %v", arg, err)
		}
	}

	got := h.hamsfileApps()
	want := map[string]bool{"pdf=com.adobe.acrobat.pdf": true, "html=com.google.Chrome": true}
	if len(got) != len(want) {
		t.Fatalf("hamsfile apps = %v, want %v", got, want)
	}
	for _, app := range got {
		if !want[app] {
			t.Errorf("unexpected hamsfile app %q", app)
		}
	}
}

// U7 — `--hams-local` routes the entry into the .hams.local.yaml
// variant instead of the shared hamsfile.
func TestHandleCommand_U7_HamsLocalFlagRoutesToLocalFile(t *testing.T) {
	h := newDutiHarness(t)
	localPath := filepath.Join(h.profileDir, "duti.hams.local.yaml")

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"pdf=com.adobe.acrobat.pdf"},
		map[string]string{"local": ""},
		h.flags); err != nil {
		t.Fatalf("set: %v", err)
	}

	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("expected local hamsfile at %q: %v", localPath, err)
	}
	if _, err := os.Stat(h.hamsfilePath); err == nil {
		t.Errorf("unexpected shared hamsfile at %q", h.hamsfilePath)
	}
}

// U8 — multi-arg invocation (e.g., raw duti flags like `-s <bundle>
// .pdf all`) routes through provider.Passthrough via the
// PassthroughExec DI seam and does NOT auto-record. This preserves
// escape-hatch access to the full duti CLI for power users — and
// crucially, the seam lets unit tests assert the call without ever
// rebinding macOS LaunchServices defaults on the host.
func TestHandleCommand_U8_MultiArgIsRawPassthrough(t *testing.T) {
	h := newDutiHarness(t)

	var gotTool string
	var gotArgs []string
	orig := provider.PassthroughExec
	t.Cleanup(func() { provider.PassthroughExec = orig })
	provider.PassthroughExec = func(_ context.Context, tool string, args []string) error {
		gotTool = tool
		gotArgs = append([]string(nil), args...)
		return nil
	}

	wantArgs := []string{"-s", "com.adobe.acrobat.pdf", ".pdf", "all"}
	if err := h.provider.HandleCommand(context.Background(), wantArgs, nil, h.flags); err != nil {
		t.Fatalf("passthrough: %v", err)
	}

	if gotTool != "duti" {
		t.Errorf("PassthroughExec tool = %q, want duti", gotTool)
	}
	if !slices.Equal(gotArgs, wantArgs) {
		t.Errorf("PassthroughExec args = %v, want %v", gotArgs, wantArgs)
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("raw-passthrough wrote hamsfile apps = %v", apps)
	}
	if sf := h.stateFile(); sf != nil {
		t.Errorf("raw-passthrough wrote state file")
	}
}

// U9 — malformed resource ID (missing `=`) returns a usage error
// BEFORE calling the runner.
func TestHandleCommand_U9_MalformedResourceIDReturnsUsageError(t *testing.T) {
	h := newDutiHarness(t)

	// Single arg but no `=` — hits the passthrough path, not the
	// auto-record path. Should not record anything.
	err := h.provider.HandleCommand(context.Background(), []string{"nopdf"}, nil, h.flags)
	_ = err
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("invalid arg wrote hamsfile apps = %v", apps)
	}

	// Single arg with `=` but empty bundle — hits the record path
	// and surfaces a usage error.
	err = h.provider.HandleCommand(context.Background(), []string{"pdf="}, nil, h.flags)
	if err == nil {
		t.Fatalf("expected usage error for empty bundle ID, got nil")
	}
	if h.runner.CallCount(fakeOpSet, "") != 0 {
		t.Errorf("runner.SetDefault called %d times, want 0", h.runner.CallCount(fakeOpSet, ""))
	}
}
