// Package uv wraps the uv tool manager for Python tool installation.
package uv

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// Provider implements the uv tool provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new uv provider wired with a real CmdRunner.
// cfg supplies store/profile paths for the CLI-first auto-record path.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the uv provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "uv",
		DisplayName:   "uv",
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		FilePrefix:    "uv",
	}
}

// Bootstrap checks if uv is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	return p.runner.LookPath()
}

// Probe queries uv for installed tools.
// Probe queries uv for installed tools.
//
// Cycle 189: state IDs with `==version` pins (pip convention — uv
// uses pip's version-specifier syntax) strip the suffix before the
// installed-map lookup. Pre-cycle-189 a pinned state entry never
// matched — drift detection was broken for any user who pinned via
// CLI like `hams uv install foo==1.2.3`. Also strips the broader
// pip specifiers (`>=`, `<=`, `~=`, `>`, `<`) so uses consistent
// drift detection for those too.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := p.runner.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := parseUvToolList(output)
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		key := stripUvVersionPin(id)
		if ver, ok := installed[key]; ok {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: ver})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		}
	}
	return results, nil
}

// stripUvVersionPin strips the pip-style version specifier suffix
// from a uv tool ID. uv uses pip's syntax (`foo==1.2.3`, `foo>=1.0`,
// etc.) rather than npm's `@` delimiter. Returns the bare tool name.
func stripUvVersionPin(id string) string {
	for _, sep := range []string{"==", ">=", "<=", "~=", ">", "<"} {
		if idx := strings.Index(id, sep); idx > 0 {
			return id[:idx]
		}
	}
	return id
}

// suppressRedundantVersionRemoves drops ActionRemove entries whose
// bare tool name (pip-specifier-stripped) matches an Install/Update/
// Skip action and tombstones the stale state entry. Cycle 191/192 —
// same rationale as npm. uv uses pip-style specifiers so the strip
// function is stripUvVersionPin.
func suppressRedundantVersionRemoves(actions []provider.Action, observed *state.File) []provider.Action {
	keepBareNames := make(map[string]bool)
	for _, a := range actions {
		if a.Type == provider.ActionRemove {
			continue
		}
		keepBareNames[stripUvVersionPin(a.ID)] = true
	}
	out := make([]provider.Action, 0, len(actions))
	for _, a := range actions {
		if a.Type == provider.ActionRemove && keepBareNames[stripUvVersionPin(a.ID)] {
			slog.Info("uv: suppressing redundant version-pin remove (bare name overlaps install)",
				"removing", a.ID)
			if observed != nil {
				observed.SetResource(a.ID, state.StateRemoved)
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

// Plan computes actions for uv tools and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	// Cycle 191/192: drop redundant version-pinned removes + tombstone
	// the stale state entry (same rationale as npm).
	actions = suppressRedundantVersionRemoves(actions, observed)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply installs a uv tool.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("uv tool install", "package", action.ID)
	return p.runner.Install(ctx, action.ID)
}

// Remove uninstalls a uv tool.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("uv tool uninstall", "package", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns installed uv tools with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for uv.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove", "uninstall", "rm":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	case "list":
		// Cycle 214: route `hams uv list` to the hams-tracked diff.
		// `uv tool list` exists but only shows tools installed via
		// `uv tool`, not the hams-tracked diff against the hamsfile.
		return provider.HandleListCmd(ctx, p, p.effectiveConfig(flags))
	default:
		return provider.WrapExecPassthrough(ctx, "uv", args, nil)
	}
}

// handleInstall runs `uv tool install <tool>` via the CmdRunner seam
// and, on success, appends each tool to the uv hamsfile. All-or-
// nothing: any install failure aborts before the hamsfile is touched.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"uv install requires a tool name",
			"Usage: hams uv install <tool>",
			"To install all recorded tools, use: hams apply --only=uv",
		)
	}
	tools := toolArgs(args)
	if len(tools) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"uv install requires at least one tool name",
			"Usage: hams uv install <tool>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would install: uv tool install %s\n", strings.Join(args, " "))
		return nil
	}

	for _, tool := range tools {
		if err := p.runner.Install(ctx, tool); err != nil {
			return err
		}
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}
	for _, tool := range tools {
		hf.AddApp(tagCLI, tool, "")
		// Cycle 206: state write matches 96/202/203/204/205.
		sf.SetResource(tool, state.StateOK)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// handleRemove runs `uv tool uninstall <tool>` via the CmdRunner seam
// and, on success, removes each tool from the uv hamsfile.
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"uv remove requires a tool name",
			"Usage: hams uv remove <tool>",
		)
	}
	tools := toolArgs(args)
	if len(tools) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"uv remove requires at least one tool name",
			"Usage: hams uv remove <tool>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove: uv tool uninstall %s\n", strings.Join(args, " "))
		return nil
	}

	for _, tool := range tools {
		if err := p.runner.Uninstall(ctx, tool); err != nil {
			return err
		}
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}
	for _, tool := range tools {
		hf.RemoveApp(tool)
		sf.SetResource(tool, state.StateRemoved)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// statePath returns the absolute path to uv.state.yaml for the
// active machine. Mirrors homebrew/mas/cargo/npm/pnpm.statePath.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads uv.state.yaml or returns a fresh one
// when the file is absent. Non-ErrNotExist load failures propagate.
func (p *Provider) loadOrCreateStateFile(flags *provider.GlobalFlags) (*state.File, error) {
	cfg := p.effectiveConfig(flags)
	sf, err := state.Load(p.statePath(flags))
	if err == nil {
		return sf, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return state.New(p.Name(), cfg.MachineID), nil
	}
	return nil, fmt.Errorf("loading uv state %s: %w", p.statePath(flags), err)
}

// toolArgs filters positional tokens: flags (leading `-`) are excluded.
func toolArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "uv" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "uv" }

// parseUvToolList parses "uv tool list" output into a name→version map.
// Each line has the form: "tool-name v1.2.3".
func parseUvToolList(output string) map[string]string {
	result := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			name := parts[0]
			ver := ""
			if len(parts) >= 2 {
				ver = strings.TrimPrefix(parts[1], "v")
			}
			result[name] = ver
		}
	}
	return result
}
