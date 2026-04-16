package mas

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// CmdRunner is the DI seam for every outbound invocation of mas.
type CmdRunner interface {
	// List runs `mas list` and returns its raw stdout.
	List(ctx context.Context) (string, error)

	// Install runs `mas install <appID>` (numeric Mac App Store ID).
	Install(ctx context.Context, appID string) error

	// Uninstall runs `mas uninstall <appID>`.
	Uninstall(ctx context.Context, appID string) error

	// LookPath verifies mas is on $PATH; Bootstrap wraps the err into
	// a BootstrapRequiredError when missing.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) List(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, cliName, "list")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mas list: %w", err)
	}
	return string(output), nil
}

func (r *realCmdRunner) Install(ctx context.Context, appID string) error {
	cmd := exec.CommandContext(ctx, cliName, "install", appID) //nolint:gosec // appID sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mas install %s: %w", appID, err)
	}
	return nil
}

func (r *realCmdRunner) Uninstall(ctx context.Context, appID string) error {
	cmd := exec.CommandContext(ctx, cliName, "uninstall", appID) //nolint:gosec // appID sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mas uninstall %s: %w", appID, err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := masBinaryLookup(cliName); err != nil {
		return err
	}
	return nil
}
