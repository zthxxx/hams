package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hamserr "github.com/zthxxx/hams/internal/error"
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

// TestConfigEdit_EditorWithArgs locks in cycle 158: $EDITOR can
// carry args (e.g. "code -w", "emacs -nw", "nvim -p"). The
// pre-cycle-158 implementation passed the whole string to
// exec.CommandContext as a single binary path → "executable file
// not found" for any non-bare $EDITOR. Now: split on whitespace,
// exec the first field as the binary and forward the rest plus
// the config path as args.
//
// Test wires $EDITOR to a fake script that writes its argv to a
// marker file, then asserts the marker shows the args were
// forwarded correctly.
func TestConfigEdit_EditorWithArgs(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	// Write a tiny shell script that records its args and exits 0.
	scriptDir := t.TempDir()
	markerPath := filepath.Join(scriptDir, "args.txt")
	scriptPath := filepath.Join(scriptDir, "fake-editor.sh")
	scriptBody := "#!/bin/sh\nfor a in \"$@\"; do echo \"$a\"; done > " + markerPath + "\n"
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}

	// $EDITOR carries TWO args (-x foo) AND the script path. The pre-
	// cycle-158 code would try to exec the whole literal string as a
	// binary path and fail.
	t.Setenv("EDITOR", scriptPath+" -x foo")

	app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
	if err := app.Run(context.Background(), []string{"hams", "config", "edit"}); err != nil {
		t.Fatalf("config edit with multi-arg EDITOR: %v", err)
	}

	body, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	gotArgs := strings.Split(strings.TrimSpace(string(body)), "\n")

	// We expect: ["-x", "foo", "<configPath>"]
	if len(gotArgs) != 3 {
		t.Fatalf("expected 3 args (-x foo <configPath>); got %d: %v", len(gotArgs), gotArgs)
	}
	if gotArgs[0] != "-x" {
		t.Errorf("args[0] = %q, want '-x'", gotArgs[0])
	}
	if gotArgs[1] != "foo" {
		t.Errorf("args[1] = %q, want 'foo'", gotArgs[1])
	}
	wantConfigPath := filepath.Join(configHome, "hams.config.yaml")
	if gotArgs[2] != wantConfigPath {
		t.Errorf("args[2] = %q, want %q", gotArgs[2], wantConfigPath)
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

// TestConfigSet_JSONMode_EmitsStructuredResult locks in cycle 246:
// `hams --json config set <key> <value>` emits a JSON object with
// key/value/target/dry_run instead of the text "Set <key> = <value>
// (in <target>)". Pre-cycle-246 both --json and text mode produced
// identical text output — a CI script running `hams --json config set
// foo bar | jq '.key'` failed because stdout was non-JSON, breaking
// the convention that other commands (apply/refresh/list/config
// get/config list/version) establish.
func TestConfigSet_JSONMode_EmitsStructuredResult(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	out := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--json", "config", "set", "profile_tag", "linux"}); err != nil {
			t.Fatalf("json config set: %v", err)
		}
	})

	assertConfigCmdJSON(t, out, map[string]any{
		"key":     "profile_tag",
		"value":   "linux",
		"target":  "global config",
		"dry_run": false,
	})
}

// TestConfigSet_JSONMode_DryRunFlagsDryRunTrue is the --dry-run
// variant of the set shape test: same shape, dry_run=true.
func TestConfigSet_JSONMode_DryRunFlagsDryRunTrue(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	out := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--json", "--dry-run", "config", "set", "profile_tag", "linux"}); err != nil {
			t.Fatalf("json dry-run config set: %v", err)
		}
	})

	assertConfigCmdJSON(t, out, map[string]any{
		"key":     "profile_tag",
		"value":   "linux",
		"target":  "global config",
		"dry_run": true,
	})
	// Config file MUST NOT exist (dry-run) — guard that JSON path
	// still respects --dry-run's no-mutation contract.
	configPath := filepath.Join(configHome, "hams.config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		t.Errorf("dry-run --json config set created config file at %q; should have been untouched", configPath)
	}
}

// TestConfigUnset_JSONMode_EmitsStructuredResult is the mirror test
// for cycle 246 on the unset path.
func TestConfigUnset_JSONMode_EmitsStructuredResult(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	configPath := filepath.Join(configHome, "hams.config.yaml")
	writeApplyTestFile(t, configPath, "profile_tag: keep-me\nmachine_id: sandbox\n")

	out := captureStdout(t, func() {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		if err := app.Run(context.Background(),
			[]string{"hams", "--json", "config", "unset", "profile_tag"}); err != nil {
			t.Fatalf("json config unset: %v", err)
		}
	})

	assertConfigCmdJSON(t, out, map[string]any{
		"key":     "profile_tag",
		"target":  "global config",
		"dry_run": false,
	})
}

// assertConfigCmdJSON unmarshals the captured stdout and compares
// each expected key/value pair. Helper factored for the three cycle-246
// regression tests above.
func assertConfigCmdJSON(t *testing.T, out string, want map[string]any) {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %q", err, out)
	}
	for k, v := range want {
		if data[k] != v {
			t.Errorf("%s = %v, want %v\nfull: %v", k, data[k], v, data)
		}
	}
}

