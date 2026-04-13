// Package apt wraps the APT package manager for Debian-based Linux distributions.
package apt

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"

	"github.com/zthxxx/hams/internal/cliutil"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
	"github.com/zthxxx/hams/internal/sudo"
)

// AutoInjectFlags auto-adds -y if not present for non-interactive installs.
var AutoInjectFlags = map[string]string{"-y": ""}

// Provider implements the APT package manager provider.
type Provider struct{}

// New creates a new apt provider.
func New() *Provider { return &Provider{} }

// Manifest returns the apt provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "apt",
		DisplayName:   "apt",
		Platform:      provider.PlatformLinux,
		ResourceClass: provider.ClassPackage,
		FilePrefix:    "apt",
	}
}

// Bootstrap checks if apt is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("apt provider is Linux-only")
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return fmt.Errorf("apt-get not found in PATH")
	}
	return nil
}

// Probe queries dpkg for installed packages.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		cmd := exec.CommandContext(ctx, "dpkg", "-s", id) //nolint:gosec // package name from state entries
		output, err := cmd.Output()
		if err != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
			continue
		}

		version := parseDpkgVersion(string(output))
		results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: version})
	}
	return results, nil
}

// Plan computes actions for apt packages.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.Tags()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply installs an apt package with sudo.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("apt install", "package", action.ID)
	cmd := sudo.RunWithSudo(ctx, "apt-get", "install", "-y", action.ID)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// Remove uninstalls an apt package with sudo.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("apt remove", "package", resourceID)
	cmd := sudo.RunWithSudo(ctx, "apt-get", "remove", "-y", resourceID)
	return cmd.Run()
}

// List returns installed packages with status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-30s %-10s %s\n", id, r.State, r.Version)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for apt.
func (p *Provider) HandleCommand(args []string, flags *cliutil.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install":
		if len(remaining) == 0 {
			return cliutil.NewUserError(cliutil.ExitUsageError,
				"apt install requires a package name",
				"Usage: hams apt install <package>",
			)
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: sudo apt-get install -y %s\n", strings.Join(remaining, " "))
			return nil
		}
		cmd := sudo.RunWithSudo(context.Background(), "apt-get", append([]string{"install", "-y"}, remaining...)...)
		return cmd.Run()
	case "remove":
		if flags.DryRun {
			fmt.Printf("[dry-run] Would remove: sudo apt-get remove -y %s\n", strings.Join(remaining, " "))
			return nil
		}
		cmd := sudo.RunWithSudo(context.Background(), "apt-get", append([]string{"remove", "-y"}, remaining...)...)
		return cmd.Run()
	default:
		return provider.WrapExecPassthrough(context.Background(), "apt-get", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "apt" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "apt" }

func parseDpkgVersion(output string) string {
	for line := range strings.SplitSeq(output, "\n") {
		if v, ok := strings.CutPrefix(line, "Version: "); ok {
			return v
		}
	}
	return ""
}
