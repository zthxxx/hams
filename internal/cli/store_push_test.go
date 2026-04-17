package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

// fakeStorePushRunner records calls and scripts return values for
// each git step in runStorePush. Fields starting with `force` are
// error injections.
type fakeStorePushRunner struct {
	statusValue    string
	forceStatusErr error
	forceAddErr    error
	forceCommitErr error
	forcePushErr   error
	gotStorePath   string
	gotCommitMsg   string
	gotAddAllCalls int
	gotCommitCalls int
	gotPushCalls   int
	gotStatusCalls int
}

func (f *fakeStorePushRunner) Status(_ context.Context, storePath string) (string, error) {
	f.gotStorePath = storePath
	f.gotStatusCalls++
	return f.statusValue, f.forceStatusErr
}

func (f *fakeStorePushRunner) AddAll(_ context.Context, _ string) error {
	f.gotAddAllCalls++
	return f.forceAddErr
}

func (f *fakeStorePushRunner) Commit(_ context.Context, _, message string) error {
	f.gotCommitCalls++
	f.gotCommitMsg = message
	return f.forceCommitErr
}

func (f *fakeStorePushRunner) Push(_ context.Context, _ string) error {
	f.gotPushCalls++
	return f.forcePushErr
}

func withPushRunner(fake storePushRunner) func() {
	original := pushRunner
	pushRunner = fake
	return func() { pushRunner = original }
}

// TestRunStorePush_CleanTreeSkipsCommit asserts that when `git status
// --porcelain` returns empty (clean working tree), runStorePush
// short-circuits: no add/commit/push — prevents the surprising
// "nothing to commit, working tree clean" exec error that the
// pre-cycle-108 code surfaced after e.g. `hams refresh` (which only
// touches .state/ files, already gitignored).
func TestRunStorePush_CleanTreeSkipsCommit(t *testing.T) {
	fake := &fakeStorePushRunner{statusValue: ""}
	t.Cleanup(withPushRunner(fake))

	if err := runStorePush(context.Background(), "/tmp/store", "hams: update store"); err != nil {
		t.Fatalf("runStorePush on clean tree: %v", err)
	}
	if fake.gotStatusCalls != 1 {
		t.Errorf("Status calls = %d, want 1", fake.gotStatusCalls)
	}
	if fake.gotAddAllCalls != 0 || fake.gotCommitCalls != 0 || fake.gotPushCalls != 0 {
		t.Errorf("clean tree should skip add/commit/push; got add=%d commit=%d push=%d",
			fake.gotAddAllCalls, fake.gotCommitCalls, fake.gotPushCalls)
	}
}

// TestRunStorePush_DirtyTreeRunsAddCommitPush asserts the full
// sequence runs when `git status` shows changes. Also verifies the
// message is forwarded to git commit.
func TestRunStorePush_DirtyTreeRunsAddCommitPush(t *testing.T) {
	fake := &fakeStorePushRunner{statusValue: " M foo.yaml"}
	t.Cleanup(withPushRunner(fake))

	const wantMsg = "hams: install htop"
	if err := runStorePush(context.Background(), "/tmp/store", wantMsg); err != nil {
		t.Fatalf("runStorePush on dirty tree: %v", err)
	}

	if fake.gotAddAllCalls != 1 {
		t.Errorf("AddAll calls = %d, want 1", fake.gotAddAllCalls)
	}
	if fake.gotCommitCalls != 1 {
		t.Errorf("Commit calls = %d, want 1", fake.gotCommitCalls)
	}
	if fake.gotPushCalls != 1 {
		t.Errorf("Push calls = %d, want 1", fake.gotPushCalls)
	}
	if fake.gotCommitMsg != wantMsg {
		t.Errorf("Commit message = %q, want %q", fake.gotCommitMsg, wantMsg)
	}
}

// TestRunStorePush_StatusErrorPropagates asserts a Status failure
// short-circuits and wraps the error — subsequent git calls are
// skipped.
func TestRunStorePush_StatusErrorPropagates(t *testing.T) {
	fake := &fakeStorePushRunner{forceStatusErr: errors.New("git status failed")}
	t.Cleanup(withPushRunner(fake))

	err := runStorePush(context.Background(), "/tmp/store", "msg")
	if err == nil {
		t.Fatalf("expected status error, got nil")
	}
	if fake.gotAddAllCalls != 0 || fake.gotCommitCalls != 0 || fake.gotPushCalls != 0 {
		t.Errorf("status error should skip later steps; got add=%d commit=%d push=%d",
			fake.gotAddAllCalls, fake.gotCommitCalls, fake.gotPushCalls)
	}
}

