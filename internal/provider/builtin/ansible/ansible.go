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
type Provider struct {
	runner CmdRunner
}

// New creates a new Ansible provider wired with a real CmdRunner.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(runner CmdRunner) *Provider { return &Provider{runner: runner} }

// ansibleInstallScript is the consent-gated install command. pipx is
// chosen over pip because PEP 668 flags system-pip installs on modern
// Python installations (Debian 12+, brew-python) with
// "externally-managed environment". pipx creates an isolated venv per
// app and is the Python community's accepted answer for installing
// tools from PyPI. Users without pipx see the chained prerequisite
// (`apt install pipx` on Debian / `brew install pipx` on macOS) in
// the error body's suggestions.
const ansibleInstallScript = "pipx install --include-deps ansible"

// ansibleBinaryLookup is the PATH-check seam Bootstrap uses.
var ansibleBinaryLookup = exec.LookPath

// Manifest returns the Ansible provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "ansible",
		DisplayName:   "Ansible",
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassCheckBased,
		DependsOn: []provider.DependOn{
			{Provider: "bash", Script: ansibleInstallScript},
		},
		FilePrefix: "ansible",
	}
}

// Bootstrap reports whether ansible-playbook is installed. A missing
// binary is signaled via provider.BootstrapRequiredError so the CLI
// consent flow can surface the pipx install script + --bootstrap
// remedy. If pipx itself is missing, the bash-provider RunScript
// invocation will fail with "pipx: command not found" and the
// surrounding bootstrap-failure path surfaces the chain.
func (p *Provider) Bootstrap(_ context.Context) error {
	if err := p.runner.LookPath(); err == nil {
		return nil
	}
	return &provider.BootstrapRequiredError{
		Provider: "ansible",
		Binary:   "ansible-playbook",
		Script:   ansibleInstallScript,
	}
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

// Plan computes actions for ansible playbooks. Each resource ID is
// the playbook path, attached as the action Resource. Hamsfile-
// declared hooks are attached to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	for i := range actions {
		actions[i].Resource = actions[i].ID
	}
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply runs an ansible playbook.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	playbookPath, ok := action.Resource.(string)
	if !ok || playbookPath == "" {
		return fmt.Errorf("ansible: resource must be a playbook path")
	}

	slog.Info("ansible-playbook", "playbook", playbookPath)
	return p.runner.RunPlaybook(ctx, playbookPath)
}

// Remove is a no-op for ansible — playbooks don't have uninstall.
func (p *Provider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("ansible: remove is a no-op for playbooks", "resource", resourceID)
	return nil
}

// List returns playbook entries with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for ansible.
func (p *Provider) HandleCommand(_ context.Context, args []string, _ map[string]string, flags *provider.GlobalFlags) error {
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
