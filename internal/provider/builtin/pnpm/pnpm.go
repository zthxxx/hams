// Package pnpm wraps the pnpm package manager for global Node.js package management.
package pnpm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// AutoInjectFlags are flags automatically added if not present.
var AutoInjectFlags = map[string]string{"--global": ""}

// Provider implements the pnpm package manager provider.
type Provider struct{}

// New creates a new pnpm provider.
func New() *Provider { return &Provider{} }

// Manifest returns the pnpm provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "pnpm",
		DisplayName:   "pnpm",
		Platform:      provider.PlatformAll,
		ResourceClass: provider.ClassPackage,
		DependsOn: []provider.DependOn{
			{Provider: "npm", Package: "pnpm"},
		},
		FilePrefix: "pnpm",
	}
}

// Bootstrap checks if pnpm is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("pnpm"); err != nil {
		return fmt.Errorf("pnpm not found in PATH; install via: npm install -g pnpm")
	}
	return nil
}

// Probe queries pnpm for globally installed packages.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := exec.CommandContext(ctx, "pnpm", "list", "-g", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("pnpm list: %w", err)
	}

	installed := parsePnpmList(string(output))
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

// Plan computes actions for pnpm packages.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply installs a pnpm package globally.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("pnpm add", "package", action.ID)
	return provider.WrapExecPassthrough(ctx, "pnpm", []string{"add", action.ID}, AutoInjectFlags)
}

// Remove uninstalls a pnpm package globally.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("pnpm remove", "package", resourceID)
	return provider.WrapExecPassthrough(ctx, "pnpm", []string{"remove", resourceID}, AutoInjectFlags)
}

// List returns installed packages with status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-30s %-10s %s\n", id, r.State, r.Version)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for pnpm.
func (p *Provider) HandleCommand(args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "add", "install", "i":
		if len(remaining) == 0 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"pnpm install requires a package name",
				"Usage: hams pnpm add <package>",
				"To install all recorded packages, use: hams apply --only=pnpm",
			)
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: pnpm add -g %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "pnpm", append([]string{"add"}, remaining...), AutoInjectFlags)
	case "remove", "rm", "uninstall":
		if flags.DryRun {
			fmt.Printf("[dry-run] Would remove: pnpm remove -g %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "pnpm", append([]string{"remove"}, remaining...), AutoInjectFlags)
	default:
		return provider.WrapExecPassthrough(context.Background(), "pnpm", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "pnpm" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "pnpm" }

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
