// Package defaults wraps the macOS defaults command for managing application preferences.
package defaults

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

// Provider implements the macOS defaults provider.
type Provider struct{}

// New creates a new defaults provider.
func New() *Provider { return &Provider{} }

// Manifest returns the defaults provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "defaults",
		DisplayName:   "defaults",
		Platform:      provider.PlatformDarwin,
		ResourceClass: provider.ClassKVConfig,
		FilePrefix:    "defaults",
	}
}

// Bootstrap checks if defaults is available (always on macOS).
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("defaults"); err != nil {
		return fmt.Errorf("defaults not found in PATH (macOS only)")
	}
	return nil
}

// Probe reads current values for tracked defaults entries.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		domain, key := parseDomainKey(id)
		if domain == "" || key == "" {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed, ErrorMsg: "invalid format"})
			continue
		}

		cmd := exec.CommandContext(ctx, "defaults", "read", domain, key) //nolint:gosec // domain/key from state entries
		output, err := cmd.Output()
		if err != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
			continue
		}

		results = append(results, provider.ProbeResult{
			ID:    id,
			State: state.StateOK,
			Value: strings.TrimSpace(string(output)),
		})
	}
	return results, nil
}

// Plan computes actions for defaults entries.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply writes a defaults value.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	// Resource ID format: "domain.key=type:value" e.g., "com.apple.dock.autohide=bool:true"
	domain, key, typeStr, value, err := parseDefaultsResource(action.ID)
	if err != nil {
		return err
	}

	slog.Info("defaults write", "domain", domain, "key", key, "type", typeStr, "value", value)
	args := []string{"write", domain, key, "-" + typeStr, value}
	cmd := exec.CommandContext(ctx, "defaults", args...) //nolint:gosec // defaults args from hamsfile declarations
	return cmd.Run()
}

// Remove deletes a defaults key.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	domain, key := parseDomainKey(resourceID)
	slog.Info("defaults delete", "domain", domain, "key", key)
	cmd := exec.CommandContext(ctx, "defaults", "delete", domain, key) //nolint:gosec // defaults args from hamsfile declarations
	return cmd.Run()
}

// List returns defaults entries with status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-50s %-10s %s\n", id, r.State, r.Value)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for defaults.
func (p *Provider) HandleCommand(args []string, flags *cliutil.GlobalFlags) error {
	if len(args) < 3 {
		return cliutil.NewUserError(cliutil.ExitUsageError,
			"defaults requires: write <domain> <key> -<type> <value>",
			"Usage: hams defaults write com.apple.dock autohide -bool true",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: defaults %s\n", strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(context.Background(), "defaults", args...) //nolint:gosec // defaults args from CLI input
	return cmd.Run()
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "defaults" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "defaults" }

// parseDomainKey splits "domain.key" from a resource ID like "domain.key=type:value".
func parseDomainKey(resourceID string) (domain, key string) {
	id := strings.SplitN(resourceID, "=", 2)[0]
	lastDot := strings.LastIndex(id, ".")
	if lastDot < 0 {
		return "", ""
	}
	return id[:lastDot], id[lastDot+1:]
}

// parseDefaultsResource parses "domain.key=type:value".
func parseDefaultsResource(id string) (domain, key, typeStr, value string, err error) {
	parts := strings.SplitN(id, "=", 2)
	if len(parts) != 2 {
		return "", "", "", "", fmt.Errorf("defaults: resource ID must be 'domain.key=type:value', got %q", id)
	}

	domain, key = parseDomainKey(id)
	if domain == "" || key == "" {
		return "", "", "", "", fmt.Errorf("defaults: invalid domain.key in %q", id)
	}

	tv := strings.SplitN(parts[1], ":", 2)
	if len(tv) != 2 {
		return "", "", "", "", fmt.Errorf("defaults: value must be 'type:value', got %q", parts[1])
	}

	return domain, key, tv[0], tv[1], nil
}
