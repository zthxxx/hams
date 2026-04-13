package runner_test

import (
	"context"
	"errors"
	"testing"

	"pgregory.net/rapid"

	"github.com/zthxxx/hams/internal/runner"
)

// MockRunner is a test double that records calls and returns configured responses.
type MockRunner struct {
	RunCalls         []MockCall
	PassthroughCalls []MockCall
	LookPathCalls    []string
	RunOutput        []byte
	RunErr           error
	PassthroughErr   error
	LookPathResult   string
	LookPathErr      error
}

// MockCall records a single command invocation.
type MockCall struct {
	Name string
	Args []string
}

func (m *MockRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.RunCalls = append(m.RunCalls, MockCall{Name: name, Args: args})
	return m.RunOutput, m.RunErr
}

func (m *MockRunner) RunPassthrough(_ context.Context, name string, args ...string) error {
	m.PassthroughCalls = append(m.PassthroughCalls, MockCall{Name: name, Args: args})
	return m.PassthroughErr
}

func (m *MockRunner) LookPath(name string) (string, error) {
	m.LookPathCalls = append(m.LookPathCalls, name)
	return m.LookPathResult, m.LookPathErr
}

func TestOSRunner_ImplementsRunner(t *testing.T) {
	var _ runner.Runner = (*runner.OSRunner)(nil)
	var _ runner.Runner = runner.DefaultRunner()
}

func TestMockRunner_ImplementsRunner(t *testing.T) {
	var _ runner.Runner = (*MockRunner)(nil)
}

func TestMockRunner_RecordsCalls(t *testing.T) {
	t.Parallel()
	mock := &MockRunner{
		RunOutput: []byte("hello"),
	}

	out, err := mock.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "hello" {
		t.Errorf("output = %q, want %q", out, "hello")
	}
	if len(mock.RunCalls) != 1 {
		t.Fatalf("RunCalls = %d, want 1", len(mock.RunCalls))
	}
	if mock.RunCalls[0].Name != "echo" {
		t.Errorf("RunCalls[0].Name = %q, want %q", mock.RunCalls[0].Name, "echo")
	}
}

func TestMockRunner_ReturnsErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("command failed")
	mock := &MockRunner{RunErr: wantErr}

	_, err := mock.Run(context.Background(), "fail")
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
}

func TestMockRunner_LookPathRecording(t *testing.T) {
	t.Parallel()
	mock := &MockRunner{
		LookPathResult: "/usr/bin/brew",
	}

	path, err := mock.LookPath("brew")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/usr/bin/brew" {
		t.Errorf("path = %q, want %q", path, "/usr/bin/brew")
	}
	if len(mock.LookPathCalls) != 1 || mock.LookPathCalls[0] != "brew" {
		t.Errorf("LookPathCalls = %v, want [brew]", mock.LookPathCalls)
	}
}

func TestMockRunner_Property_RunPreservesArgs(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "name")
		nArgs := rapid.IntRange(0, 5).Draw(t, "nArgs")
		args := make([]string, nArgs)
		for i := range args {
			args[i] = rapid.StringMatching(`[a-z0-9]{1,10}`).Draw(t, "arg")
		}

		mock := &MockRunner{}
		_, _ = mock.Run(context.Background(), name, args...)

		if len(mock.RunCalls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.RunCalls))
		}
		call := mock.RunCalls[0]
		if call.Name != name {
			t.Errorf("name = %q, want %q", call.Name, name)
		}
		if len(call.Args) != nArgs {
			t.Errorf("args len = %d, want %d", len(call.Args), nArgs)
		}
	})
}

// OSRunner tests that execute real commands are in runner_integration_test.go
// with the "integration" build tag to avoid host-side effects in unit tests.
