package git

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

// gitHarness wires a ConfigProvider against a FakeCmdRunner + tempdir
// profile so HandleCommand tests can assert real hamsfile/state writes
// without ever exec-ing the host's `git` binary.
type gitHarness struct {
	t            *testing.T
	storeDir     string
	profileDir   string
	stateDir     string
	hamsfilePath string
	statePath    string
	flags        *provider.GlobalFlags
	runner       *FakeCmdRunner
	provider     *ConfigProvider
}

func newGitHarness(t *testing.T) *gitHarness {
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
	p := NewConfigProvider(cfg).WithRunner(runner)
	return &gitHarness{
		t:            t,
		storeDir:     storeDir,
		profileDir:   profileDir,
		stateDir:     stateDir,
		hamsfilePath: filepath.Join(profileDir, "git-config.hams.yaml"),
		statePath:    filepath.Join(stateDir, "git-config.state.yaml"),
		flags:        &provider.GlobalFlags{Store: storeDir, Profile: profileTag},
		runner:       runner,
		provider:     p,
	}
}

func (h *gitHarness) hamsfileApps() []string {
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

func (h *gitHarness) stateFile() *state.File {
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

// U1 — `hams git-config <key> <value>` with no args returns a usage
// error AND does not call the runner.
func TestHandleCommand_U1_NoArgsReturnsUsageError(t *testing.T) {
	h := newGitHarness(t)

	err := h.provider.HandleCommand(context.Background(), []string{}, nil, h.flags)
	if err == nil {
		t.Fatalf("expected usage error, got nil")
	}
	if len(h.runner.SetCalls) != 0 {
		t.Errorf("runner.SetGlobal called %d times with empty args, want 0", len(h.runner.SetCalls))
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("hamsfile apps = %v, want none", apps)
	}
}

// U2 — `hams git-config <key>` with only one arg also returns a usage
// error. Protects users from accidentally running `git config --global
// user.name` with no value, which in real git would print the current
// value instead of setting it.
func TestHandleCommand_U2_OneArgReturnsUsageError(t *testing.T) {
	h := newGitHarness(t)

	err := h.provider.HandleCommand(context.Background(), []string{"user.name"}, nil, h.flags)
	if err == nil {
		t.Fatalf("expected usage error, got nil")
	}
	if len(h.runner.SetCalls) != 0 {
		t.Errorf("runner.SetGlobal called %d times, want 0", len(h.runner.SetCalls))
	}
}

// U3 — `hams git-config <key> <value>` in dry-run mode prints but
// neither calls the runner nor writes to hamsfile/state.
func TestHandleCommand_U3_DryRunSkipsAllSideEffects(t *testing.T) {
	h := newGitHarness(t)
	h.flags.DryRun = true

	if err := h.provider.HandleCommand(context.Background(), []string{"user.name", "zthxxx"}, nil, h.flags); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if len(h.runner.SetCalls) != 0 {
		t.Errorf("runner.SetGlobal called %d times in dry-run, want 0", len(h.runner.SetCalls))
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("dry-run wrote hamsfile apps = %v", apps)
	}
	if sf := h.stateFile(); sf != nil {
		t.Errorf("dry-run wrote state file")
	}
}

// U4 — `hams git-config user.name zthxxx` auto-records to hamsfile
// and state AND calls runner.SetGlobal exactly once.
func TestHandleCommand_U4_InstallAutoRecordsToHamsfileAndState(t *testing.T) {
	h := newGitHarness(t)

	if err := h.provider.HandleCommand(context.Background(), []string{"user.name", "zthxxx"}, nil, h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(h.runner.SetCalls) != 1 {
		t.Fatalf("runner.SetGlobal called %d times, want 1", len(h.runner.SetCalls))
	}
	if h.runner.SetCalls[0].Key != "user.name" || h.runner.SetCalls[0].Value != "zthxxx" {
		t.Errorf("runner.SetGlobal call = %+v, want {user.name zthxxx}", h.runner.SetCalls[0])
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "user.name=zthxxx" {
		t.Errorf("hamsfile apps = %v, want [user.name=zthxxx]", apps)
	}
	sf := h.stateFile()
	if sf == nil {
		t.Fatalf("state file missing")
	}
	r, ok := sf.Resources["user.name=zthxxx"]
	if !ok {
		t.Fatalf("state missing user.name=zthxxx resource; have %v", keys(sf.Resources))
	}
	if r.State != state.StateOK {
		t.Errorf("state.Resources[user.name=zthxxx].State = %v, want StateOK", r.State)
	}
	if r.Value != "zthxxx" {
		t.Errorf("state.Resources[user.name=zthxxx].Value = %q, want zthxxx", r.Value)
	}
}

// U5 — re-running with the same key+value is idempotent: the hamsfile
// keeps exactly one entry, state stays ok, and the runner is invoked
// twice (CLI always runs git config to be sure the host is in sync,
// but the hamsfile is single-valued per key).
func TestHandleCommand_U5_RerunSameValueIsIdempotent(t *testing.T) {
	h := newGitHarness(t)

	for i := range 2 {
		if err := h.provider.HandleCommand(context.Background(), []string{"user.name", "zthxxx"}, nil, h.flags); err != nil {
			t.Fatalf("install iteration %d: %v", i, err)
		}
	}
	if len(h.runner.SetCalls) != 2 {
		t.Errorf("runner.SetGlobal called %d times, want 2", len(h.runner.SetCalls))
	}
	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "user.name=zthxxx" {
		t.Errorf("hamsfile apps = %v, want single [user.name=zthxxx]", apps)
	}
}

// U6 — re-running with the SAME key but a DIFFERENT value replaces
// the old hamsfile entry in place: the stale value is removed and the
// new one is recorded. State marks the old entry removed and the new
// one ok. This keeps the hamsfile aligned with `git config`'s
// overwrite semantics (the host only has one value for user.name at a
// time; the hamsfile should reflect that).
func TestHandleCommand_U6_RerunNewValueReplacesEntry(t *testing.T) {
	h := newGitHarness(t)

	if err := h.provider.HandleCommand(context.Background(), []string{"user.name", "zthxxx"}, nil, h.flags); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := h.provider.HandleCommand(context.Background(), []string{"user.name", "zthxxx2"}, nil, h.flags); err != nil {
		t.Fatalf("second install: %v", err)
	}

	apps := h.hamsfileApps()
	if len(apps) != 1 || apps[0] != "user.name=zthxxx2" {
		t.Errorf("hamsfile apps = %v, want [user.name=zthxxx2]", apps)
	}

	sf := h.stateFile()
	if sf == nil {
		t.Fatalf("state file missing")
	}
	oldR, ok := sf.Resources["user.name=zthxxx"]
	if !ok {
		t.Fatalf("state missing old entry")
	}
	if oldR.State != state.StateRemoved {
		t.Errorf("old entry state = %v, want StateRemoved", oldR.State)
	}
	newR, ok := sf.Resources["user.name=zthxxx2"]
	if !ok {
		t.Fatalf("state missing new entry")
	}
	if newR.State != state.StateOK || newR.Value != "zthxxx2" {
		t.Errorf("new entry = {State:%v Value:%q}, want {ok zthxxx2}", newR.State, newR.Value)
	}
}

// U7 — when runner.SetGlobal fails, the CLI returns the error AND
// does not record anything to hamsfile/state. Protects the user from
// a hamsfile that claims a setting that the host never actually has.
func TestHandleCommand_U7_RunnerErrorShortCircuitsRecord(t *testing.T) {
	h := newGitHarness(t)
	h.runner.ForceSetError = errors.New("git exited non-zero")

	err := h.provider.HandleCommand(context.Background(), []string{"user.name", "zthxxx"}, nil, h.flags)
	if err == nil {
		t.Fatalf("expected runner error, got nil")
	}
	if apps := h.hamsfileApps(); len(apps) != 0 {
		t.Errorf("failed install still wrote hamsfile apps = %v", apps)
	}
	if sf := h.stateFile(); sf != nil {
		t.Errorf("failed install still wrote state file")
	}
}

// U8 — different keys coexist in the hamsfile as independent entries.
// This guards against the "replace by prefix" logic accidentally
// treating unrelated keys as collisions.
func TestHandleCommand_U8_IndependentKeysCoexist(t *testing.T) {
	h := newGitHarness(t)

	for _, kv := range [][2]string{{"user.name", "zthxxx"}, {"user.email", "zth@example.com"}} {
		if err := h.provider.HandleCommand(context.Background(), []string{kv[0], kv[1]}, nil, h.flags); err != nil {
			t.Fatalf("install %v: %v", kv, err)
		}
	}

	got := h.hamsfileApps()
	want := map[string]bool{"user.name=zthxxx": true, "user.email=zth@example.com": true}
	if len(got) != len(want) {
		t.Fatalf("hamsfile apps = %v, want %v", got, want)
	}
	for _, app := range got {
		if !want[app] {
			t.Errorf("unexpected hamsfile app %q", app)
		}
	}
}

// U9 — `--hams-local` flag routes the recorded entry into the
// .hams.local.yaml variant instead of the shared hamsfile. Mirrors
// apt's local-override behavior.
func TestHandleCommand_U9_HamsLocalFlagRoutesToLocalFile(t *testing.T) {
	h := newGitHarness(t)
	localPath := filepath.Join(h.profileDir, "git-config.hams.local.yaml")

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"user.name", "zthxxx"},
		map[string]string{"local": ""},
		h.flags); err != nil {
		t.Fatalf("install: %v", err)
	}

	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("expected local hamsfile at %q: %v", localPath, err)
	}
	if _, err := os.Stat(h.hamsfilePath); err == nil {
		t.Errorf("unexpected shared hamsfile at %q; --hams-local should only write local variant", h.hamsfilePath)
	}
}

// keys is a small helper to format resource-map keys for assertion
// failures.
func keys(m map[string]*state.Resource) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
