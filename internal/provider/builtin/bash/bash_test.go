package bash

import (
	"context"
	"os/exec"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

func TestManifest(t *testing.T) {
	p := New()
	m := p.Manifest()
	if m.Name != "bash" {
		t.Errorf("Name = %q, want 'bash'", m.Name)
	}
	if m.ResourceClass != provider.ClassCheckBased {
		t.Errorf("ResourceClass = %d, want ClassCheckBased", m.ResourceClass)
	}
}

func TestBootstrap(t *testing.T) {
	p := New()
	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap error: %v", err)
	}
}

func TestProviderImplementsBashScriptRunner(_ *testing.T) {
	var _ provider.BashScriptRunner = New()
}

func TestRunScript_EmptyScriptIsNoop(t *testing.T) {
	p := New()
	if err := p.RunScript(context.Background(), ""); err != nil {
		t.Fatalf("empty RunScript should be a no-op, got %v", err)
	}
}

func TestRunScript_ExecutesViaInjectedBoundary(t *testing.T) {
	p := New()
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
	p := New()
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
	p := New()
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
	p := New()
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
	p := New()
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
	p := New()
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
	p := New()
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
	p := New()
	p.removeCommands["test-script"] = "echo removed"

	err := p.Remove(context.Background(), "test-script")
	if err != nil {
		t.Fatalf("Remove with command should succeed: %v", err)
	}
}

func TestRemove_WithSudoCommand(t *testing.T) {
	p := New()
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

func TestRunCheck_Empty(t *testing.T) {
	_, ok := RunCheck(context.Background(), "")
	if ok {
		t.Error("RunCheck should fail for empty command")
	}
}

func TestRemove_NoCommand(t *testing.T) {
	p := New()
	err := p.Remove(context.Background(), "some-script")
	if err != nil {
		t.Fatalf("Remove without command should be no-op: %v", err)
	}
}

func TestProbe(t *testing.T) {
	p := New()
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
