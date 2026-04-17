// Package ansible wraps ansible-playbook for complex orchestration via playbooks.
package ansible

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// Provider implements the Ansible provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new Ansible provider wired with a real CmdRunner.
// cfg supplies store/profile paths used by the `list` CLI subcommand
// to locate the hamsfile and state file. Apply-from-hamsfile does not
// read cfg — it goes through CmdRunner alone.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider { return &Provider{cfg: cfg, runner: runner} }

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
//
// Per builtin-providers.md §"Ansible Provider" the spec-mandated
// subcommands are:
//
//   - `hams ansible run <urn-id>` — run a single hamsfile-tracked playbook.
//   - `hams ansible list` — show tracked playbooks with status.
//   - `hams ansible remove <urn-id>` — delete from hamsfile (no rollback).
//
// Cycle 213 wires `list` end-to-end via the same diff helper the
// top-level `hams list --only=ansible` uses. `run` and `remove`
// require hamsfile-edit support that v1 has not yet shipped — they
// return an ExitUsageError pointing the user at `hams apply --only=ansible`
// (which reads the same URN-shaped entries via Plan) or hand-editing
// the hamsfile.
//
// For backward compatibility with the pre-cycle-213 ad-hoc passthrough
// (`hams ansible <playbook.yml>` exec'd ansible-playbook directly), any
// first-arg that isn't one of the three verbs still falls through to
// the exec path. Users relying on ad-hoc invocations keep working.
func (p *Provider) HandleCommand(ctx context.Context, args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"ansible requires a playbook path or subcommand",
			"Usage: hams ansible list",
			"       hams ansible <playbook.yml>            (ad-hoc passthrough)",
			"       hams ansible run <urn-id>              (planned v1.1)",
			"       hams ansible remove <urn-id>           (planned v1.1)",
		)
	}

	switch args[0] {
	case "list":
		return p.handleList(flags)
	case "run", "remove":
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("ansible %s is planned for v1.1 (URN-editing on the CLI is not yet wired)", args[0]),
			"Use 'hams apply --only=ansible' to run all tracked playbooks",
			"Or hand-edit the ansible hamsfile: <profile-dir>/ansible.hams.yaml",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: ansible-playbook %s\n", strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(ctx, "ansible-playbook", args...) //nolint:gosec // ansible args from CLI
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// handleList loads the hamsfile + state for ansible and prints the
// DiffDesiredVsState output. Mirrors what `hams list --only=ansible`
// prints for a user who went directly to the provider subcommand
// instead of the top-level list. JSON mode unsupported here — the
// top-level `hams --json list --only=ansible` is the machine-parseable
// entrypoint.
func (p *Provider) handleList(flags *provider.GlobalFlags) error {
	cfg := p.effectiveConfig(flags)
	if cfg.StorePath == "" {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"no store directory configured",
			"Set store_path in hams config or pass --store",
		)
	}

	hfPath := filepath.Join(cfg.ProfileDir(), "ansible.hams.yaml")
	hf, err := hamsfile.LoadOrCreateEmpty(hfPath)
	if err != nil {
		return fmt.Errorf("loading ansible hamsfile: %w", err)
	}

	statePath := filepath.Join(cfg.StateDir(), "ansible.state.yaml")
	sf, err := state.Load(statePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("loading ansible state: %w", err)
		}
		sf = state.New(p.Name(), cfg.MachineID)
	}

	output, listErr := p.List(context.Background(), hf, sf)
	if listErr != nil {
		return listErr
	}
	fmt.Print(output)
	return nil
}

// effectiveConfig returns the provider's config overlaid with any
// per-invocation flags (--store, --profile). Mirrors the helper used
// by other providers' CLI paths.
func (p *Provider) effectiveConfig(flags *provider.GlobalFlags) *config.Config {
	if p.cfg == nil {
		p.cfg = &config.Config{}
	}
	cfg := *p.cfg
	if flags == nil {
		return &cfg
	}
	if flags.Store != "" {
		cfg.StorePath = flags.Store
	}
	if flags.Profile != "" {
		cfg.ProfileTag = flags.Profile
	}
	return &cfg
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "ansible" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "Ansible" }
