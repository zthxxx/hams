// Package duti wraps the duti command for managing default app associations on macOS.
package duti

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

// Provider implements the duti default-app association provider.
type Provider struct{}

// New creates a new duti provider.
func New() *Provider { return &Provider{} }

// Manifest returns the duti provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "duti",
		DisplayName:   "duti",
		Platform:      provider.PlatformDarwin,
		ResourceClass: provider.ClassKVConfig,
		FilePrefix:    "duti",
	}
}

// Bootstrap checks if duti is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("duti"); err != nil {
		return fmt.Errorf("duti not found in PATH (macOS only; install via: brew install duti)")
	}
	return nil
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

		cmd := exec.CommandContext(ctx, "duti", "-x", ext) //nolint:gosec // ext from tracked state entries
		output, cmdErr := cmd.Output()
		if cmdErr != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
			continue
		}

		currentApp := parseDutiOutput(string(output))
		results = append(results, provider.ProbeResult{
			ID:    id,
			State: state.StateOK,
			Value: currentApp,
		})
	}
	return results, nil
}

// Plan computes actions for duti associations.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply sets a default app association via duti.
// Resource ID format: "<ext>=<bundle-id>" e.g. "pdf=com.adobe.acrobat.pdf".
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	ext, bundleID, err := parseResourceID(action.ID)
	if err != nil {
		return err
	}
	slog.Info("duti set", "ext", ext, "bundle_id", bundleID)
	cmd := exec.CommandContext(ctx, "duti", "-s", bundleID, "."+ext, "all") //nolint:gosec // duti args from hamsfile declarations
	return cmd.Run()
}

// Remove is a no-op for duti; macOS does not have a direct "reset to default" command.
func (p *Provider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("duti does not support automatic removal; reset the association manually via System Settings", "resource", resourceID)
	return nil
}

// List returns duti associations with status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-30s %-10s %s\n", id, r.State, r.Value)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for duti.
func (p *Provider) HandleCommand(args []string, flags *cliutil.GlobalFlags) error {
	if len(args) == 0 {
		return cliutil.NewUserError(cliutil.ExitUsageError,
			"duti requires arguments",
			"Usage: hams duti <ext>=<bundle-id>",
			"Example: hams duti pdf=com.adobe.acrobat.pdf",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: duti %s\n", strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(context.Background(), "duti", args...) //nolint:gosec // duti args from CLI input
	return cmd.Run()
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "duti" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "duti" }

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
