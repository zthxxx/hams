package ansible

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// CmdRunner is the DI seam for every outbound invocation of
// ansible-playbook. Ansible's surface is small (run a playbook,
// check PATH); the interface mirrors that.
type CmdRunner interface {
	// RunPlaybook executes `ansible-playbook <path>`.
	RunPlaybook(ctx context.Context, path string) error

	// LookPath verifies ansible-playbook is on $PATH; Bootstrap
	// wraps the err into a BootstrapRequiredError when missing.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner that shells out
// to the real ansible-playbook binary.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) RunPlaybook(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "ansible-playbook", path) //nolint:gosec // path from hamsfile entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible-playbook %s: %w", path, err)
	}
	return nil
}

func (r *realCmdRunner) LookPath() error {
	if _, err := ansibleBinaryLookup("ansible-playbook"); err != nil {
		return err
	}
	return nil
}
