// Package uv wraps the uv tool manager for Python tool installation.
package uv

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

// Provider implements the uv tool provider.
type Provider struct {
	runner CmdRunner
}

// New creates a new uv provider wired with a real CmdRunner.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(runner CmdRunner) *Provider { return &Provider{runner: runner} }

// Manifest returns the uv provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "uv",
		DisplayName:   "uv",
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		FilePrefix:    "uv",
	}
}

// Bootstrap checks if uv is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	return p.runner.LookPath()
}

// Probe queries uv for installed tools.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := p.runner.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := parseUvToolList(output)
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

// Plan computes actions for uv tools and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply installs a uv tool.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("uv tool install", "package", action.ID)
	return p.runner.Install(ctx, action.ID)
}

// Remove uninstalls a uv tool.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("uv tool uninstall", "package", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns installed uv tools with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for uv.
func (p *Provider) HandleCommand(_ context.Context, args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		if len(remaining) == 0 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"uv install requires a tool name",
				"Usage: hams uv install <tool>",
				"To install all recorded tools, use: hams apply --only=uv",
			)
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: uv tool install %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "uv", append([]string{"tool", "install"}, remaining...), nil)
	case "remove", "uninstall", "rm":
		if flags.DryRun {
			fmt.Printf("[dry-run] Would remove: uv tool uninstall %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "uv", append([]string{"tool", "uninstall"}, remaining...), nil)
	default:
		return provider.WrapExecPassthrough(context.Background(), "uv", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "uv" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "uv" }

// parseUvToolList parses "uv tool list" output into a name→version map.
// Each line has the form: "tool-name v1.2.3".
func parseUvToolList(output string) map[string]string {
	result := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			name := parts[0]
			ver := ""
			if len(parts) >= 2 {
				ver = strings.TrimPrefix(parts[1], "v")
			}
			result[name] = ver
		}
	}
	return result
}
