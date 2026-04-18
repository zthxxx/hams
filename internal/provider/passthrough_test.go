package provider

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// TestPassthrough_InvokesExec asserts Passthrough forwards (tool, args)
// to PassthroughExec unchanged when DryRun is off.
func TestPassthrough_InvokesExec(t *testing.T) {
	t.Parallel()
	var gotTool string
	var gotArgs []string
	orig := PassthroughExec
	t.Cleanup(func() { PassthroughExec = orig })
	PassthroughExec = func(_ context.Context, tool string, args []string) error {
		gotTool = tool
		gotArgs = append([]string(nil), args...)
		return nil
	}

	if err := Passthrough(context.Background(), "git", []string{"log", "--oneline"}, nil); err != nil {
		t.Fatalf("Passthrough: %v", err)
	}
	if gotTool != "git" {
		t.Errorf("tool = %q, want git", gotTool)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "log" || gotArgs[1] != "--oneline" {
		t.Errorf("args = %v, want [log --oneline]", gotArgs)
	}
}

// TestPassthrough_PropagatesExecError asserts non-zero exits propagate.
func TestPassthrough_PropagatesExecError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("exit status 1")
	orig := PassthroughExec
	t.Cleanup(func() { PassthroughExec = orig })
	PassthroughExec = func(_ context.Context, _ string, _ []string) error {
		return wantErr
	}

	err := Passthrough(context.Background(), "brew", []string{"upgrade"}, nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// TestPassthrough_DryRunSkipsExec asserts DryRun prints the preview
// and does NOT invoke PassthroughExec.
func TestPassthrough_DryRunSkipsExec(t *testing.T) {
	t.Parallel()
	var called bool
	orig := PassthroughExec
	t.Cleanup(func() { PassthroughExec = orig })
	PassthroughExec = func(_ context.Context, _ string, _ []string) error {
		called = true
		return nil
	}

	var buf bytes.Buffer
	flags := &GlobalFlags{DryRun: true, Out: &buf}
	if err := Passthrough(context.Background(), "apt-get", []string{"install", "htop"}, flags); err != nil {
		t.Fatalf("Passthrough: %v", err)
	}
	if called {
		t.Error("PassthroughExec should NOT be invoked under DryRun")
	}
	got := buf.String()
	if !strings.Contains(got, "[dry-run]") {
		t.Errorf("output = %q, want dry-run prefix", got)
	}
	if !strings.Contains(got, "apt-get install htop") {
		t.Errorf("output = %q, want `apt-get install htop` preview", got)
	}
}

// TestPassthrough_DryRunZeroArgs covers the empty-args branch.
func TestPassthrough_DryRunZeroArgs(t *testing.T) {
	t.Parallel()
	orig := PassthroughExec
	t.Cleanup(func() { PassthroughExec = orig })
	PassthroughExec = func(_ context.Context, _ string, _ []string) error {
		t.Fatal("exec should not be called")
		return nil
	}

	var buf bytes.Buffer
	flags := &GlobalFlags{DryRun: true, Out: &buf}
	if err := Passthrough(context.Background(), "git", nil, flags); err != nil {
		t.Fatalf("Passthrough: %v", err)
	}
	if !strings.Contains(buf.String(), "Would run: git") {
		t.Errorf("output = %q, want `Would run: git`", buf.String())
	}
}
