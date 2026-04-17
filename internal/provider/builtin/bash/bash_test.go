package bash

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

func TestManifest(t *testing.T) {
	p := New(nil)
	m := p.Manifest()
	if m.Name != "bash" {
		t.Errorf("Name = %q, want 'bash'", m.Name)
	}
	if m.ResourceClass != provider.ClassCheckBased {
		t.Errorf("ResourceClass = %d, want ClassCheckBased", m.ResourceClass)
	}
}

func TestBootstrap(t *testing.T) {
	p := New(nil)
	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap error: %v", err)
	}
}

func TestProviderImplementsBashScriptRunner(_ *testing.T) {
	var _ provider.BashScriptRunner = New(nil)
}

func TestRunScript_EmptyScriptIsNoop(t *testing.T) {
	p := New(nil)
	if err := p.RunScript(context.Background(), ""); err != nil {
		t.Fatalf("empty RunScript should be a no-op, got %v", err)
	}
}

func TestRunScript_ExecutesViaInjectedBoundary(t *testing.T) {
	p := New(nil)
	original := bootstrapExecCommand
	defer func() { bootstrapExecCommand = original }()

	var gotName string
	var gotArgs []string
	bootstrapExecCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = args
		// Delegate to a harmless no-op; /bin/sh -c : exits 0 on both
		// macOS and Linux without requiring a specific binary path.
		return original(ctx, "/bin/sh", "-c", ":")
	}

	if err := p.RunScript(context.Background(), "echo hello"); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if gotName != "/bin/bash" {
		t.Errorf("expected /bin/bash, got %q", gotName)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "-c" || gotArgs[1] != "echo hello" {
		t.Errorf("expected ['-c', 'echo hello'], got %v", gotArgs)
	}
}

func TestRunScript_PropagatesExecFailure(t *testing.T) {
	p := New(nil)
	original := bootstrapExecCommand
	defer func() { bootstrapExecCommand = original }()

	bootstrapExecCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return original(ctx, "/bin/sh", "-c", "exit 42")
	}

	err := p.RunScript(context.Background(), "any")
	if err == nil {
		t.Fatalf("expected failure, got nil")
	}
}

func TestApply_SimpleCommand(t *testing.T) {
	p := New(nil)
	action := provider.Action{
		ID:       "test-echo",
		Type:     provider.ActionInstall,
		Resource: bashResource{Run: "echo hello"},
	}

	err := p.Apply(context.Background(), action)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
}

func TestApply_FailingCommand(t *testing.T) {
	p := New(nil)
	action := provider.Action{
		ID:       "test-fail",
		Type:     provider.ActionInstall,
		Resource: bashResource{Run: "exit 1"},
	}

	err := p.Apply(context.Background(), action)
	if err == nil {
		t.Fatal("expected error for failing command")
	}
}

func TestApply_EmptyResource(t *testing.T) {
	p := New(nil)
	action := provider.Action{
		ID:       "test-empty",
		Type:     provider.ActionInstall,
		Resource: bashResource{},
	}

	err := p.Apply(context.Background(), action)
	if err == nil {
		t.Fatal("expected error for empty resource")
	}
}

func TestApply_CheckPassesSkipsRun(t *testing.T) {
	p := New(nil)
	action := provider.Action{
		ID:   "test-check-pass",
		Type: provider.ActionInstall,
		Resource: bashResource{
			Run:   "exit 1",          // Would fail if actually run.
			Check: "echo already-ok", // Passes → run is skipped.
		},
	}

	err := p.Apply(context.Background(), action)
	if err != nil {
		t.Fatalf("Apply should skip run when check passes: %v", err)
	}
}

func TestApply_CheckFailsRunsCommand(t *testing.T) {
	p := New(nil)
	action := provider.Action{
		ID:   "test-check-fail",
		Type: provider.ActionInstall,
		Resource: bashResource{
			Run:   "echo running",
			Check: "exit 1", // Fails → run proceeds.
		},
	}

	err := p.Apply(context.Background(), action)
	if err != nil {
		t.Fatalf("Apply should proceed when check fails: %v", err)
	}
}

