package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CmdRunner is the DI seam for every outbound invocation of the git
// CLI. Production wires a real implementation that shells out to the
// host's git binary; unit tests inject a fake that records calls and
// manipulates an in-memory KV store. The seam keeps git-config
// provider tests host-safe — they never touch the developer's real
// `~/.gitconfig`.
type CmdRunner interface {
	// SetGlobal runs `git config --global <key> <value>`. Used by both
	// the Apply path (declarative apply) and the HandleCommand path
	// (CLI auto-record).
	SetGlobal(ctx context.Context, key, value string) error

	// UnsetGlobal runs `git config --global --unset <key>`. Used by the
	// Remove path.
	UnsetGlobal(ctx context.Context, key string) error

	// GetGlobal runs `git config --global --get <key>` and returns the
	// trimmed value. Non-zero exit surfaces as an error so the Probe
	// path can record StateFailed.
	GetGlobal(ctx context.Context, key string) (string, error)
}

// NewRealCmdRunner returns the production CmdRunner backed by the
// host's git binary.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) SetGlobal(ctx context.Context, key, value string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", key, value) //nolint:gosec // args come from hamsfile/CLI declarations
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git config --global %s: %w", key, err)
	}
	return nil
}

func (r *realCmdRunner) UnsetGlobal(ctx context.Context, key string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--unset", key) //nolint:gosec // key comes from hamsfile/CLI declarations
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git config --global --unset %s: %w", key, err)
	}
	return nil
}

func (r *realCmdRunner) GetGlobal(ctx context.Context, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--get", key) //nolint:gosec // key comes from state entries
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git config --get %s: %w", key, err)
	}
	return strings.TrimSpace(string(output)), nil
}
