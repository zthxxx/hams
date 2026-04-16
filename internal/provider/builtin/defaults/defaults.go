// Package defaults wraps the macOS defaults command for managing application preferences.
package defaults

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// cliName is the macOS `defaults` CLI binary and provider name.
const cliName = "defaults"

// Provider implements the macOS defaults provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new defaults provider wired with a real CmdRunner.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the defaults provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   cliName,
		Platforms:     []provider.Platform{provider.PlatformDarwin},
		ResourceClass: provider.ClassKVConfig,
		FilePrefix:    cliName,
	}
}

// Bootstrap checks if defaults is available (always on macOS).
func (p *Provider) Bootstrap(_ context.Context) error {
	return p.runner.LookPath()
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

		value, err := p.runner.Read(ctx, domain, key)
		if err != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
			continue
		}

		results = append(results, provider.ProbeResult{
			ID:    id,
			State: state.StateOK,
			Value: value,
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
	return p.runner.Write(ctx, domain, key, typeStr, value)
}

// Remove deletes a defaults key.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	domain, key := parseDomainKey(resourceID)
	slog.Info("defaults delete", "domain", domain, "key", key)
	return p.runner.Delete(ctx, domain, key)
}

// List returns defaults entries with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for defaults.
func (p *Provider) HandleCommand(_ context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) < 3 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"defaults requires: write <domain> <key> -<type> <value>",
			"Usage: hams defaults write com.apple.dock autohide -bool true",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: defaults %s\n", strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(context.Background(), cliName, args...) //nolint:gosec // defaults args from CLI input
	if err := cmd.Run(); err != nil {
		return err
	}

	if len(args) >= 5 && args[0] == "write" {
		p.recordPreviewCmd(args, hamsFlags, flags)
	}

	return nil
}

// recordPreviewCmd saves the preview-cmd field to the hamsfile for a defaults write.
func (p *Provider) recordPreviewCmd(args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) {
	if p.cfg == nil || p.cfg.StorePath == "" {
		return
	}

	domain := args[1]
	key := args[2]
	typeStr := strings.TrimPrefix(args[3], "-")
	value := args[4]
	resourceID := fmt.Sprintf("%s.%s=%s:%s", domain, key, typeStr, value)
	previewCmd := "defaults " + strings.Join(args, " ")

	suffix := ".hams.yaml"
	if _, ok := hamsFlags["local"]; ok {
		suffix = ".hams.local.yaml"
	}

	cfg := p.effectiveConfig(flags)
	path := filepath.Join(cfg.ProfileDir(), cliName+suffix)
	hf, err := hamsfile.Read(path)
	if err != nil {
		slog.Debug("could not load hamsfile for preview-cmd", "path", path, "error", err)
		return
	}

	hf.SetPreviewCmd(resourceID, previewCmd)
	if writeErr := hf.Write(); writeErr != nil {
		slog.Debug("could not save preview-cmd", "path", path, "error", writeErr)
	}
}

// effectiveConfig returns the config with flag overrides applied.
func (p *Provider) effectiveConfig(flags *provider.GlobalFlags) *config.Config {
	if p.cfg == nil {
		p.cfg = &config.Config{}
	}
	cfg := *p.cfg
	if flags == nil {
		return &cfg
	}
	if flags.Store != "" {
		cfg.StorePath = flags.Store
	}
	if flags.Profile != "" {
		cfg.ProfileTag = flags.Profile
	}
	return &cfg
}

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return cliName }

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
