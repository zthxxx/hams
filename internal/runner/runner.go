// Package runner defines the external command execution boundary interface for dependency injection.
// All providers and subsystems that execute external commands should accept a Runner interface
// rather than calling exec.Command directly, enabling isolated unit testing via mock runners.
package runner

import (
	"context"
	"os"
	"os/exec"
)

// Runner abstracts external command execution for dependency injection.
// Implementations can wrap real exec.Command calls or provide mock behavior for testing.
type Runner interface {
	// Run executes a command and returns its combined output.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)

	// RunPassthrough executes a command with stdin/stdout/stderr connected to the terminal.
	RunPassthrough(ctx context.Context, name string, args ...string) error

	// LookPath checks if an executable exists in PATH.
	LookPath(name string) (string, error)
}

// OSRunner is the default Runner that delegates to os/exec.
type OSRunner struct{}

// Run executes a command and returns its output.
func (r *OSRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // args are provider-controlled
	return cmd.CombinedOutput()
}

// RunPassthrough executes a command with terminal I/O connected.
func (r *OSRunner) RunPassthrough(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // args are provider-controlled
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LookPath searches for an executable in PATH.
func (r *OSRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// DefaultRunner returns the standard OS command runner.
func DefaultRunner() Runner {
	return &OSRunner{}
}
