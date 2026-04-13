// Package goinstall wraps `go install` for installing Go binaries.
package goinstall

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

// Provider implements the go install provider.
type Provider struct{}

// New creates a new go install provider.
func New() *Provider { return &Provider{} }

// Manifest returns the go install provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "goinstall",
		DisplayName:   "go install",
		Platform:      provider.PlatformAll,
		ResourceClass: provider.ClassPackage,
		FilePrefix:    "goinstall",
	}
}

// Bootstrap checks if go is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go not found in PATH")
	}
	return nil
}

// Probe checks installed Go binaries by examining GOPATH/bin.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	cmd := exec.CommandContext(ctx, "go", "env", "GOPATH")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go env GOPATH: %w", err)
	}
	gopath := strings.TrimSpace(string(out))

	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		// Derive binary name from the package path (last path element, before @version).
		pkg := strings.SplitN(id, "@", 2)[0]
		parts := strings.Split(pkg, "/")
		binName := parts[len(parts)-1]
		binPath := gopath + "/bin/" + binName

		checkCmd := exec.CommandContext(ctx, binPath, "--version") //nolint:gosec // path derived from GOPATH + tracked binary name
		if err := checkCmd.Run(); err != nil {
			// Fall back to LookPath.
			if _, lookErr := exec.LookPath(binName); lookErr != nil {
				results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
				continue
			}
		}
		results = append(results, provider.ProbeResult{ID: id, State: state.StateOK})
	}
	return results, nil
}

// Plan computes actions for go install packages.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.Tags()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// injectLatest appends @latest to a resource ID if no version is specified.
func injectLatest(resourceID string) string {
	if !strings.Contains(resourceID, "@") {
		return resourceID + "@latest"
	}
	return resourceID
}

// Apply installs a Go package via go install.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	pkg := injectLatest(action.ID)
	slog.Info("go install", "package", pkg)
	return provider.WrapExecPassthrough(ctx, "go", []string{"install", pkg}, nil)
}

// Remove is a no-op for go install; binaries must be removed manually.
func (p *Provider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("go install does not support automatic removal; remove the binary manually", "package", resourceID)
	return nil
}

// List returns installed go packages with status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-50s %-10s\n", id, r.State)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for goinstall.
func (p *Provider) HandleCommand(args []string, flags *cliutil.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		if len(remaining) == 0 {
			return cliutil.NewUserError(cliutil.ExitUsageError,
				"goinstall requires a package path",
				"Usage: hams goinstall install <pkg[@version]>",
				"To install all recorded packages, use: hams apply --only=goinstall",
			)
		}
		pkgs := make([]string, 0, len(remaining))
		for _, r := range remaining {
			pkgs = append(pkgs, injectLatest(r))
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: go install %s\n", strings.Join(pkgs, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "go", append([]string{"install"}, pkgs...), nil)
	default:
		return provider.WrapExecPassthrough(context.Background(), "go", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "goinstall" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "go install" }