func TestApply_SudoPrefix(t *testing.T) {
	// We cannot actually run sudo in tests, but we can verify the command
	// construction via maybeAddSudo.
	cmd := maybeAddSudo("echo hello", true)
	if cmd != "sudo echo hello" {
		t.Errorf("maybeAddSudo(true) = %q, want %q", cmd, "sudo echo hello")
	}

	cmd = maybeAddSudo("echo hello", false)
	if cmd != "echo hello" {
		t.Errorf("maybeAddSudo(false) = %q, want %q", cmd, "echo hello")
	}
}

func TestRemove_WithCommand(t *testing.T) {
	p := New(nil)
	p.removeCommands["test-script"] = "echo removed"

	err := p.Remove(context.Background(), "test-script")
	if err != nil {
		t.Fatalf("Remove with command should succeed: %v", err)
	}
}

func TestRemove_WithSudoCommand(t *testing.T) {
	p := New(nil)
	// Simulate what Plan does: store the already-prefixed command.
	p.removeCommands["test-sudo-script"] = "sudo echo removed"

	// We can't actually test sudo execution, but verify the command is stored.
	cmd, ok := p.removeCommands["test-sudo-script"]
	if !ok || cmd != "sudo echo removed" {
		t.Errorf("removeCommands should contain sudo-prefixed command, got %q", cmd)
	}
}

func TestRunCheck_Success(t *testing.T) {
	output, ok := RunCheck(context.Background(), "echo hello")
	if !ok {
		t.Error("RunCheck should succeed for 'echo hello'")
	}
	if output != "hello" {
		t.Errorf("output = %q, want 'hello'", output)
	}
}

func TestRunCheck_Failure(t *testing.T) {
	_, ok := RunCheck(context.Background(), "exit 1")
	if ok {
		t.Error("RunCheck should fail for 'exit 1'")
	}
}

