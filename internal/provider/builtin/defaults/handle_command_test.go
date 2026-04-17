package defaults

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

// defaultsHarness wires a defaults Provider against a FakeCmdRunner +
// tempdir profile so HandleCommand tests can assert real hamsfile/
// state writes without ever exec-ing the host's `defaults` binary.
type defaultsHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	hamsfilePath string
	statePath    string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *Provider
}

func newDefaultsHarness(t *testing.T) *defaultsHarness {
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
	return &defaultsHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		hamsfilePath: filepath.Join(profileDir, "defaults.hams.yaml"),
		statePath:    filepath.Join(stateDir, "defaults.state.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     New(cfg, runner),
	}
}

func (h *defaultsHarness) hamsfileApps() []string {
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

func (h *defaultsHarness) stateFile() *state.File {
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
	h := newDefaultsHarness(t)

	err := h.provider.HandleCommand(context.Background(), []string{}, nil, h.flags)
	if err == nil {
		t.Fatalf("expected usage error, got nil")
	}
	if h.runner.CallCount(fakeOpWrite, "") != 0 {
		t.Errorf("runner.Write called %d times with empty args, want 0", h.runner.CallCount(fakeOpWrite, ""))
	}
}

// U2 — `defaults write <...>` in dry-run mode prints but neither
// calls the runner nor writes to hamsfile/state.
func TestHandleCommand_U2_WriteDryRunSkipsAllSideEffects(t *testing.T) {
	h := newDefaultsHarness(t)
	h.flags.DryRun = true

	err := h.provider.HandleCommand(context.Background(),
		[]string{"write", "com.apple.dock", "autohide", "-bool", "true"}, nil, h.flags)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if h.runner.CallCount(fakeOpWrite, "") != 0 {
		t.Errorf("runner.Write called %d times in dry-run, want 0", h.runner.CallCount(fakeOpWrite, ""))
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("dry-run wrote hamsfile apps = %v", apps)
	}
	if sf := h.stateFile(); sf != nil {
		t.Errorf("dry-run wrote state file")
	}
}

// TestHandleCommand_WriteStrictArgCount locks in cycle 164: the
// pre-cycle-164 implementation accepted 5 OR MORE args and silently
// dropped the rest. Critical failure: `hams defaults write
// com.apple.dock SetText -string Hello World` (forgot to quote)
// silently called `defaults write … "Hello"` and recorded only
// "Hello" — far worse than a typo because the user thought the full
// "Hello World" string was set. Now: surface the mismatch with a
// quoting hint.
func TestHandleCommand_WriteStrictArgCount(t *testing.T) {
	t.Parallel()
	cases := [][]string{
		// Forgot to quote multi-word value — common case.
		{"write", "com.apple.dock", "SetText", "-string", "Hello", "World"},
		// Stray trailing arg.
		{"write", "com.apple.dock", "autohide", "-bool", "true", "extra"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			t.Parallel()
			h := newDefaultsHarness(t)
			err := h.provider.HandleCommand(context.Background(), args, nil, h.flags)
			if err == nil {
				t.Fatalf("expected usage error for %v; got nil", args)
			}
			if h.runner.CallCount(fakeOpWrite, "") != 0 {
				t.Errorf("runner.Write must not be invoked on usage error")
			}
			if apps := h.hamsfileApps(); len(apps) != 0 {
				t.Errorf("hamsfile must not be mutated on usage error, got %v", apps)
			}
			if !strings.Contains(err.Error(), "exactly 4 args") {
				t.Errorf("error should say 'exactly 4 args'; got %q", err.Error())
			}
		})
	}
}

// TestHandleCommand_DeleteStrictArgCount locks in the parallel for
// `defaults delete`. Multi-key deletion was silently dropped.
func TestHandleCommand_DeleteStrictArgCount(t *testing.T) {
	t.Parallel()
	args := []string{"delete", "com.apple.dock", "autohide", "other-key"}
	h := newDefaultsHarness(t)
	err := h.provider.HandleCommand(context.Background(), args, nil, h.flags)
	if err == nil {
		t.Fatalf("expected usage error for %v; got nil", args)
	}
	if h.runner.CallCount(fakeOpDelete, "") != 0 {
		t.Errorf("runner.Delete must not be invoked on usage error")
	}
	if !strings.Contains(err.Error(), "exactly 2 args") {
		t.Errorf("error should say 'exactly 2 args'; got %q", err.Error())
	}
}

