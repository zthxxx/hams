package duti

import (
	"context"
	"fmt"
	"os/exec"
)

// CmdRunner is the DI seam for every outbound invocation of duti.
// duti's resource model is "<ext>=<bundle-id>" (file extension to
// macOS application bundle ID). Production wires realCmdRunner; unit
// tests inject FakeCmdRunner.
type CmdRunner interface {
	// QueryDefault runs `duti -x <ext>` to get the current default
	// app for the given file extension. Returns the raw stdout (the
	// caller passes through parseDutiOutput).
	QueryDefault(ctx context.Context, ext string) (string, error)

	// SetDefault runs `duti -s <bundleID> .<ext> all` to bind the
	// extension to the given bundle ID for all roles (viewer, editor).
	SetDefault(ctx context.Context, ext, bundleID string) error

	// LookPath verifies duti is on $PATH; Bootstrap wraps the err
	// into a BootstrapRequiredError when missing.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) QueryDefault(ctx context.Context, ext string) (string, error) {
	cmd := exec.CommandContext(ctx, "duti", "-x", ext) //nolint:gosec // ext from tracked state entries
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("duti -x %s: %w", ext, err)
	}
	return string(output), nil
}

func (r *realCmdRunner) SetDefault(ctx context.Context, ext, bundleID string) error {
	cmd := exec.CommandContext(ctx, "duti", "-s", bundleID, "."+ext, "all") //nolint:gosec // duti args from hamsfile declarations
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("duti -s %s .%s all: %w", bundleID, ext, err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := dutiBinaryLookup("duti"); err != nil {
		return err
	}
	return nil
}
