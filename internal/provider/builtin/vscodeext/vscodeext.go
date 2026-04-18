// Package vscodeext wraps the VS Code CLI for extension management.
package vscodeext

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

const (
	// cliName is the vscodeext provider's manifest + CLI name. The
	// package directory stays `vscodeext` because the Go type is
	// `code.Provider` would collide with a "code" stdlib-adjacent
	// noun; the user-facing name is "code" (the VS Code CLI binary)
	// and that is what Manifest.Name / FilePrefix / registry key all
	// expose. hams has not formally released, so no migration compat
	// layer is required for the legacy `code-ext` manifest name.
	cliName = "code"
	// displayName is the human-readable display name.
	displayName = "VS Code Extensions"
)

// Provider implements the VS Code extension provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new VS Code extension provider wired with a real
// CmdRunner. cfg supplies store/profile paths for the CLI-first
// auto-record path.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the vscodeext provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   displayName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		DependsOn: []provider.DependOn{
			{
				Provider: "brew",
				Package:  "visual-studio-code",
			},
		},
		FilePrefix: "code",
	}
}

// Bootstrap checks if the code CLI is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	return p.runner.LookPath()
}

// Probe queries VS Code for installed extensions.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := p.runner.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := parseExtensionList(output)
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		// Cycle 188: strip the optional `@version` suffix from the
		// state ID before matching against the `installed` map (which
		// keys on bare publisher.extension only — parseExtensionList
		// drops the version from the key). Pre-cycle-188 a state
		// entry like "foo.bar@1.2.3" NEVER matched — Probe always
		// reported StateFailed, drift detection was broken for any
		// user who pinned a version via `hams code install
		// publisher.ext@1.2.3`. Extension IDs are case-insensitive.
		lowerID := stripExtensionVersionPin(strings.ToLower(id))
		if ver, ok := installed[lowerID]; ok {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: ver})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		}
	}
	return results, nil
}

// stripExtensionVersionPin strips the optional @version suffix from
// a VS Code extension ID. Extension IDs are of form
// `publisher.extension[@version]` — no scoped-package ambiguity since
// there's no leading `@`. Cycle 188 introduced the inline strip; cycle
// 191 extracts it as a helper so the Plan-level version-pin-remove
// suppressor (below) can share the logic.
func stripExtensionVersionPin(id string) string {
	if idx := strings.Index(id, "@"); idx > 0 {
		return id[:idx]
	}
	return id
}

