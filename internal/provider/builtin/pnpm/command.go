package pnpm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// CmdRunner is the DI seam for every outbound invocation of pnpm.
// Production wires realCmdRunner; unit tests inject FakeCmdRunner.
// The seam keeps pnpm-provider tests host-safe — they never invoke
// real pnpm.
type CmdRunner interface {
	// List runs `pnpm list -g --json` and returns its raw stdout.
	List(ctx context.Context) (string, error)

	// Install runs `pnpm add -g <pkg>`. pnpm uses `add`, not
	// `install`, for the global-add verb.
	Install(ctx context.Context, pkg string) error

	// Uninstall runs `pnpm remove -g <pkg>`. pnpm uses `remove`, not
	// `uninstall`.
	Uninstall(ctx context.Context, pkg string) error

	// LookPath verifies pnpm is on $PATH; returns nil when present
	// and exec.ErrNotFound (or similar) when absent. Bootstrap wraps
	// the error result into a BootstrapRequiredError.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner that shells out
// to the real pnpm binary.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) List(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "pnpm", "list", "-g", "--json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pnpm list: %w", err)
	}
	return string(output), nil
}

func (r *realCmdRunner) Install(ctx context.Context, pkg string) error {
	cmd := exec.CommandContext(ctx, "pnpm", "add", "-g", pkg) //nolint:gosec // pkg sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pnpm add -g %s: %w", pkg, err)
	}
	return nil
}

func (r *realCmdRunner) Uninstall(ctx context.Context, pkg string) error {
	cmd := exec.CommandContext(ctx, "pnpm", "remove", "-g", pkg) //nolint:gosec // pkg sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pnpm remove -g %s: %w", pkg, err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := pnpmBinaryLookup("pnpm"); err != nil {
		return err
	}
	return nil
}