// TestList_DelegatesToProviderDiff asserts the bash provider's
// List method is a thin wrapper around provider.DiffDesiredVsState
// + provider.FormatDiff (the standard diff path that cycle 148
// fixed for determinism). A regression that bypassed the diff
// machinery would silently flap output OR omit the "+ not installed"
// markers — both already caught upstream, but a wrapper-level
// regression test makes the dependency explicit.
func TestList_DelegatesToProviderDiff(t *testing.T) {
	p := New(nil)
	yamlDoc := `
install:
  - urn: urn:hams:bash:zsh-setup
    run: "echo zsh"
  - urn: urn:hams:bash:vim-setup
    run: "echo vim"
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}
	sf := state.New("bash", "test")

	out, err := p.List(context.Background(), hf, sf)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Both URNs are in desired but not in state → both appear as "+ not installed".
	if !strings.Contains(out, "urn:hams:bash:zsh-setup") {
		t.Errorf("output should contain zsh-setup, got:\n%s", out)
	}
	if !strings.Contains(out, "urn:hams:bash:vim-setup") {
		t.Errorf("output should contain vim-setup, got:\n%s", out)
	}
	if !strings.Contains(out, "(not installed)") {
		t.Errorf("output should contain '(not installed)' marker, got:\n%s", out)
	}
}

// TestRunCheck_HonorsContext locks in cycle 160: RunCheck previously
// used bitfield/script which doesn't honor context cancellation, so
// a hanging check command (e.g. `sleep 30`) kept running after the
// caller's context was canceled. Now: switched to exec.CommandContext
// so SIGINT/SIGTERM aborts the check promptly.
//
// Test pre-cancels the context, runs `sleep 30` as a check, asserts
// the call returns much faster than the 30s sleep would take if the
// process weren't being killed.
func TestRunCheck_HonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel: exec.CommandContext kills the process before/during start

	start := time.Now()
	_, ok := RunCheck(ctx, "sleep 30")
	elapsed := time.Since(start)

	if ok {
		t.Errorf("canceled check should NOT report ok=true")
	}
	// Even with start-up overhead, 1s is way under the 30s sleep.
	if elapsed > 1*time.Second {
		t.Errorf("RunCheck with canceled ctx took %v; want < 1s (context not honored)", elapsed)
	}
}

// TestRunBash_HonorsContext: same as TestRunCheck_HonorsContext but
// for runBash, which is the entry point bash.Apply uses for
// `run:` commands in the hamsfile.
func TestRunBash_HonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := runBash(ctx, "sleep 30")
	elapsed := time.Since(start)

	if err == nil {
		t.Errorf("canceled runBash should return non-nil error")
	}
	if elapsed > 1*time.Second {
		t.Errorf("runBash with canceled ctx took %v; want < 1s (context not honored)", elapsed)
	}
}

func TestRunCheck_Empty(t *testing.T) {
	_, ok := RunCheck(context.Background(), "")
	if ok {
		t.Error("RunCheck should fail for empty command")
	}
}

func TestRemove_NoCommand(t *testing.T) {
	p := New(nil)
	err := p.Remove(context.Background(), "some-script")
	if err != nil {
		t.Fatalf("Remove without command should be no-op: %v", err)
	}
}

// TestBashParseResources_WarnsOnDuplicateURN locks in cycle 193:
// a hamsfile with two entries under the same `urn:` value loses the
// FIRST entry silently — ComputePlan's first-occurrence-wins dedup
// (cycle 111) and bashParseResources's last-wins storage disagree.
// Apply ends up running the LAST entry's `run` command while the
// preview output iterates via ListApps (first-occurrence). The user
// thinks their FIRST script ran; actually it was the second one.
func TestBashParseResources_WarnsOnDuplicateURN(t *testing.T) {
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()
	defer slog.SetDefault(slog.Default())
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))

	yamlDoc := `
install:
  - urn: urn:hams:bash:init
    run: "echo first"
  - urn: urn:hams:bash:init
    run: "echo second"
`
	var root yaml.Node
	if uErr := yaml.Unmarshal([]byte(yamlDoc), &root); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	_, parseErr := bashParseResources(hf)
	if parseErr != nil {
		t.Fatalf("bashParseResources: %v", parseErr)
	}

	if cerr := w.Close(); cerr != nil {
		t.Logf("close pipe: %v", cerr)
	}
	buf, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	if cerr := r.Close(); cerr != nil {
		t.Logf("close reader: %v", cerr)
	}

	stderr := string(buf)
	if !strings.Contains(stderr, "duplicate urn") {
		t.Errorf("expected duplicate-urn warning; stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "urn:hams:bash:init") {
		t.Errorf("warning should mention the duplicate URN; stderr=%q", stderr)
	}
}

// TestBashParseResources_WarnsOnMissingURN locks in cycle 180:
// hamsfile entries with `run:` but no `urn:` are common user typos
// (forgot the URN line). Pre-cycle-180 they were silently dropped:
// ListApps skipped them, bashParseResources skipped them, the script
// never ran, and the user had no clue why. Now: emit a slog.Warn so
// the user sees their typo when they run with debug or check logs.
func TestBashParseResources_WarnsOnMissingURN(t *testing.T) {
	// Capture slog output via stderr redirection.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	// Force slog to write to the new stderr.
	defer slog.SetDefault(slog.Default())
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))

	yamlDoc := `
install:
  - run: "echo hello"
  - urn: urn:hams:bash:legitimate
    run: "echo legit"
`
	var root yaml.Node
	if uErr := yaml.Unmarshal([]byte(yamlDoc), &root); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	resourceByID, parseErr := bashParseResources(hf)
	if parseErr != nil {
		t.Fatalf("bashParseResources: %v", parseErr)
	}

	if cerr := w.Close(); cerr != nil {
		t.Logf("close pipe writer: %v", cerr)
	}
	stderrBuf, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	if cerr := r.Close(); cerr != nil {
		t.Logf("close pipe reader: %v", cerr)
	}

	// Only the legitimate entry should be in the map.
	if _, ok := resourceByID["urn:hams:bash:legitimate"]; !ok {
		t.Errorf("legitimate entry should be parsed; got map %v", resourceByID)
	}

	stderr := string(stderrBuf)
	if !strings.Contains(stderr, "no urn") {
		t.Errorf("expected warning about missing urn; stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "echo hello") {
		t.Errorf("warning should include the run command; stderr=%q", stderr)
	}
}

// TestPlan_ParsesAndEnrichesActions drives bashParseResources → Plan
// end to end: a hamsfile with two bash URNs (one with check+sudo, one
// plain) should produce two Install actions, each enriched with the
// parsed bashResource. Regression for a previously 0% branch in the
// bash provider (bashParseResources + Plan).
func TestPlan_ParsesAndEnrichesActions(t *testing.T) {
	yamlDoc := `
install:
  - urn: urn:hams:bash:zsh-setup
    run: "echo zsh"
    check: "test -f ~/.zshrc"
    sudo: false
  - urn: urn:hams:bash:dev-tools
    run: "echo tools"
    remove: "echo rm-tools"
    sudo: true
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlDoc), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hf := &hamsfile.File{Path: "test.yaml", Root: &root}

	p := New(nil)
	observed := state.New("bash", "test")
	actions, err := p.Plan(context.Background(), hf, observed)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want 2 (one per urn)", len(actions))
	}

	// Find action by ID (map iteration in ComputePlan is non-deterministic).
	byID := map[string]provider.Action{}
	for _, a := range actions {
		byID[a.ID] = a
	}

	zsh, ok := byID["urn:hams:bash:zsh-setup"]
	if !ok {
		t.Fatalf("zsh-setup action missing; got %v", byID)
	}
	res, ok := zsh.Resource.(bashResource)
	if !ok {
		t.Fatalf("zsh-setup Resource is %T, want bashResource", zsh.Resource)
	}
	if res.Run != "echo zsh" || res.Check != "test -f ~/.zshrc" || res.Sudo {
		t.Errorf("zsh-setup resource = %+v, want {Run:echo zsh Check:test -f ~/.zshrc Sudo:false}", res)
	}

	tools := byID["urn:hams:bash:dev-tools"]
	res = tools.Resource.(bashResource) //nolint:errcheck // test assertion, already verified type above
	if !res.Sudo || res.Remove != "echo rm-tools" {
		t.Errorf("dev-tools resource = %+v, want Sudo=true Remove='echo rm-tools'", res)
	}
	// sudo: true should prefix the remove command in the cache.
	if got := p.removeCommands["urn:hams:bash:dev-tools"]; got != "sudo echo rm-tools" {
		t.Errorf("removeCommands[dev-tools] = %q, want 'sudo echo rm-tools'", got)
	}
}

