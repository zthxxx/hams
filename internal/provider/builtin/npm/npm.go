// Package npm wraps the npm package manager for global Node.js package management.
package npm

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

// cliName is the npm provider's manifest + CLI name.
const cliName = "npm"

// AutoInjectFlags are flags automatically added if not present.
var AutoInjectFlags = map[string]string{"--global": ""}

// Provider implements the npm package manager provider.
type Provider struct{}

// New creates a new npm provider.
func New() *Provider { return &Provider{} }

// Manifest returns the npm provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   cliName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		FilePrefix:    cliName,
	}
}

// Bootstrap checks if npm is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm not found in PATH")
	}
	return nil
}

// Probe queries npm for globally installed packages.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := exec.CommandContext(ctx, "npm", "list", "-g", "--json", "--depth=0").Output()
	if err != nil {
		return nil, fmt.Errorf("npm list: %w", err)
	}

	installed := parseNpmList(string(output))
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

// Plan computes actions for npm packages.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply installs an npm package globally.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("npm install", "package", action.ID)
	return provider.WrapExecPassthrough(ctx, "npm", []string{"install", action.ID}, AutoInjectFlags)
}

// Remove uninstalls an npm package globally.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("npm uninstall", "package", resourceID)
	return provider.WrapExecPassthrough(ctx, "npm", []string{"uninstall", resourceID}, AutoInjectFlags)
}

// List returns installed packages with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for npm.
func (p *Provider) HandleCommand(_ context.Context, args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		if len(remaining) == 0 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"npm install requires a package name",
				"Usage: hams npm install <package>",
				"To install all recorded packages, use: hams apply --only=npm",
			)
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: npm install -g %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "npm", append([]string{"install"}, remaining...), AutoInjectFlags)
	case "remove", "uninstall", "rm":
		if flags.DryRun {
			fmt.Printf("[dry-run] Would remove: npm uninstall -g %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "npm", append([]string{"uninstall"}, remaining...), AutoInjectFlags)
	default:
		return provider.WrapExecPassthrough(context.Background(), "npm", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return cliName }

func parseNpmList(output string) map[string]string {
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
