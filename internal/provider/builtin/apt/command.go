package apt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zthxxx/hams/internal/sudo"
)

// CmdRunner is the DI seam for every outbound invocation of apt-get and dpkg.
// Production wires a real implementation composed with sudo.CmdBuilder; unit
// tests inject a fake that records calls and manipulates an in-memory
// "installed packages" set. The seam keeps apt-provider tests host-safe —
// they never shell out to apt-get or dpkg.
type CmdRunner interface {
	// Install runs `sudo apt-get install -y <pkg>`, streaming stdout/stderr
	// to the user's terminal.
	Install(ctx context.Context, pkg string) error

	// Remove runs `sudo apt-get remove -y <pkg>`, streaming stdout/stderr.
	Remove(ctx context.Context, pkg string) error

	// IsInstalled probes `dpkg -s <pkg>`. Returns (true, version, nil) when
	// the package is present with `Status: install ok installed`; (false, "",
	// nil) when absent (dpkg exits non-zero); (false, "", err) for other
	// failures (e.g., missing dpkg binary).
	IsInstalled(ctx context.Context, pkg string) (installed bool, version string, err error)
}

// NewRealCmdRunner returns the production CmdRunner backed by a sudo builder.
// Commands stream stdout/stderr to the host's terminal.
func NewRealCmdRunner(sb sudo.CmdBuilder) CmdRunner {
	return &realCmdRunner{sudo: sb}
}

type realCmdRunner struct {
	sudo sudo.CmdBuilder
}

func (r *realCmdRunner) Install(ctx context.Context, pkg string) error {
	cmd := r.sudo.Command(ctx, "apt-get", "install", "-y", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apt-get install %s: %w", pkg, err)
	}
	return nil
}

func (r *realCmdRunner) Remove(ctx context.Context, pkg string) error {
	cmd := r.sudo.Command(ctx, "apt-get", "remove", "-y", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apt-get remove %s: %w", pkg, err)
	}
	return nil
}

func (r *realCmdRunner) IsInstalled(ctx context.Context, pkg string) (bool, string, error) {
	// dpkg -s does not require sudo.
	cmd := exec.CommandContext(ctx, "dpkg", "-s", pkg) //nolint:gosec // pkg sourced from state/config entries
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Non-zero exit = not installed (or not known to dpkg).
			return false, "", nil
		}
		return false, "", fmt.Errorf("dpkg -s %s: %w", pkg, err)
	}
	text := string(output)
	if !strings.Contains(text, "Status: install ok installed") {
		return false, "", nil
	}
	return true, parseDpkgVersion(text), nil
}
