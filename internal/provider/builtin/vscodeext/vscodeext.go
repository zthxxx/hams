// Package vscodeext wraps the VS Code CLI for extension management.
package vscodeext

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

const (
	// cliName is the vscodeext provider's manifest + CLI name.
	// Per CLAUDE.md Current Tasks: "The code-ext provider likewise
	// should expose only the `hams code` entry point." `code` is
	// VSCode-specific; a Cursor provider would be separate.
	cliName = "code"
	// filePrefix stays as `code` so new scaffolded hamsfiles read
	// `code.hams.yaml`, matching the CLI verb rather than the pre-
	// rename `vscodeext.hams.yaml`. Existing stores carrying the old
	// file name will need a one-time rename — pre-v1, breaking the
	// on-disk name is acceptable since the CLI verb itself is the
	// documented identity of the provider.
	filePrefix = "code"
	// displayName shows "VS Code Extensions" in help and errors, the
	// product-branded form (with the space; older spec wrote
	// "VSCode Extension" without).
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
		FilePrefix: filePrefix,
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
			slog.Info("code-ext: suppressing redundant version-pin remove (bare name overlaps install)",
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

// HandleCommand processes CLI subcommands for code-ext.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove", "uninstall", "rm":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	case "list":
		// Cycle 214: route `hams code-ext list` to the hams-tracked
		// diff. `code list` is not a valid VS Code CLI subcommand
		// (ext listing is `code --list-extensions`), so passthrough
		// produced a cryptic VS Code error.
		return provider.HandleListCmd(ctx, p, p.effectiveConfig(flags))
	default:
		return provider.Passthrough(ctx, "code", args, flags)
	}
}

// handleInstall runs `code --install-extension <ext>` via the CmdRunner
// seam and, on success, appends each extension ID to the code-ext
// hamsfile.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return provider.UsageRequiresResource(cliName, "install", "extension ID", "publisher.extension")
	}
	exts := extensionArgs(args)
	if len(exts) == 0 {
		return provider.UsageRequiresAtLeastOne(cliName, "install", "extension ID", "publisher.extension")
	}
	hfPath, hfErr := p.hamsfilePath(hamsFlags, flags)
	if hfErr != nil {
		return hfErr
	}
	return provider.AutoRecordInstall(ctx, p.runner, exts,
		p.effectiveConfig(flags), flags,
		hfPath, p.statePath(flags),
		provider.PackageDispatchOpts{
			CLIName: cliName, InstallVerb: "install", RemoveVerb: "uninstall", HamsTag: tagCLI,
		},
	)
}

// handleRemove runs `code --uninstall-extension <ext>` via the
// CmdRunner seam and, on success, removes each extension from the
// code-ext hamsfile.
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return provider.UsageRequiresResource(cliName, "remove", "extension ID", "publisher.extension")
	}
	exts := extensionArgs(args)
	if len(exts) == 0 {
		return provider.UsageRequiresAtLeastOne(cliName, "remove", "extension ID", "publisher.extension")
	}
	hfPath, hfErr := p.hamsfilePath(hamsFlags, flags)
	if hfErr != nil {
		return hfErr
	}
	return provider.AutoRecordRemove(ctx, p.runner, exts,
		p.effectiveConfig(flags), flags,
		hfPath, p.statePath(flags),
		provider.PackageDispatchOpts{
			CLIName: cliName, InstallVerb: "install", RemoveVerb: "uninstall", HamsTag: tagCLI,
		},
	)
}

// statePath returns the absolute path to vscodeext.state.yaml for the
// active machine. The FilePrefix is "vscodeext" (NOT "code-ext") for
// historical reasons: early v1 shipped `vscodeext.hams.yaml` as the
// canonical filename before the CLI name was finalized as `code-ext`.
// Renaming the prefix now would invalidate every existing user's
// hamsfile and state paths, so CLI and file-layer names diverge by
// design. Mirrors homebrew/mas/cargo/npm/pnpm/uv/goinstall.statePath.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
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
