package npm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// CmdRunner is the DI seam for every outbound invocation of npm.
// Production wires realCmdRunner (which shells out to `npm`); unit
// tests inject FakeCmdRunner. The seam keeps npm-provider tests
// host-safe — they never invoke real npm.
type CmdRunner interface {
	// List runs `npm list -g --json --depth=0` and returns its raw
	// stdout (a JSON document parsed by parseNpmList).
	List(ctx context.Context) (string, error)

	// Install runs `npm install -g <pkg>`.
	Install(ctx context.Context, pkg string) error

	// Uninstall runs `npm uninstall -g <pkg>`.
	Uninstall(ctx context.Context, pkg string) error

	// LookPath verifies npm is on $PATH.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner that shells out
// to the real npm binary.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) List(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "npm", "list", "-g", "--json", "--depth=0")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("npm list: %w", err)
	}
	return string(output), nil
}

func (r *realCmdRunner) Install(ctx context.Context, pkg string) error {
	cmd := exec.CommandContext(ctx, "npm", "install", "-g", pkg) //nolint:gosec // pkg sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm install -g %s: %w", pkg, err)
	}
	return nil
}

func (r *realCmdRunner) Uninstall(ctx context.Context, pkg string) error {
	cmd := exec.CommandContext(ctx, "npm", "uninstall", "-g", pkg) //nolint:gosec // pkg sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm uninstall -g %s: %w", pkg, err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm not found in PATH")
	}
	return nil
}
