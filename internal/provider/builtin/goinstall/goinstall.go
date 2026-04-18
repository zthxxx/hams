// Package goinstall wraps `go install` for installing Go binaries.
package goinstall

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
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/baseprovider"
	"github.com/zthxxx/hams/internal/state"
)

const (
	// cliName is the goinstall provider's manifest + CLI name.
	cliName = "goinstall"
	// displayName is the human-readable display name.
	displayName = "go install"
)

// Provider implements the go install provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new go install provider wired with a real CmdRunner.
// cfg supplies store/profile paths for the CLI-first auto-record path.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the go install provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   displayName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		FilePrefix:    cliName,
	}
}

// Bootstrap checks if go is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	return p.runner.LookPath()
}

// Probe checks installed Go binaries by delegating presence detection
// to the runner (which in production: queries `go env GOPATH`, runs
// `<gopath>/bin/<name> --version`, falls back to exec.LookPath).
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		if p.runner.IsBinaryInstalled(ctx, id) {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		}
	}
	return results, nil
}

// Plan computes actions for go install packages and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	return provider.PopulateActionHooks(actions, desired), nil
}

// injectLatest appends @latest to a resource ID if no version is specified.
func injectLatest(resourceID string) string {
	if !strings.Contains(resourceID, "@") {
		return resourceID + "@latest"
	}
	return resourceID
}

// Apply installs a Go package via go install.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	pkg := injectLatest(action.ID)
	slog.Info("go install", "package", pkg)
	return p.runner.Install(ctx, pkg)
}

// Remove is a no-op for go install; binaries must be removed manually.
func (p *Provider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("go install does not support automatic removal; remove the binary manually", "package", resourceID)
	return nil
}

// List returns installed go packages with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for goinstall.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "list":
		// Cycle 214: route `hams goinstall list` to the hams-tracked
		// diff. `go list` is a real subcommand of the go toolchain
		// but it prints Go package info, not installed binaries —
		// wrong affordance for the user who just ran
		// `hams goinstall install github.com/…`.
		return provider.HandleListCmd(ctx, p, baseprovider.EffectiveConfig(p.cfg, flags))
	default:
		// Unintercepted subcommand — passthrough to real `go` with
		// stdio preserved so `hams goinstall help`, `hams goinstall
		// version`, etc. behave identically to the unwrapped tool.
		return provider.Passthrough(ctx, "go", args, flags)
	}
}

// handleInstall runs `go install <pkg@version>` via the CmdRunner seam
// and, on success, appends the resolved (pinned) package path to the
// goinstall hamsfile. The recorded entry preserves user intent — if
// the user typed `go install foo@v1.2.3`, `foo@v1.2.3` is what lands
// in the hamsfile; bare `foo` becomes `foo@latest` via `injectLatest`
// so later `hams apply` reproduces the original install.
//
// goinstall has no uninstall verb (binaries must be removed manually),
// so only the install branch auto-records.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			i18n.Tf(i18n.ProviderErrInstallNeedsPackage, map[string]any{"Provider": "goinstall"}),
			"Usage: hams goinstall install <pkg[@version]>",
			i18n.Tf(i18n.ProviderErrInstallNeedsPackageBulk, map[string]any{"Provider": "goinstall"}),
		)
	}
	paths := packageArgs(args)
	if len(paths) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			i18n.Tf(i18n.ProviderErrInstallNeedsPackageAtLeastOne, map[string]any{"Provider": "goinstall"}),
			"Usage: hams goinstall install <pkg[@version]>",
		)
	}
	pkgs := make([]string, 0, len(paths))
	for _, r := range paths {
		pkgs = append(pkgs, injectLatest(r))
	}
	if flags.DryRun {
		fmt.Println(i18n.Tf(i18n.ProviderStatusDryRunInstall, map[string]any{"Cmd": "go install " + strings.Join(pkgs, " ")}))
		return nil
	}

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(baseprovider.EffectiveConfig(p.cfg, flags), flags, "goinstall install")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	for _, pkg := range pkgs {
		if err := p.runner.Install(ctx, pkg); err != nil {
			return err
		}
	}

	hf, err := baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
	if err != nil {
		return err
	}
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		hf.AddApp(tagCLI, pkg, "")
		// Cycle 207: state write matches 96/202/203/204/205/206.
		// Without this, `hams list --only=goinstall` returned empty
		// right after a successful install. goinstall has no
		// uninstall verb so there is no symmetric StateRemoved
		// branch — binaries must be removed manually.
		sf.SetResource(pkg, state.StateOK)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// statePath returns the absolute path to goinstall.state.yaml for the
// active machine. Mirrors homebrew/mas/cargo/npm/pnpm/uv.statePath.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := baseprovider.EffectiveConfig(p.cfg, flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads goinstall.state.yaml or returns a fresh
// one when the file is absent. Non-ErrNotExist load failures propagate.
func (p *Provider) loadOrCreateStateFile(flags *provider.GlobalFlags) (*state.File, error) {
	cfg := baseprovider.EffectiveConfig(p.cfg, flags)
	sf, err := state.Load(p.statePath(flags))
	if err == nil {
		return sf, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return state.New(p.Name(), cfg.MachineID), nil
	}
	return nil, fmt.Errorf("loading goinstall state %s: %w", p.statePath(flags), err)
}

// packageArgs filters positional tokens: flags (leading `-`) are
// excluded so they don't get recorded as package paths.
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
func (p *Provider) DisplayName() string { return displayName }
