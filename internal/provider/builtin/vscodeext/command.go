package vscodeext

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// CmdRunner is the DI seam for every outbound invocation of `code`.
// vscodeext's surface is small (list, install, uninstall, PATH-check)
// — same shape as cargo. Production wires realCmdRunner; unit tests
// inject FakeCmdRunner.
type CmdRunner interface {
	// List runs `code --list-extensions --show-versions` and returns
	// raw stdout for parseExtensionList to consume.
	List(ctx context.Context) (string, error)

	// Install runs `code --install-extension <id>`.
	Install(ctx context.Context, id string) error

	// Uninstall runs `code --uninstall-extension <id>`.
	Uninstall(ctx context.Context, id string) error

	// LookPath verifies `code` is on $PATH.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) List(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "code", "--list-extensions", "--show-versions")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("code --list-extensions: %w", err)
	}
	return string(output), nil
}

func (r *realCmdRunner) Install(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, "code", "--install-extension", id) //nolint:gosec // id sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("code --install-extension %s: %w", id, err)
	}
	return nil
}

func (r *realCmdRunner) Uninstall(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, "code", "--uninstall-extension", id) //nolint:gosec // id sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("code --uninstall-extension %s: %w", id, err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := exec.LookPath("code"); err != nil {
		return fmt.Errorf("code CLI not found in PATH; ensure VS Code is installed and 'code' is on PATH")
	}
	return nil
}