// suppressRedundantVersionRemoves drops ActionRemove entries whose
// bare extension ID matches an Install/Update/Skip action in the
// same plan and tombstones the stale state entry. Cycle 191/192 —
// same rationale as npm: `code --uninstall-extension foo.bar@1.2.3`
// would uninstall foo.bar entirely after the new version was just
// installed.
func suppressRedundantVersionRemoves(actions []provider.Action, observed *state.File) []provider.Action {
	keepBareNames := make(map[string]bool)
	for _, a := range actions {
		if a.Type == provider.ActionRemove {
			continue
		}
		keepBareNames[stripExtensionVersionPin(strings.ToLower(a.ID))] = true
	}
	out := make([]provider.Action, 0, len(actions))
	for _, a := range actions {
		if a.Type == provider.ActionRemove && keepBareNames[stripExtensionVersionPin(strings.ToLower(a.ID))] {
			slog.Info("code: suppressing redundant version-pin remove (bare name overlaps install)",
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

// Plan computes actions for VS Code extensions and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	// Cycle 191/192: drop redundant version-pinned removes + tombstone
	// the stale state entry (same rationale as npm).
	actions = suppressRedundantVersionRemoves(actions, observed)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply installs a VS Code extension.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("code --install-extension", "extension", action.ID)
	return p.runner.Install(ctx, action.ID)
}

// Remove uninstalls a VS Code extension.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("code --uninstall-extension", "extension", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns installed VS Code extensions with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for code.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove", "uninstall", "rm":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	case "list":
		// Cycle 214: route `hams code list` to the hams-tracked
		// diff. `code list` is not a valid VS Code CLI subcommand
		// (ext listing is `code --list-extensions`), so passthrough
		// produced a cryptic VS Code error.
		return provider.HandleListCmd(ctx, p, p.effectiveConfig(flags))
	default:
		return provider.WrapExecPassthrough(ctx, "code", args, nil)
	}
}

// handleInstall runs `code --install-extension <ext>` via the CmdRunner
// seam and, on success, appends each extension ID to the code
// hamsfile.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"code install requires an extension ID",
			"Usage: hams code install <publisher.extension>",
			"To install all recorded extensions, use: hams apply --only=code",
		)
	}
	exts := extensionArgs(args)
	if len(exts) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"code install requires at least one extension ID",
			"Usage: hams code install <publisher.extension>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would install: code --install-extension %s\n", strings.Join(exts, " "))
		return nil
	}

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(p.effectiveConfig(flags), flags, "code install")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	for _, ext := range exts {
		if err := p.runner.Install(ctx, ext); err != nil {
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
	for _, ext := range exts {
		hf.AddApp(tagCLI, ext, "")
		// Cycle 208: state write matches cycles 96/202-207. Final
		// Package-class provider to gain CP-1 state-write parity.
		sf.SetResource(ext, state.StateOK)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// handleRemove runs `code --uninstall-extension <ext>` via the
// CmdRunner seam and, on success, removes each extension from the
// code hamsfile.
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"code remove requires an extension ID",
			"Usage: hams code remove <publisher.extension>",
		)
	}
	exts := extensionArgs(args)
	if len(exts) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"code remove requires at least one extension ID",
			"Usage: hams code remove <publisher.extension>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove: code --uninstall-extension %s\n", strings.Join(exts, " "))
		return nil
	}

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(p.effectiveConfig(flags), flags, "code remove")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	for _, ext := range exts {
		if err := p.runner.Uninstall(ctx, ext); err != nil {
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
	for _, ext := range exts {
		hf.RemoveApp(ext)
		sf.SetResource(ext, state.StateRemoved)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// statePath returns the absolute path to code.state.yaml for the
// active machine. Manifest.Name / FilePrefix / the CLI verb all agree
// on `code` — hams has not formally released, so we do not carry the
// legacy `vscodeext.hams.yaml` / `code-ext` divergence forward.
// Mirrors homebrew/mas/cargo/npm/pnpm/uv/goinstall.statePath.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads vscodeext.state.yaml or returns a fresh
// one when the file is absent. Non-ErrNotExist load failures propagate.
func (p *Provider) loadOrCreateStateFile(flags *provider.GlobalFlags) (*state.File, error) {
	cfg := p.effectiveConfig(flags)
	sf, err := state.Load(p.statePath(flags))
	if err == nil {
		return sf, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return state.New(p.Name(), cfg.MachineID), nil
	}
	return nil, fmt.Errorf("loading code state %s: %w", p.statePath(flags), err)
}

// extensionArgs filters positional tokens: flags (leading `-`) are
// excluded so they don't get recorded as extension IDs.
func extensionArgs(args []string) []string {
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
func (p *Provider) DisplayName() string { return displayName }

// parseExtensionList parses `code --list-extensions --show-versions` output.
// Each line has the form "publisher.extension@version". Lines whose name
// is empty or contains internal whitespace are skipped — extension IDs
// cannot contain whitespace per VS Code's marketplace identity rules,
// so any such line is malformed (likely an ANSI-escape leak from CI or
// a paginator splash) and including it would corrupt the diff.
func parseExtensionList(output string) map[string]string {
	result := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "@", 2)
		name := strings.ToLower(parts[0])
		if name == "" || strings.ContainsAny(name, " \t\n\r") {
			continue
		}
		ver := ""
		if len(parts) == 2 {
			ver = parts[1]
		}
		result[name] = ver
	}
	return result
}
