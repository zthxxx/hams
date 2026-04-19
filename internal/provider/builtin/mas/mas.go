// Package mas wraps the Mac App Store CLI (mas) for macOS app management.
package mas

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

const (
	cliName     = "mas"
	displayName = "Mac App Store"
)

// masInstallScript is the consent-gated install command.
const masInstallScript = "brew install mas"

// masBinaryLookup is the PATH-check seam Bootstrap uses.
var masBinaryLookup = exec.LookPath

// Provider implements the Mac App Store provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new mas provider wired with a real CmdRunner.
// cfg supplies store/profile paths for the CLI-first auto-record path.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the mas provider metadata.
//
// Two DependsOn entries, each with a single purpose (see pnpm.go for
// the full rationale): DAG-only entry ordering brew before mas, and
// a bash-hosted script entry whose install command `brew install mas`
// calls into brew at the shell layer. Only `bash` implements
// provider.BashScriptRunner.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   displayName,
		Platforms:     []provider.Platform{provider.PlatformDarwin},
		ResourceClass: provider.ClassPackage,
		DependsOn: []provider.DependOn{
			{Provider: "brew", Platform: provider.PlatformDarwin},
			{Provider: "bash", Script: masInstallScript, Platform: provider.PlatformDarwin},
		},
		FilePrefix: cliName,
	}
}

// Bootstrap reports whether mas is installed. A missing binary is
// signaled via provider.BootstrapRequiredError so the CLI consent
// flow can surface the install script + --bootstrap remedy.
func (p *Provider) Bootstrap(_ context.Context) error {
	if err := p.runner.LookPath(); err == nil {
		return nil
	}
	return &provider.BootstrapRequiredError{
		Provider: cliName,
		Binary:   cliName,
		Script:   masInstallScript,
	}
}

// Probe queries mas for installed apps.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := p.runner.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := parseMasList(output)
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		if ver, ok := installed[id]; ok {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: ver})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		}
	}
	return results, nil
}

// Plan computes actions for mas apps and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply installs a Mac App Store app by numeric ID.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("mas install", "app_id", action.ID)
	return p.runner.Install(ctx, action.ID)
}

// Remove uninstalls a Mac App Store app by numeric ID.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("mas uninstall", "app_id", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns installed mas apps with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for mas.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove", "uninstall", "rm":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	case "list":
		// Cycle 214: route `hams mas list` to the hams-tracked diff.
		// `mas list` is a real mas subcommand (shows all signed-in
		// Mac App Store apps) but shows host-wide state rather than
		// hams's recorded apps — wrong affordance for the user who
		// wants to know what hams is tracking.
		return provider.HandleListCmd(ctx, p, p.effectiveConfig(flags))
	default:
		return provider.Passthrough(ctx, cliName, args, flags)
	}
}

// handleInstall runs `mas install <id>` via the CmdRunner seam and,
// on success, appends each numeric app ID to the mas hamsfile.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return provider.UsageRequiresResource("mas", "install", "numeric app ID", "app-id")
	}
	ids := appIDArgs(args)
	if len(ids) == 0 {
		return provider.UsageRequiresAtLeastOne("mas", "install", "numeric app ID", "app-id")
	}
	hfPath, hfErr := p.hamsfilePath(hamsFlags, flags)
	if hfErr != nil {
		return hfErr
	}
	return provider.AutoRecordInstall(ctx, p.runner, ids,
		p.effectiveConfig(flags), flags,
		hfPath, p.statePath(flags),
		provider.PackageDispatchOpts{
			CLIName: "mas", InstallVerb: "install", RemoveVerb: "uninstall", HamsTag: tagCLI,
		},
	)
}

// handleRemove runs `mas uninstall <id>` via the CmdRunner seam and,
// on success, removes each ID from the mas hamsfile.
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return provider.UsageRequiresResource("mas", "remove", "numeric app ID", "app-id")
	}
	ids := appIDArgs(args)
	if len(ids) == 0 {
		return provider.UsageRequiresAtLeastOne("mas", "remove", "numeric app ID", "app-id")
	}
	hfPath, hfErr := p.hamsfilePath(hamsFlags, flags)
	if hfErr != nil {
		return hfErr
	}
	return provider.AutoRecordRemove(ctx, p.runner, ids,
		p.effectiveConfig(flags), flags,
		hfPath, p.statePath(flags),
		provider.PackageDispatchOpts{
			CLIName: "mas", InstallVerb: "install", RemoveVerb: "uninstall", HamsTag: tagCLI,
		},
	)
}

// statePath returns the absolute path to mas.state.yaml for the
// active machine. Mirrors homebrew.statePath.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// appIDArgs filters positional tokens: flags (leading `-`) are
// excluded so they don't get recorded as app IDs.
func appIDArgs(args []string) []string {
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

// parseMasList parses `mas list` output into an appID→version map.
// Each line has the form: "1234567890  App Name (version)".
func parseMasList(output string) map[string]string {
	result := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}
		appID := parts[0]
		ver := ""
		// Version appears in parentheses at the end: "App Name (1.2.3)".
		if len(parts) >= 2 {
			last := parts[len(parts)-1]
			if strings.HasPrefix(last, "(") && strings.HasSuffix(last, ")") {
				ver = strings.Trim(last, "()")
			}
		}
		result[appID] = ver
	}
	return result
}