// TestConfigSet_StrictArgCount locks in cycle 156: `hams config set`
// previously accepted >= 2 args and silently dropped extras. Critical
// failure mode: `hams config set notification.bark_token abc def ghi`
// (forgot to quote a token containing spaces) silently stored only
// "abc". Far worse than a typo: users believed the token was set
// correctly. Now: surface the mismatch with a hint about quoting.
func TestConfigSet_StrictArgCount(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)

	cases := []struct {
		name string
		args []string
	}{
		{"too-few-zero-args", []string{"hams", "config", "set"}},
		{"too-few-one-arg", []string{"hams", "config", "set", "profile_tag"}},
		{"too-many-three-args", []string{"hams", "config", "set", "notification.bark_token", "abc", "def"}},
		{"too-many-four-args", []string{"hams", "config", "set", "key", "abc", "def", "ghi"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
			err := app.Run(context.Background(), tc.args)
			if err == nil {
				t.Fatalf("expected error for %v; got nil", tc.args)
			}
			var ufe *hamserr.UserFacingError
			if !errors.As(err, &ufe) {
				t.Fatalf("expected *UserFacingError, got %T: %v", err, err)
			}
			if ufe.Code != hamserr.ExitUsageError {
				t.Errorf("Code = %d, want ExitUsageError", ufe.Code)
			}
			if !strings.Contains(ufe.Message, "exactly one key") {
				t.Errorf("error message should say 'exactly one key'; got %q", ufe.Message)
			}
		})
	}

	// Quoting-hint suggestion must appear in too-many-args case so the
	// user understands they may have forgotten to quote a value with
	// spaces.
	app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
	err := app.Run(context.Background(),
		[]string{"hams", "config", "set", "notification.bark_token", "abc", "def"})
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T", err)
	}
	joined := strings.Join(ufe.Suggestions, " | ")
	if !strings.Contains(joined, "Quote") {
		t.Errorf("suggestions should hint about quoting; got: %q", joined)
	}
}

// TestConfigGet_StrictArgCount locks in cycle 156: silent extra-arg
// drop hides typos.
func TestConfigGet_StrictArgCount(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", t.TempDir())

	cases := [][]string{
		{"hams", "config", "get"},                          // zero args
		{"hams", "config", "get", "profile_tag", "extra"},  // extra arg
		{"hams", "config", "get", "profile_tag", "a", "b"}, // extra args
	}
	for _, args := range cases {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		err := app.Run(context.Background(), args)
		if err == nil {
			t.Errorf("expected error for %v; got nil", args)
			continue
		}
		var ufe *hamserr.UserFacingError
		if !errors.As(err, &ufe) {
			t.Errorf("expected *UserFacingError for %v, got %T", args, err)
			continue
		}
		if !strings.Contains(ufe.Message, "exactly one key") {
			t.Errorf("error message should say 'exactly one key'; got %q", ufe.Message)
		}
	}
}

// TestConfigUnset_StrictArgCount locks in cycle 156's mirror.
func TestConfigUnset_StrictArgCount(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", t.TempDir())

	cases := [][]string{
		{"hams", "config", "unset"},                         // zero args
		{"hams", "config", "unset", "profile_tag", "extra"}, // extra arg
	}
	for _, args := range cases {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		err := app.Run(context.Background(), args)
		if err == nil {
			t.Errorf("expected error for %v; got nil", args)
			continue
		}
		var ufe *hamserr.UserFacingError
		if !errors.As(err, &ufe) {
			t.Errorf("expected *UserFacingError for %v, got %T", args, err)
			continue
		}
		if !strings.Contains(ufe.Message, "exactly one key") {
			t.Errorf("error message should say 'exactly one key'; got %q", ufe.Message)
		}
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

// TestConfigEdit_MissingEditorEmitsFriendlyError locks in cycle 228:
// $EDITOR pointing at a missing binary previously produced the opaque
// "fork/exec /nonexistent/binary: no such file or directory" error
// from exec.Run. Now: pre-checked via exec.LookPath and surfaced as
// a UserFacingError with ExitNotFound code + actionable suggestions
// (install the editor, pick a different one, edit config file directly).
func TestConfigEdit_MissingEditorEmitsFriendlyError(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("HAMS_CONFIG_HOME", configHome)
	t.Setenv("HAMS_DATA_HOME", dataHome)
	t.Setenv("EDITOR", "/absolutely-nonexistent-editor-binary-xyz")

	app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
	err := app.Run(context.Background(), []string{"hams", "config", "edit"})
	if err == nil {
		t.Fatal("expected error when $EDITOR is missing")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("want *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitNotFound {
		t.Errorf("Code = %d, want ExitNotFound (%d)", ufe.Code, hamserr.ExitNotFound)
	}
	if !strings.Contains(ufe.Message, "absolutely-nonexistent-editor-binary-xyz") {
		t.Errorf("message should name the missing editor; got %q", ufe.Message)
	}
	// Message must name "$EDITOR" so user knows which env var to fix.
	if !strings.Contains(ufe.Message, "EDITOR") {
		t.Errorf("message should mention EDITOR env var; got %q", ufe.Message)
	}
	// Regression: the old cryptic "fork/exec" prefix MUST NOT appear.
	if strings.Contains(ufe.Message, "fork/exec") {
		t.Errorf("message should not include raw fork/exec text; got %q", ufe.Message)
	}
}