func TestProbe(t *testing.T) {
	p := New(nil)
	sf := state.New("bash", "test")
	sf.SetResource("init-zsh", state.StateOK)
	sf.SetResource("removed-script", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}

	// Should return only non-removed resources.
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].ID != "init-zsh" {
		t.Errorf("Probe result ID = %q, want 'init-zsh'", results[0].ID)
	}
}

// TestProbe_CheckCmdPassingCapturesStdout asserts the branch where
// the stored CheckCmd exits 0: state stays StateOK AND the stdout
// is captured into ProbeResult.Stdout so upstream diff machinery
// can distinguish "idempotent check ran, output matched" from
// "no check defined". Previously 0% coverage on this branch.
func TestProbe_CheckCmdPassingCapturesStdout(t *testing.T) {
	p := New(nil)
	sf := state.New("bash", "test")
	// A check that always passes (exit 0) and prints a stable line.
	sf.SetResource("check-passes", state.StateOK,
		state.WithCheckCmd("printf 'ok-line\\n'"))

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].State != state.StateOK {
		t.Errorf("state = %v, want StateOK", results[0].State)
	}
	if !strings.Contains(results[0].Stdout, "ok-line") {
		t.Errorf("stdout = %q, want to contain 'ok-line'", results[0].Stdout)
	}
}

// TestProbe_CheckCmdFailingFlagsPending asserts the branch where
// the stored CheckCmd exits non-zero: Probe flips the resource
// state to StatePending so the next apply's ComputePlan re-runs
// the Install action. This is the core drift-detection contract
// for bash provider — previously 0% coverage.
func TestProbe_CheckCmdFailingFlagsPending(t *testing.T) {
	p := New(nil)
	sf := state.New("bash", "test")
	// Explicit non-zero exit simulates a check that's drifted (the
	// feature it asserts is no longer configured on the host).
	sf.SetResource("check-fails", state.StateOK,
		state.WithCheckCmd("exit 1"))

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].State != state.StatePending {
		t.Errorf("state = %v, want StatePending (check failed → drift detected)", results[0].State)
	}
}
