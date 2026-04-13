// Package git implements providers for git config management and repository cloning.
package git

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

// ConfigProvider implements the git config KV provider.
type ConfigProvider struct{}

// NewConfigProvider creates a new git config provider.
func NewConfigProvider() *ConfigProvider { return &ConfigProvider{} }

// Manifest returns the git config provider metadata.
func (p *ConfigProvider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "git-config",
		DisplayName:   "git config",
		Platform:      provider.PlatformAll,
		ResourceClass: provider.ClassKVConfig,
		FilePrefix:    "git-config",
	}
}

// Bootstrap checks if git is available.
func (p *ConfigProvider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH")
	}
	return nil
}

// Probe reads current git config values for tracked resources.
func (p *ConfigProvider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		value, err := readGitConfig(ctx, id)
		if err != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed, ErrorMsg: err.Error()})
			continue
		}

		results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Value: value})
	}
	return results, nil
}

// Plan computes actions for git config entries.
func (p *ConfigProvider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.Tags()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply sets a git config value.
func (p *ConfigProvider) Apply(ctx context.Context, action provider.Action) error {
	// Resource format: "scope.section.key=value" e.g., "global.user.name=zthxxx"
	parts := strings.SplitN(action.ID, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("git config: resource ID must be 'scope.key=value', got %q", action.ID)
	}

	key := parts[0]
	value := parts[1]

	slog.Info("git config", "key", key, "value", value)
	cmd := exec.CommandContext(ctx, "git", "config", "--global", key, value) //nolint:gosec // git config args from hamsfile declarations
	return cmd.Run()
}

// Remove unsets a git config value.
func (p *ConfigProvider) Remove(ctx context.Context, resourceID string) error {
	key := strings.SplitN(resourceID, "=", 2)[0]
	slog.Info("git config --unset", "key", key)
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--unset", key) //nolint:gosec // git config key from hamsfile declarations
	return cmd.Run()
}

// List returns configured git values with status.
func (p *ConfigProvider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-40s %-10s %s\n", id, r.State, r.Value)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for git config.
func (p *ConfigProvider) HandleCommand(args []string, flags *cliutil.GlobalFlags) error {
	if len(args) < 2 {
		return cliutil.NewUserError(cliutil.ExitUsageError,
			"git-config requires key and value",
			"Usage: hams git-config <key> <value>",
			"Example: hams git-config user.name zthxxx",
		)
	}

	key := args[0]
	value := args[1]

	if flags.DryRun {
		fmt.Printf("[dry-run] Would set: git config --global %s %s\n", key, value)
		return nil
	}

	cmd := exec.CommandContext(context.Background(), "git", "config", "--global", key, value) //nolint:gosec // git config args from CLI input
	return cmd.Run()
}

// Name returns the CLI name.
func (p *ConfigProvider) Name() string { return "git-config" }

// DisplayName returns the display name.
func (p *ConfigProvider) DisplayName() string { return "git config" }

func readGitConfig(ctx context.Context, resourceID string) (string, error) {
	key := strings.SplitN(resourceID, "=", 2)[0]
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--get", key) //nolint:gosec // git config key from state entries
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git config --get %s: %w", key, err)
	}
	return strings.TrimSpace(string(output)), nil
}
