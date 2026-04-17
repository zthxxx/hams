// Package pnpm wraps the pnpm package manager for global Node.js package management.
package pnpm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// cliName is the pnpm provider's manifest + CLI name.
const cliName = "pnpm"

// AutoInjectFlags are flags automatically added if not present (used
// by the HandleCommand passthrough; apply/remove paths use the
// CmdRunner which always passes -g).
var AutoInjectFlags = map[string]string{"--global": ""}

// Provider implements the pnpm package manager provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new pnpm provider wired with a real CmdRunner.
// cfg supplies store/profile paths used by the CLI-first auto-record
// path (`hams pnpm add <pkg>` writes to the hamsfile after a
// successful runner call).
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// pnpmInstallScript is the consent-gated install command. npm is the
// host (already on PATH by the time this runs, since pnpm depends on
// npm in the DAG). Extracted so unit tests can assert Script-matches-
// manifest invariants without duplicating the string.
const pnpmInstallScript = "npm install -g pnpm"

// pnpmBinaryLookup is the PATH-check seam Bootstrap uses. Swapped in
// tests to simulate "pnpm missing" / "pnpm present" without mutating
// the host's real PATH. Production value is exec.LookPath.
var pnpmBinaryLookup = exec.LookPath

// Manifest returns the pnpm provider metadata.
//
// Two DependsOn entries, each with a single purpose:
//
//   - `{Provider: "npm"}` — DAG ordering only (no Script). Ensures
//     npm is processed before pnpm across the apply pipeline.
//   - `{Provider: "bash", Script: ...}` — script host. `bash` is the
//     only provider that implements `provider.BashScriptRunner`, so
//     any DependsOn entry with a `.Script` MUST target bash; the
//     script's own invocation (here `npm install -g pnpm`) is what
//     calls into npm. Separating these avoids the conflation that
//     would otherwise make RunBootstrap type-assert an npm provider
//     to BashScriptRunner and fail.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   cliName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		DependsOn: []provider.DependOn{
			{Provider: "npm", Package: cliName},
			{Provider: "bash", Script: pnpmInstallScript},
		},
		FilePrefix: cliName,
	}
}

// Bootstrap reports whether pnpm is installed. A missing binary is
// signaled via provider.BootstrapRequiredError (which wraps
// provider.ErrBootstrapRequired); the CLI orchestrator decides whether
// to run the manifest-declared install script based on --bootstrap /
// TTY prompt. Bootstrap itself NEVER executes a network install.
//
// LookPath is delegated to the CmdRunner so unit tests can simulate
// "missing pnpm" via WithLookPathError without mutating the host's
// real PATH.
func (p *Provider) Bootstrap(_ context.Context) error {
	if err := p.runner.LookPath(); err == nil {
		return nil
	}
	return &provider.BootstrapRequiredError{
		Provider: "pnpm",
		Binary:   "pnpm",
		Script:   pnpmInstallScript,
	}
}

// Probe queries pnpm for globally installed packages.
//
// Cycle 189: state IDs with `@version` pins strip the suffix before
// the installed-map lookup. Pre-cycle-189 a pinned state entry
// never matched — drift detection was broken for any user who
// pinned via CLI. Scoped packages (`@scope/bar`) preserve the
// leading `@`; only the LAST `@` (position > 0) is treated as the
// version delimiter.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := p.runner.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := parsePnpmList(output)
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		key := stripPnpmVersionPin(id)
		if ver, ok := installed[key]; ok {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: ver})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		}
	}
	return results, nil
}

// stripPnpmVersionPin strips the optional `@version` suffix from a
// pnpm package ID, preserving the leading `@` of a scoped package.
// Same rule as npm (cycle 189).
func stripPnpmVersionPin(id string) string {
	idx := strings.LastIndex(id, "@")
	if idx <= 0 {
		return id
	}
	return id[:idx]
}

// suppressRedundantVersionRemoves drops ActionRemove entries whose
// bare package name (version-stripped) matches an Install/Update/Skip
// action and tombstones the stale state entry. Cycle 191/192 — same
// rationale as npm.
func suppressRedundantVersionRemoves(actions []provider.Action, observed *state.File) []provider.Action {
	keepBareNames := make(map[string]bool)
	for _, a := range actions {
		if a.Type == provider.ActionRemove {
			continue
		}
		keepBareNames[stripPnpmVersionPin(a.ID)] = true
	}
	out := make([]provider.Action, 0, len(actions))
	for _, a := range actions {
		if a.Type == provider.ActionRemove && keepBareNames[stripPnpmVersionPin(a.ID)] {
			slog.Info("pnpm: suppressing redundant version-pin remove (bare name overlaps install)",
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

// Plan computes actions for pnpm packages and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	// Cycle 191/192: drop redundant version-pinned removes + tombstone
	// the stale state entry (same rationale as npm).
	actions = suppressRedundantVersionRemoves(actions, observed)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply installs a pnpm package globally.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("pnpm add", "package", action.ID)
	return p.runner.Install(ctx, action.ID)
}

// Remove uninstalls a pnpm package globally.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("pnpm remove", "package", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns installed packages with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for pnpm.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "add", "install", "i":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove", "rm", "uninstall":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	case "list":
		// Cycle 214: route `hams pnpm list` to the hams-tracked diff.
		// `pnpm list -g` prints the full dependency tree, not hams's
		// recorded packages.
		return provider.HandleListCmd(ctx, p, p.effectiveConfig(flags))
	default:
		return provider.WrapExecPassthrough(ctx, "pnpm", args, nil)
	}
}

// handleInstall runs `pnpm add -g <pkg>` via the CmdRunner seam and,
// on success, appends each package to the pnpm hamsfile. All-or-
// nothing: any install failure aborts before the hamsfile is touched.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"pnpm install requires a package name",
			"Usage: hams pnpm add <package>",
			"To install all recorded packages, use: hams apply --only=pnpm",
		)
	}
	pkgs := packageArgs(args)
	if len(pkgs) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"pnpm install requires at least one package name",
			"Usage: hams pnpm add <package>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would install: pnpm add -g %s\n", strings.Join(args, " "))
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
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		hf.AddApp(tagCLI, pkg, "")
		// Cycle 205: state write is additive, matching cycles
		// 96/202/203/204. Without this, `hams list --only=pnpm`
		// returned empty immediately after a successful install.
		sf.SetResource(pkg, state.StateOK)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// handleRemove runs `pnpm remove -g <pkg>` via the CmdRunner seam and,
// on success, removes each package from the pnpm hamsfile.
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"pnpm remove requires a package name",
			"Usage: hams pnpm remove <package>",
		)
	}
	pkgs := packageArgs(args)
	if len(pkgs) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"pnpm remove requires at least one package name",
			"Usage: hams pnpm remove <package>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove: pnpm remove -g %s\n", strings.Join(args, " "))
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
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		hf.RemoveApp(pkg)
		sf.SetResource(pkg, state.StateRemoved)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// statePath returns the absolute path to pnpm.state.yaml for the
// active machine. Mirrors homebrew/mas/cargo/npm.statePath.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads pnpm.state.yaml or returns a fresh one
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
	return nil, fmt.Errorf("loading pnpm state %s: %w", p.statePath(flags), err)
}

// packageArgs filters positional tokens: flags (leading `-`) are
// excluded so they don't get recorded as package names.
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

func parsePnpmList(output string) map[string]string {
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
