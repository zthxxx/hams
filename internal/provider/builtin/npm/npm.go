// Package npm wraps the npm package manager for global Node.js package management.
package npm

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// cliName is the npm provider's manifest + CLI name.
const cliName = "npm"

// AutoInjectFlags are flags automatically added if not present.
// Used by HandleCommand passthrough; the apply/remove paths invoke the
// CmdRunner directly which always passes --global.
var AutoInjectFlags = map[string]string{"--global": ""}

// Provider implements the npm package manager provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new npm provider wired with a real CmdRunner.
// cfg supplies store/profile paths used by the CLI-first auto-record
// path (`hams npm install <pkg>` writes to the hamsfile after a
// successful runner call).
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the npm provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   cliName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		FilePrefix:    cliName,
	}
}

// Bootstrap checks if npm is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	return p.runner.LookPath()
}

// Probe queries npm for globally installed packages.
//
// Cycle 189: a state entry like `foo@1.2.3` or `@scope/bar@1.2.3`
// (recorded by `hams npm install -g foo@1.2.3`) strips the pin
// suffix before looking up in the bare-name `installed` map. Pre-
// cycle-189 the full state ID including `@version` was used as the
// lookup key and never matched — any user who pinned via CLI saw
// their drift detection permanently broken.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := p.runner.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := parseNpmList(output)
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		key := stripNpmVersionPin(id)
		if ver, ok := installed[key]; ok {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: ver})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		}
	}
	return results, nil
}

// stripNpmVersionPin strips the optional `@version` suffix from an
// npm package ID, preserving the leading `@` of a scoped package.
// Examples:
//
//	foo@1.2.3           → foo
//	@scope/bar@1.2.3    → @scope/bar
//	@scope/bar          → @scope/bar  (no version)
//	foo                 → foo
//
// Rule: use the LAST `@` as the version delimiter — but only when
// its position is > 0 (the initial `@` of a scoped name is preserved).
func stripNpmVersionPin(id string) string {
	idx := strings.LastIndex(id, "@")
	if idx <= 0 {
		return id
	}
	return id[:idx]
}

// Plan computes actions for npm packages and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	// Cycle 191/192: when a pin-upgrade produces both Install X@5.4
	// and Remove X@5.3 actions in the same apply, drop the Remove
	// exec AND directly tombstone the stale state entry so it doesn't
	// accumulate as StateOK noise. Rationale: npm uninstall with a
	// version-pinned arg is treated by npm as bare-name uninstall —
	// running it AFTER the new version was just installed would
	// uninstall the new version too.
	actions = suppressRedundantVersionRemoves(actions, observed)
	return provider.PopulateActionHooks(actions, desired), nil
}

// suppressRedundantVersionRemoves drops ActionRemove entries whose
// bare package name (version-stripped) matches an Install/Update/Skip
// action's bare name in the same plan. The provider's Remove exec
// would otherwise run `npm uninstall <pkg>@<old-ver>` which npm
// interprets as bare uninstall, clobbering the fresh install.
//
// Cycle 192: ALSO marks the suppressed ID as StateRemoved in the
// observed state file. Cycle 191 just dropped the action, leaving
// state at StateOK — which accumulated stale pin entries over many
// upgrades (visible in `hams list` and confusing to users). Now the
// state tombstone is applied directly at Plan time, before the
// executor loop; the subsequent sf.Save in runApply persists both
// the new Install's StateOK AND the stale pin's StateRemoved.
//
// This is npm-family-specific; the same helper is duplicated in
// pnpm/uv/vscodeext because extracting it into provider/plan.go
// would require coupling to pin-stripping functions that differ
// per ecosystem.
func suppressRedundantVersionRemoves(actions []provider.Action, observed *state.File) []provider.Action {
	keepBareNames := make(map[string]bool)
	for _, a := range actions {
		if a.Type == provider.ActionRemove {
			continue
		}
		keepBareNames[stripNpmVersionPin(a.ID)] = true
	}
	out := make([]provider.Action, 0, len(actions))
	for _, a := range actions {
		if a.Type == provider.ActionRemove && keepBareNames[stripNpmVersionPin(a.ID)] {
			slog.Info("npm: suppressing redundant version-pin remove (bare name overlaps install)",
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

// Apply installs an npm package globally.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("npm install", "package", action.ID)
	return p.runner.Install(ctx, action.ID)
}

// Remove uninstalls an npm package globally.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("npm uninstall", "package", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns installed packages with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for npm.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove", "uninstall", "rm":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	case "list":
		// Cycle 214: route `hams npm list` to the hams-tracked diff
		// via HandleListCmd. `npm list -g` shows the full global
		// dependency tree, not hams's recorded packages.
		return provider.HandleListCmd(ctx, p, p.effectiveConfig(flags))
	default:
		return provider.Passthrough(ctx, "npm", args, flags)
	}
}

// handleInstall runs `npm install -g <pkg>` via the CmdRunner seam and,
// on success, appends each package to the npm hamsfile so `hams apply`
// on another machine can restore it. All-or-nothing: any install
// failure aborts before the hamsfile is touched.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return provider.UsageRequiresResource(cliName, "install", "package name", "package")
	}
	pkgs := packageArgs(args)
	if len(pkgs) == 0 {
		return provider.UsageRequiresAtLeastOne(cliName, "install", "package name", "package")
	}
	hfPath, hfErr := p.hamsfilePath(hamsFlags, flags)
	if hfErr != nil {
		return hfErr
	}
	return provider.AutoRecordInstall(ctx, p.runner, pkgs,
		p.effectiveConfig(flags), flags,
		hfPath, p.statePath(flags),
		provider.PackageDispatchOpts{
			CLIName: cliName, InstallVerb: "install", RemoveVerb: "uninstall", HamsTag: tagCLI,
		},
	)
}

// handleRemove runs `npm uninstall -g <pkg>` via the CmdRunner seam
// and, on success, removes the package from the npm hamsfile.
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return provider.UsageRequiresResource(cliName, "remove", "package name", "package")
	}
	pkgs := packageArgs(args)
	if len(pkgs) == 0 {
		return provider.UsageRequiresAtLeastOne(cliName, "remove", "package name", "package")
	}
	hfPath, hfErr := p.hamsfilePath(hamsFlags, flags)
	if hfErr != nil {
		return hfErr
	}
	return provider.AutoRecordRemove(ctx, p.runner, pkgs,
		p.effectiveConfig(flags), flags,
		hfPath, p.statePath(flags),
		provider.PackageDispatchOpts{
			CLIName: cliName, InstallVerb: "install", RemoveVerb: "uninstall", HamsTag: tagCLI,
		},
	)
}

// statePath returns the absolute path to npm.state.yaml for the
// active machine. Mirrors homebrew/mas/cargo.statePath.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// packageArgs filters positional tokens from args: flags (leading `-`)
// are excluded so they don't get recorded as package names. Mirrors
// apt.packageArgs and cargo.crateArgs.
func packageArgs(args []string) []string {
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
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return cliName }

func parseNpmList(output string) map[string]string {
	result := make(map[string]string)
	var data struct {
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return result
	}
	for name := range data.Dependencies {
		result[name] = ""
	}
	return result
}