// TestRunStorePush_AddAllErrorShortCircuits asserts an Add failure
// prevents commit and push from running.
func TestRunStorePush_AddAllErrorShortCircuits(t *testing.T) {
	fake := &fakeStorePushRunner{
		statusValue: " M foo.yaml",
		forceAddErr: errors.New("add failed"),
	}
	t.Cleanup(withPushRunner(fake))

	if err := runStorePush(context.Background(), "/tmp/store", "msg"); err == nil {
		t.Fatalf("expected add error, got nil")
	}
	if fake.gotCommitCalls != 0 || fake.gotPushCalls != 0 {
		t.Errorf("add error should skip commit+push; got commit=%d push=%d",
			fake.gotCommitCalls, fake.gotPushCalls)
	}
}

// TestRunStorePush_CommitErrorShortCircuits asserts a Commit failure
// prevents push from running (so we don't push a dirty working tree
// that wasn't actually committed).
func TestRunStorePush_CommitErrorShortCircuits(t *testing.T) {
	fake := &fakeStorePushRunner{
		statusValue:    " M foo.yaml",
		forceCommitErr: errors.New("commit failed"),
	}
	t.Cleanup(withPushRunner(fake))

	if err := runStorePush(context.Background(), "/tmp/store", "msg"); err == nil {
		t.Fatalf("expected commit error, got nil")
	}
	if fake.gotPushCalls != 0 {
		t.Errorf("commit error should skip push; got push=%d", fake.gotPushCalls)
	}
}

// TestConfigEditDryRun_SkipsMutationsAndEditor locks in cycle
// 146: `hams --dry-run config edit` prints "Would open <path> in
// <editor>" and returns without (a) creating the config dir, (b)
// creating the stub config file, or (c) exec-ing the editor.
// Previously --dry-run was ignored: the edit command performed
// the MkdirAll + WriteFile stub + editor exec regardless.
func TestConfigEditDryRun_SkipsMutationsAndEditor(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	// Point EDITOR at a bogus binary so that if the dry-run
	// accidentally fell through to exec, the test would fail loudly
	// with "executable file not found" instead of silently passing
	// by running a random editor in CI.
	t.Setenv("EDITOR", "this-editor-must-not-be-exec")

	configPath := filepath.Join(configHome, "hams.config.yaml")

	out := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--dry-run", "config", "edit"}); err != nil {
			t.Fatalf("dry-run config edit: %v", err)
		}
	})

	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("dry-run output missing [dry-run] prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "this-editor-must-not-be-exec") {
		t.Errorf("dry-run output should echo the editor name; got:\n%s", out)
	}
	// Config file MUST NOT have been created by the dry-run.
	if _, err := os.Stat(configPath); err == nil {
		t.Errorf("dry-run created config file %q; should have been untouched", configPath)
	}
}

// TestConfigSetDryRun_SkipsWrite locks in cycle 145: `hams
// --dry-run config set <key> <val>` prints the intent-level
// preview and returns without invoking WriteConfigKey. Previously
// dry-run was ignored: the real config file got mutated.
func TestConfigSetDryRun_SkipsWrite(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	// Start without a config file so we can assert none was
	// created by the dry-run invocation.
	configPath := filepath.Join(configHome, "hams.config.yaml")

	out := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--dry-run", "config", "set", "profile_tag", "linux"}); err != nil {
			t.Fatalf("dry-run config set: %v", err)
		}
	})

	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("dry-run output missing [dry-run] prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "profile_tag = linux") {
		t.Errorf("dry-run output should echo the set key=value; got:\n%s", out)
	}
	// Config file MUST NOT exist — dry-run didn't mutate anything.
	if _, err := os.Stat(configPath); err == nil {
		t.Errorf("dry-run created config file %q; should have been untouched", configPath)
	}
}

