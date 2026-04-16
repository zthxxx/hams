// Package duti wraps the duti command for managing default app associations on macOS.
package duti

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// cliName is the duti provider's manifest + CLI name.
const cliName = "duti"

// Provider implements the duti default-app association provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new duti provider wired with a real CmdRunner.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// dutiInstallScript is the consent-gated install command. brew is the
// host (already on PATH if the user's fresh-Mac chain went brew →
// duti). Extracted so unit tests can assert Script-matches-manifest.
const dutiInstallScript = "brew install duti"

// dutiBinaryLookup is the PATH-check seam Bootstrap uses.
var dutiBinaryLookup = exec.LookPath

// Manifest returns the duti provider metadata.
//
// Two DependsOn entries, each with a single purpose (see pnpm.go for
// the full rationale): one DAG-only entry ordering brew before duti,
// and one bash-hosted script entry whose install command `brew install
// duti` calls into the (already-bootstrapped) brew provider at the
// shell layer. Only `bash` implements provider.BashScriptRunner.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   cliName,
		Platforms:     []provider.Platform{provider.PlatformDarwin},
		ResourceClass: provider.ClassKVConfig,
		DependsOn: []provider.DependOn{
			{Provider: "brew", Platform: provider.PlatformDarwin},
			{Provider: "bash", Script: dutiInstallScript, Platform: provider.PlatformDarwin},
		},
		FilePrefix: cliName,
	}
}

// Bootstrap reports whether duti is installed. A missing binary is
// signaled via provider.BootstrapRequiredError so the CLI consent
// flow can surface the install script + --bootstrap remedy.
func (p *Provider) Bootstrap(_ context.Context) error {
	if err := p.runner.LookPath(); err == nil {
		return nil
	}
	return &provider.BootstrapRequiredError{
		Provider: "duti",
		Binary:   "duti",
		Script:   dutiInstallScript,
	}
}

// Probe checks the current default app for each tracked file extension.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		ext, _, err := parseResourceID(id)
		if err != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed, ErrorMsg: err.Error()})
			continue
		}

		output, queryErr := p.runner.QueryDefault(ctx, ext)
		if queryErr != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
			continue
		}

		currentApp := parseDutiOutput(output)
		results = append(results, provider.ProbeResult{
			ID:    id,
			State: state.StateOK,
			Value: currentApp,
		})
	}
	return results, nil
}

// Plan computes actions for duti associations and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply sets a default app association via duti.
// Resource ID format: "<ext>=<bundle-id>" e.g. "pdf=com.adobe.acrobat.pdf".
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	ext, bundleID, err := parseResourceID(action.ID)
	if err != nil {
		return err
	}
	slog.Info("duti set", "ext", ext, "bundle_id", bundleID)
	return p.runner.SetDefault(ctx, ext, bundleID)
}

// Remove is a no-op for duti; macOS does not have a direct "reset to default" command.
func (p *Provider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("duti does not support automatic removal; reset the association manually via System Settings", "resource", resourceID)
	return nil
}

// List returns duti associations with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for duti. The canonical
// shape `hams duti <ext>=<bundle-id>` is auto-recorded into the
// hamsfile and state so subsequent `hams apply` runs on other
// machines reproduce the association. Other argument shapes (raw
// `duti` flags like `-x`, `-s`) are passed through to the host
// binary without bookkeeping.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"duti requires arguments",
			"Usage: hams duti <ext>=<bundle-id>",
			"Example: hams duti pdf=com.adobe.acrobat.pdf",
		)
	}

	// Canonical shape: a single `<ext>=<bundle-id>` argument. Anything
	// else (multiple args, flags) is a raw duti invocation and flows
	// through the exec passthrough.
	if len(args) == 1 && strings.Contains(args[0], "=") && !strings.HasPrefix(args[0], "-") {
		return p.handleSet(ctx, args[0], hamsFlags, flags)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: duti %s\n", strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(ctx, "duti", args...) //nolint:gosec // duti args from CLI input
	return cmd.Run()
}

// handleSet parses `<ext>=<bundle-id>`, runs the CmdRunner seam and
// auto-records the association. Same-ext different-bundle-id
// invocations replace the stale hamsfile entry in place.
func (p *Provider) handleSet(ctx context.Context, arg string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	ext, bundleID, err := parseResourceID(arg)
	if err != nil {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			err.Error(),
			"Usage: hams duti <ext>=<bundle-id>",
			"Example: hams duti pdf=com.adobe.acrobat.pdf",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: duti -s %s .%s all\n", bundleID, ext)
		return nil
	}

	if err := p.runner.SetDefault(ctx, ext, bundleID); err != nil {
		return err
	}

	return p.recordSet(ext, bundleID, hamsFlags, flags)
}

// recordSet persists the (ext, bundleID) pair into the hamsfile and
// state. If an entry for the same ext with a DIFFERENT bundle-id
// already exists, it is replaced in place (old → StateRemoved,
// new → StateOK) so the hamsfile stays single-valued per extension.
func (p *Provider) recordSet(ext, bundleID string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	sf := p.loadOrCreateStateFile(flags)

	newEntry := ext + "=" + bundleID
	prefix := ext + "="
	for _, existing := range hf.ListApps() {
		if existing == newEntry {
			continue
		}
		if strings.HasPrefix(existing, prefix) {
			hf.RemoveApp(existing)
			sf.SetResource(existing, state.StateRemoved)
		}
	}

	hf.AddApp(tagCLI, newEntry, "")
	sf.SetResource(newEntry, state.StateOK, state.WithValue(bundleID))

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return cliName }

// parseResourceID splits "<ext>=<bundle-id>" into its components.
func parseResourceID(id string) (ext, bundleID string, err error) {
	parts := strings.SplitN(id, "=", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("duti: resource ID must be '<ext>=<bundle-id>', got %q", id)
	}
	return strings.TrimPrefix(parts[0], "."), parts[1], nil
}

// parseDutiOutput extracts the bundle ID or app name from `duti -x <ext>` output.
// The output typically has the app name on the first line.
func parseDutiOutput(output string) string {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