// U3 — `defaults write` auto-records to hamsfile + state and calls
// runner.Write with the parsed (domain, key, type, value).
func TestHandleCommand_U3_WriteAutoRecordsToHamsfileAndState(t *testing.T) {
	h := newDefaultsHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"write", "com.apple.dock", "autohide", "-bool", "true"}, nil, h.flags); err != nil {
		t.Fatalf("write: %v", err)
	}
	if h.runner.CallCount(fakeOpWrite, "com.apple.dock:autohide") != 1 {
		t.Errorf("runner.Write(dock.autohide) = %d, want 1",
			h.runner.CallCount(fakeOpWrite, "com.apple.dock:autohide"))
	}

	const wantID = "com.apple.dock.autohide=bool:true"
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != wantID {
		t.Errorf("hamsfile apps = %v, want [%s]", apps, wantID)
	}

	sf := h.stateFile()
	if sf == nil {
		t.Fatalf("state file missing")
	}
	r, ok := sf.Resources[wantID]
	if !ok {
		t.Fatalf("state missing %s resource", wantID)
	}
	if r.State != state.StateOK || r.Value != "true" {
		t.Errorf("state.Resources[%s] = {State:%v Value:%q}, want {ok true}", wantID, r.State, r.Value)
	}
}

// U4 — re-write with the same (domain, key) but a DIFFERENT value
// replaces the old hamsfile entry in place. Mirrors git-config's U6.
func TestHandleCommand_U4_RewriteNewValueReplacesEntry(t *testing.T) {
	h := newDefaultsHarness(t)

	for _, v := range []string{"true", "false"} {
		if err := h.provider.HandleCommand(context.Background(),
			[]string{"write", "com.apple.dock", "autohide", "-bool", v}, nil, h.flags); err != nil {
			t.Fatalf("write %s: %v", v, err)
		}
	}

	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "com.apple.dock.autohide=bool:false" {
		t.Errorf("hamsfile apps = %v, want [com.apple.dock.autohide=bool:false]", apps)
	}

	sf := h.stateFile()
	if sf == nil {
		t.Fatalf("state file missing")
	}
	if oldR, ok := sf.Resources["com.apple.dock.autohide=bool:true"]; !ok || oldR.State != state.StateRemoved {
		t.Errorf("old entry state = %v, want StateRemoved (present=%v)", oldR, ok)
	}
	if newR, ok := sf.Resources["com.apple.dock.autohide=bool:false"]; !ok || newR.State != state.StateOK {
		t.Errorf("new entry state = %v, want StateOK (present=%v)", newR, ok)
	}
}

// U5 — runner.Write error short-circuits the record path. The
// hamsfile and state file stay untouched so they never claim a
// setting `defaults` rejected.
func TestHandleCommand_U5_WriteErrorShortCircuitsRecord(t *testing.T) {
	h := newDefaultsHarness(t)
	h.runner.WithWriteError("com.apple.dock", "autohide", errors.New("boom"))

	err := h.provider.HandleCommand(context.Background(),
		[]string{"write", "com.apple.dock", "autohide", "-bool", "true"}, nil, h.flags)
	if err == nil {
		t.Fatalf("expected runner error, got nil")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("failed write still wrote hamsfile apps = %v", apps)
	}
	if sf := h.stateFile(); sf != nil {
		t.Errorf("failed write still wrote state file")
	}
}

