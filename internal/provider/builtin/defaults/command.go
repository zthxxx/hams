package defaults

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CmdRunner is the DI seam for every outbound invocation of the macOS
// `defaults` CLI. The four operations match the four `defaults` verbs
// the provider uses (read, write, delete, plus PATH check). Production
// wires realCmdRunner; unit tests inject FakeCmdRunner.
type CmdRunner interface {
	// Read runs `defaults read <domain> <key>` and returns trimmed
	// stdout (the current value).
	Read(ctx context.Context, domain, key string) (string, error)

	// Write runs `defaults write <domain> <key> -<typeStr> <value>`.
	Write(ctx context.Context, domain, key, typeStr, value string) error

	// Delete runs `defaults delete <domain> <key>`.
	Delete(ctx context.Context, domain, key string) error

	// LookPath verifies `defaults` is on $PATH (always present on
	// macOS; a missing binary signals "not on a Mac").
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) Read(ctx context.Context, domain, key string) (string, error) {
	cmd := exec.CommandContext(ctx, cliName, "read", domain, key) //nolint:gosec // domain/key from state entries
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("defaults read %s %s: %w", domain, key, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (r *realCmdRunner) Write(ctx context.Context, domain, key, typeStr, value string) error {
	cmd := exec.CommandContext(ctx, cliName, "write", domain, key, "-"+typeStr, value) //nolint:gosec // args sourced from hamsfile declarations
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("defaults write %s %s -%s %s: %w", domain, key, typeStr, value, err)
	}
	return nil
}

func (r *realCmdRunner) Delete(ctx context.Context, domain, key string) error {
	cmd := exec.CommandContext(ctx, cliName, "delete", domain, key) //nolint:gosec // domain/key from hamsfile/state entries
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("defaults delete %s %s: %w", domain, key, err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := exec.LookPath(cliName); err != nil {
		return fmt.Errorf("defaults not found in PATH (macOS only)")
	}
	return nil
}
