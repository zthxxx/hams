// Package ansible wraps ansible-playbook for complex orchestration via playbooks.
package ansible

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// Provider implements the Ansible provider.
type Provider struct{}

// New creates a new Ansible provider.
func New() *Provider { return &Provider{} }

// Manifest returns the Ansible provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "ansible",
		DisplayName:   "Ansible",
		Platform:      provider.PlatformAll,
		ResourceClass: provider.ClassCheckBased,
		FilePrefix:    "ansible",
	}
}

// Bootstrap checks if ansible-playbook is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		return fmt.Errorf("ansible-playbook not found in PATH; install via: pip install ansible")
	}
	return nil
}

// Probe checks playbook status from state.
func (p *Provider) Probe(_ context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		results = append(results, provider.ProbeResult{ID: id, State: r.State})
	}
	return results, nil
}

// Plan computes actions for ansible playbooks.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply runs an ansible playbook.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	playbookPath, ok := action.Resource.(string)
	if !ok || playbookPath == "" {
		return fmt.Errorf("ansible: resource must be a playbook path")
	}

	slog.Info("ansible-playbook", "playbook", playbookPath)
	cmd := exec.CommandContext(ctx, "ansible-playbook", playbookPath) //nolint:gosec // playbook path from hamsfile
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Remove is a no-op for ansible — playbooks don't have uninstall.
func (p *Provider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("ansible: remove is a no-op for playbooks", "resource", resourceID)
	return nil
}

// List returns playbook entries with status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-50s %s\n", id, r.State)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for ansible.
func (p *Provider) HandleCommand(args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"ansible requires a playbook path",
			"Usage: hams ansible <playbook.yml>",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: ansible-playbook %s\n", strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(context.Background(), "ansible-playbook", args...) //nolint:gosec // ansible args from CLI
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "ansible" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "Ansible" }