// TestConfigUnsetDryRun_SkipsUnset locks in cycle 145's mirror
// fix: `hams --dry-run config unset <key>` doesn't mutate the
// config file.
func TestConfigUnsetDryRun_SkipsUnset(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	// Seed a config file with profile_tag set so "unset" has
	// something to potentially remove.
	configPath := filepath.Join(configHome, "hams.config.yaml")
	writeApplyTestFile(t, configPath, "profile_tag: keep-me\nmachine_id: sandbox\n")

	out := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--dry-run", "config", "unset", "profile_tag"}); err != nil {
			t.Fatalf("dry-run config unset: %v", err)
		}
	})

	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("dry-run output missing [dry-run] prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "Would unset profile_tag") {
		t.Errorf("dry-run output should echo the unset key; got:\n%s", out)
	}
	// Config file MUST still have profile_tag set — dry-run didn't
	// mutate anything.
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(body), "profile_tag: keep-me") {
		t.Errorf("dry-run removed profile_tag from config; should have been untouched; file:\n%s", string(body))
	}
}

// TestStoreInitDryRun_SkipsAllSideEffects locks in cycle 144:
// `hams --dry-run store init` prints the intent-level preview and
// returns without creating any directory or writing any file.
// Previously --dry-run was ignored: init performed the real
// mkdir + yaml/gitignore writes regardless, contradicting the
// global flag's "no changes" contract.
func TestStoreInitDryRun_SkipsAllSideEffects(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	storeDir := filepath.Join(t.TempDir(), "fresh-store") // intentionally non-existent
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: dev\nmachine_id: sandbox\nstore_path: "+storeDir+"\n")

	out := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--dry-run", "store", "init"}); err != nil {
			t.Fatalf("dry-run store init: %v", err)
		}
	})

	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("dry-run output missing [dry-run] prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "Would create profile dir") {
		t.Errorf("dry-run output should preview profile dir creation; got:\n%s", out)
	}
	// The store_path (and profile/state subdirs) MUST NOT have been
	// created by the dry-run.
	if _, err := os.Stat(storeDir); err == nil {
		t.Errorf("dry-run created storeDir %q; should have been untouched", storeDir)
	}
}

// TestStorePushDryRun_SkipsAllSideEffects locks in cycle 143:
// `hams --dry-run store push` prints the intent-level preview and
// returns without invoking any git operation. Previously --dry-run
// was ignored entirely: the command performed the real commit +
// push, contradicting the global flag's documented "no changes"
// contract.
func TestStorePushDryRun_SkipsAllSideEffects(t *testing.T) {
	fake := &fakeStorePushRunner{statusValue: " M foo.yaml"}
	t.Cleanup(withPushRunner(fake))

	configHome := t.TempDir()
	dataHome := t.TempDir()
	storeDir := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	// Seed a .git so ensureStoreIsGitRepo passes.
	if err := os.MkdirAll(filepath.Join(storeDir, ".git"), 0o750); err != nil {
		t.Fatalf("seed .git: %v", err)
	}
	writeApplyTestFile(t, filepath.Join(configHome, "hams.config.yaml"),
		"profile_tag: t\nmachine_id: m\nstore_path: "+storeDir+"\n")

	out := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--dry-run", "store", "push", "-m", "test message"}); err != nil {
			t.Fatalf("dry-run store push: %v", err)
		}
	})

	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("dry-run output missing [dry-run] prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "test message") {
		t.Errorf("dry-run output should echo commit message; got:\n%s", out)
	}
	// Fake runner MUST NOT have been invoked at all.
	if fake.gotStatusCalls != 0 || fake.gotAddAllCalls != 0 || fake.gotCommitCalls != 0 || fake.gotPushCalls != 0 {
		t.Errorf("dry-run should not invoke any git step; got status=%d add=%d commit=%d push=%d",
			fake.gotStatusCalls, fake.gotAddAllCalls, fake.gotCommitCalls, fake.gotPushCalls)
	}
}

// TestRunStorePush_PushErrorSurfaces asserts a Push failure is
// returned so the caller (shell script, CI pipeline) sees a non-zero
// exit code.
func TestRunStorePush_PushErrorSurfaces(t *testing.T) {
	fake := &fakeStorePushRunner{
		statusValue:  " M foo.yaml",
		forcePushErr: errors.New("push failed"),
	}
	t.Cleanup(withPushRunner(fake))

	if err := runStorePush(context.Background(), "/tmp/store", "msg"); err == nil {
		t.Fatalf("expected push error, got nil")
	}
}