// U6 — `defaults delete` removes the matching hamsfile entry and
// marks the state resource as removed.
func TestHandleCommand_U6_DeleteRemovesHamsfileEntryAndMarksStateRemoved(t *testing.T) {
	h := newDefaultsHarness(t)

	// Seed via write first so the hamsfile has an entry to delete.
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"write", "com.apple.dock", "autohide", "-bool", "true"}, nil, h.flags); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(),
		[]string{"delete", "com.apple.dock", "autohide"}, nil, h.flags); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if h.runner.CallCount(fakeOpDelete, "com.apple.dock:autohide") != 1 {
		t.Errorf("runner.Delete called %d times, want 1",
			h.runner.CallCount(fakeOpDelete, "com.apple.dock:autohide"))
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("post-delete hamsfile apps = %v, want []", apps)
	}
	sf := h.stateFile()
	if sf == nil {
		t.Fatalf("state file missing")
	}
	r, ok := sf.Resources["com.apple.dock.autohide=bool:true"]
	if !ok || r.State != state.StateRemoved {
		t.Errorf("state.Resources[com.apple.dock.autohide=bool:true] = (present=%v state=%v), want removed",
			ok, r)
	}
}

// U7 — `defaults delete` with no matching hamsfile entry still
// records a StateRemoved tombstone so a future apply doesn't re-assert
// an old value (and so `hams list` can show the delete in the audit
// trail).
func TestHandleCommand_U7_DeleteWithoutPriorWriteRecordsTombstone(t *testing.T) {
	h := newDefaultsHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"delete", "com.apple.dock", "autohide"}, nil, h.flags); err != nil {
		t.Fatalf("delete: %v", err)
	}

	sf := h.stateFile()
	if sf == nil {
		t.Fatalf("state file missing")
	}
	r, ok := sf.Resources["com.apple.dock.autohide="]
	if !ok || r.State != state.StateRemoved {
		t.Errorf("tombstone missing: resources = %v", sf.Resources)
	}
}

// U8 — `defaults read` passes through to exec (since no test harness
// for exec passthrough, just assert we do NOT touch hamsfile/state).
func TestHandleCommand_U8_ReadPassesThroughWithoutRecording(t *testing.T) {
	h := newDefaultsHarness(t)

	// `read` takes a different path (exec.CommandContext); in tests
	// it'll fail (no defaults binary on Linux CI), which is fine —
	// we only assert that the record path was NOT entered.
	err := h.provider.HandleCommand(context.Background(),
		[]string{"read", "com.apple.dock", "autohide"}, nil, h.flags)
	_ = err // intentional: exec result is irrelevant; we check side-effects below

	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("read path wrote hamsfile apps = %v", apps)
	}
	if sf := h.stateFile(); sf != nil {
		t.Errorf("read path wrote state file")
	}
}

// U9 — `--hams-local` routes the auto-record entry into the
// .hams.local.yaml variant instead of the shared hamsfile.
func TestHandleCommand_U9_HamsLocalFlagRoutesToLocalFile(t *testing.T) {
	h := newDefaultsHarness(t)
	localPath := filepath.Join(h.profileDir, "defaults.hams.local.yaml")

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"write", "com.apple.dock", "autohide", "-bool", "true"},
		map[string]string{"local": ""},
		h.flags); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("expected local hamsfile at %q: %v", localPath, err)
	}
	if _, err := os.Stat(h.hamsfilePath); err == nil {
		t.Errorf("unexpected shared hamsfile at %q; --hams-local should only write local", h.hamsfilePath)
	}
}

// U10 — after write, the hamsfile entry carries a preview-cmd field
// so `hams list` can render the reproduction command. We check the
// raw YAML rather than a getter because hamsfile doesn't surface
// PreviewCmd — the field lives alongside the other entry extras.
func TestHandleCommand_U10_WriteRecordsPreviewCmd(t *testing.T) {
	h := newDefaultsHarness(t)

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"write", "com.apple.dock", "autohide", "-bool", "true"}, nil, h.flags); err != nil {
		t.Fatalf("write: %v", err)
	}

	body, err := os.ReadFile(h.hamsfilePath)
	if err != nil {
		t.Fatalf("read hamsfile: %v", err)
	}
	want := "preview-cmd: defaults write com.apple.dock autohide -bool true"
	if !strings.Contains(string(body), want) {
		t.Errorf("hamsfile missing preview-cmd line %q; body=\n%s", want, string(body))
	}
}
