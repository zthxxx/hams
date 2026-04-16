// Package goinstall wraps `go install` for installing Go binaries.
package goinstall

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
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
	runner CmdRunner
}

// New creates a new go install provider wired with a real CmdRunner.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(runner CmdRunner) *Provider { return &Provider{runner: runner} }

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

// Plan computes actions for go install packages.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
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
func (p *Provider) HandleCommand(_ context.Context, args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		if len(remaining) == 0 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"goinstall requires a package path",
				"Usage: hams goinstall install <pkg[@version]>",
				"To install all recorded packages, use: hams apply --only=goinstall",
			)
		}
		pkgs := make([]string, 0, len(remaining))
		for _, r := range remaining {
			pkgs = append(pkgs, injectLatest(r))
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: go install %s\n", strings.Join(pkgs, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "go", append([]string{"install"}, pkgs...), nil)
	default:
		return provider.WrapExecPassthrough(context.Background(), "go", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return displayName }
