// Package vscodeext wraps the VS Code CLI for extension management.
package vscodeext

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

const (
	// cliName is the vscodeext provider's manifest + CLI name.
	cliName = "code-ext"
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
		FilePrefix: "vscodeext",
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
		// Extension IDs are case-insensitive.
		lowerID := strings.ToLower(id)
		if ver, ok := installed[lowerID]; ok {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: ver})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		}
	}
	return results, nil
}

// Plan computes actions for VS Code extensions and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
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
	default:
		return provider.WrapExecPassthrough(ctx, "code", args, nil)
	}
}

// handleInstall runs `code --install-extension <ext>` via the CmdRunner
// seam and, on success, appends each extension ID to the code-ext
// hamsfile.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"code-ext install requires an extension ID",
			"Usage: hams code-ext install <publisher.extension>",
			"To install all recorded extensions, use: hams apply --only=code-ext",
		)
	}
	exts := extensionArgs(args)
	if len(exts) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"code-ext install requires at least one extension ID",
			"Usage: hams code-ext install <publisher.extension>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would install: code --install-extension %s\n", strings.Join(exts, " "))
		return nil
	}

	for _, ext := range exts {
		if err := p.runner.Install(ctx, ext); err != nil {
			return err
		}
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	for _, ext := range exts {
		hf.AddApp(tagCLI, ext, "")
	}
	return hf.Write()
}

// handleRemove runs `code --uninstall-extension <ext>` via the
// CmdRunner seam and, on success, removes each extension from the
// code-ext hamsfile.
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"code-ext remove requires an extension ID",
			"Usage: hams code-ext remove <publisher.extension>",
		)
	}
	exts := extensionArgs(args)
	if len(exts) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"code-ext remove requires at least one extension ID",
			"Usage: hams code-ext remove <publisher.extension>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove: code --uninstall-extension %s\n", strings.Join(exts, " "))
		return nil
	}

	for _, ext := range exts {
		if err := p.runner.Uninstall(ctx, ext); err != nil {
			return err
		}
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	for _, ext := range exts {
		hf.RemoveApp(ext)
	}
	return hf.Write()
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
