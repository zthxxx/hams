package cargo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CmdRunner is the DI seam for every outbound invocation of cargo.
// Production wires realCmdRunner (which shells out to `cargo`); unit
// tests inject FakeCmdRunner that records calls and maintains a
// virtual installed-crate set. The seam keeps cargo-provider tests
// host-safe — they never invoke real cargo.
type CmdRunner interface {
	// List runs `cargo install --list` and returns its raw stdout.
	// The provider's Probe parses this through parseCargoList.
	List(ctx context.Context) (string, error)

	// Install runs `cargo install <crate>`. crate is forwarded verbatim
	// so version pin syntax like `crate@<version>` reaches cargo.
	Install(ctx context.Context, crate string) error

	// Uninstall runs `cargo uninstall <crate>`.
	Uninstall(ctx context.Context, crate string) error

	// LookPath verifies cargo is on $PATH. Returns nil when present,
	// non-nil with an actionable error message otherwise.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner that shells out to
// the real cargo binary. Output streams to the host's terminal so the
// user sees download progress and error context.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) List(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "cargo", "install", "--list")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cargo install --list: %w", err)
	}
	return string(output), nil
}

func (r *realCmdRunner) Install(ctx context.Context, crate string) error {
	cmd := exec.CommandContext(ctx, "cargo", "install", crate) //nolint:gosec // crate sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cargo install %s: %w", strings.TrimSpace(crate), err)
	}
	return nil
}

func (r *realCmdRunner) Uninstall(ctx context.Context, crate string) error {
	cmd := exec.CommandContext(ctx, "cargo", "uninstall", crate) //nolint:gosec // crate sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cargo uninstall %s: %w", strings.TrimSpace(crate), err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := exec.LookPath("cargo"); err != nil {
		return fmt.Errorf("cargo not found in PATH")
	}
	return nil
}
