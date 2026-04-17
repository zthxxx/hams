// Package npm wraps the npm package manager for global Node.js package management.
package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
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
	return provider.PopulateActionHooks(actions, desired), nil
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
	default:
		return provider.WrapExecPassthrough(ctx, "npm", args, nil)
	}
}

// handleInstall runs `npm install -g <pkg>` via the CmdRunner seam and,
// on success, appends each package to the npm hamsfile so `hams apply`
// on another machine can restore it. All-or-nothing: any install
// failure aborts before the hamsfile is touched.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"npm install requires a package name",
			"Usage: hams npm install <package>",
			"To install all recorded packages, use: hams apply --only=npm",
		)
	}
	pkgs := packageArgs(args)
	if len(pkgs) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"npm install requires at least one package name",
			"Usage: hams npm install <package>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would install: npm install -g %s\n", strings.Join(args, " "))
		return nil
	}

	for _, pkg := range pkgs {
		if err := p.runner.Install(ctx, pkg); err != nil {
			return err
		}
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		hf.AddApp(tagCLI, pkg, "")
	}
	return hf.Write()
}

// handleRemove runs `npm uninstall -g <pkg>` via the CmdRunner seam
// and, on success, removes the package from the npm hamsfile.
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"npm remove requires a package name",
			"Usage: hams npm remove <package>",
		)
	}
	pkgs := packageArgs(args)
	if len(pkgs) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"npm remove requires at least one package name",
			"Usage: hams npm remove <package>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove: npm uninstall -g %s\n", strings.Join(args, " "))
		return nil
	}

	for _, pkg := range pkgs {
		if err := p.runner.Uninstall(ctx, pkg); err != nil {
			return err
		}
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		hf.RemoveApp(pkg)
	}
	return hf.Write()
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
