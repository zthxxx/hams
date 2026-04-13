// Package mas wraps the Mac App Store CLI (mas) for macOS app management.
package mas

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

// Provider implements the Mac App Store provider.
type Provider struct{}

// New creates a new mas provider.
func New() *Provider { return &Provider{} }

// Manifest returns the mas provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "mas",
		DisplayName:   "Mac App Store",
		Platform:      provider.PlatformDarwin,
		ResourceClass: provider.ClassPackage,
		FilePrefix:    "mas",
	}
}

// Bootstrap checks if mas is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("mas"); err != nil {
		return fmt.Errorf("mas not found in PATH (macOS only; install via: brew install mas)")
	}
	return nil
}

// Probe queries mas for installed apps.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	cmd := exec.CommandContext(ctx, "mas", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mas list: %w", err)
	}

	installed := parseMasList(string(output))
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

// Plan computes actions for mas apps.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.Tags()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply installs a Mac App Store app by numeric ID.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("mas install", "app_id", action.ID)
	return provider.WrapExecPassthrough(ctx, "mas", []string{"install", action.ID}, nil)
}

// Remove uninstalls a Mac App Store app by numeric ID.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("mas uninstall", "app_id", resourceID)
	return provider.WrapExecPassthrough(ctx, "mas", []string{"uninstall", resourceID}, nil)
}

// List returns installed mas apps with status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-12s %-10s %s\n", id, r.State, r.Version)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for mas.
func (p *Provider) HandleCommand(args []string, flags *cliutil.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		if len(remaining) == 0 {
			return cliutil.NewUserError(cliutil.ExitUsageError,
				"mas install requires a numeric app ID",
				"Usage: hams mas install <app-id>",
				"To install all recorded apps, use: hams apply --only=mas",
			)
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: mas install %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "mas", append([]string{"install"}, remaining...), nil)
	case "remove", "uninstall", "rm":
		if flags.DryRun {
			fmt.Printf("[dry-run] Would remove: mas uninstall %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "mas", append([]string{"uninstall"}, remaining...), nil)
	default:
		return provider.WrapExecPassthrough(context.Background(), "mas", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "mas" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "Mac App Store" }

// parseMasList parses `mas list` output into an appID→version map.
// Each line has the form: "1234567890  App Name (version)".
func parseMasList(output string) map[string]string {
	result := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}
		appID := parts[0]
		ver := ""
		// Version appears in parentheses at the end: "App Name (1.2.3)".
		if len(parts) >= 2 {
			last := parts[len(parts)-1]
			if strings.HasPrefix(last, "(") && strings.HasSuffix(last, ")") {
				ver = strings.Trim(last, "()")
			}
		}
		result[appID] = ver
	}
	return result
}
