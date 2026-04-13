// Package cargo wraps the Cargo package manager for Rust tool installation.
package cargo

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/zthxxx/hams/internal/cliutil"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// Provider implements the cargo install provider.
type Provider struct{}

// New creates a new cargo provider.
func New() *Provider { return &Provider{} }

// Manifest returns the cargo provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "cargo",
		DisplayName:   "cargo",
		Platform:      provider.PlatformAll,
		ResourceClass: provider.ClassPackage,
		FilePrefix:    "cargo",
	}
}

// Bootstrap checks if cargo is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("cargo"); err != nil {
		return fmt.Errorf("cargo not found in PATH")
	}
	return nil
}

// Probe queries cargo for installed packages.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	cmd := exec.CommandContext(ctx, "cargo", "install", "--list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cargo install --list: %w", err)
	}

	installed := parseCargoList(string(output))
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

// Plan computes actions for cargo packages.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.Tags()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply installs a cargo package.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("cargo install", "package", action.ID)
	return provider.WrapExecPassthrough(ctx, "cargo", []string{"install", action.ID}, nil)
}

// Remove uninstalls a cargo package.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("cargo uninstall", "package", resourceID)
	return provider.WrapExecPassthrough(ctx, "cargo", []string{"uninstall", resourceID}, nil)
}

// List returns installed cargo packages with status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-30s %-10s %s\n", id, r.State, r.Version)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for cargo.
func (p *Provider) HandleCommand(args []string, flags *cliutil.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		if len(remaining) == 0 {
			return cliutil.NewUserError(cliutil.ExitUsageError,
				"cargo install requires a crate name",
				"Usage: hams cargo install <crate>",
				"To install all recorded crates, use: hams apply --only=cargo",
			)
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: cargo install %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "cargo", append([]string{"install"}, remaining...), nil)
	case "remove", "uninstall", "rm":
		if flags.DryRun {
			fmt.Printf("[dry-run] Would remove: cargo uninstall %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "cargo", append([]string{"uninstall"}, remaining...), nil)
	default:
		return provider.WrapExecPassthrough(context.Background(), "cargo", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "cargo" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "cargo" }

// parseCargoList parses `cargo install --list` output.
// Lines with a package have the form "name v1.2.3:".
func parseCargoList(output string) map[string]string {
	result := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, " ") {
			// Indented lines are binary names, not package names.
			continue
		}
		// Format: "crate-name v1.2.3:"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			name := parts[0]
			ver := strings.TrimSuffix(strings.TrimPrefix(parts[1], "v"), ":")
			result[name] = ver
		}
	}
	return result
}
