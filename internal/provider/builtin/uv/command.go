package uv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// CmdRunner is the DI seam for every outbound invocation of uv.
// Production wires realCmdRunner; unit tests inject FakeCmdRunner.
type CmdRunner interface {
	// List runs `uv tool list` and returns its raw stdout.
	List(ctx context.Context) (string, error)

	// Install runs `uv tool install <tool>`.
	Install(ctx context.Context, tool string) error

	// Uninstall runs `uv tool uninstall <tool>`.
	Uninstall(ctx context.Context, tool string) error

	// LookPath verifies uv is on $PATH.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) List(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "uv", "tool", "list")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("uv tool list: %w", err)
	}
	return string(output), nil
}

func (r *realCmdRunner) Install(ctx context.Context, tool string) error {
	cmd := exec.CommandContext(ctx, "uv", "tool", "install", tool) //nolint:gosec // tool sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uv tool install %s: %w", tool, err)
	}
	return nil
}

func (r *realCmdRunner) Uninstall(ctx context.Context, tool string) error {
	cmd := exec.CommandContext(ctx, "uv", "tool", "uninstall", tool) //nolint:gosec // tool sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uv tool uninstall %s: %w", tool, err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := exec.LookPath("uv"); err != nil {
		return fmt.Errorf("uv not found in PATH")
	}
	return nil
}
