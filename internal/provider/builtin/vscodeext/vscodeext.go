// Package vscodeext wraps the VS Code CLI for extension management.
package vscodeext

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

const (
	// cliName is the vscodeext provider's manifest + CLI name.
	cliName = "code-ext"
	// displayName is the human-readable display name.
	displayName = "VS Code Extensions"
)

// Provider implements the VS Code extension provider.
type Provider struct{}

// New creates a new VS Code extension provider.
func New() *Provider { return &Provider{} }

// Manifest returns the vscodeext provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   displayName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		DependsOn: []provider.DependOn{
			{
				Provider: "brew",
				Package:  "visual-studio-code",
			},
		},
		FilePrefix: "vscodeext",
	}
}

// Bootstrap checks if the code CLI is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("code"); err != nil {
		return fmt.Errorf("code CLI not found in PATH; ensure VS Code is installed and 'code' is on PATH")
	}
	return nil
}

// Probe queries VS Code for installed extensions.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	cmd := exec.CommandContext(ctx, "code", "--list-extensions", "--show-versions")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("code --list-extensions: %w", err)
	}

	installed := parseExtensionList(string(output))
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}
		// Extension IDs are case-insensitive.
		lowerID := strings.ToLower(id)
		if ver, ok := installed[lowerID]; ok {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: ver})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		}
	}
	return results, nil
}

// Plan computes actions for VS Code extensions.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply installs a VS Code extension.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("code --install-extension", "extension", action.ID)
	return provider.WrapExecPassthrough(ctx, "code", []string{"--install-extension", action.ID}, nil)
}

// Remove uninstalls a VS Code extension.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("code --uninstall-extension", "extension", resourceID)
	return provider.WrapExecPassthrough(ctx, "code", []string{"--uninstall-extension", resourceID}, nil)
}

// List returns installed VS Code extensions with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for code-ext.
func (p *Provider) HandleCommand(_ context.Context, args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		if len(remaining) == 0 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"code-ext install requires an extension ID",
				"Usage: hams code-ext install <publisher.extension>",
				"To install all recorded extensions, use: hams apply --only=code-ext",
			)
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: code --install-extension %s\n", strings.Join(remaining, " "))
			return nil
		}
		installArgs := []string{}
		for _, ext := range remaining {
			installArgs = append(installArgs, "--install-extension", ext)
		}
		return provider.WrapExecPassthrough(context.Background(), "code", installArgs, nil)
	case "remove", "uninstall", "rm":
		if flags.DryRun {
			fmt.Printf("[dry-run] Would remove: code --uninstall-extension %s\n", strings.Join(remaining, " "))
			return nil
		}
		uninstallArgs := []string{}
		for _, ext := range remaining {
			uninstallArgs = append(uninstallArgs, "--uninstall-extension", ext)
		}
		return provider.WrapExecPassthrough(context.Background(), "code", uninstallArgs, nil)
	default:
		return provider.WrapExecPassthrough(context.Background(), "code", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return displayName }

// parseExtensionList parses `code --list-extensions --show-versions` output.
// Each line has the form "publisher.extension@version". Lines whose name
// is empty or contains internal whitespace are skipped — extension IDs
// cannot contain whitespace per VS Code's marketplace identity rules,
// so any such line is malformed (likely an ANSI-escape leak from CI or
// a paginator splash) and including it would corrupt the diff.
func parseExtensionList(output string) map[string]string {
	result := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "@", 2)
		name := strings.ToLower(parts[0])
		if name == "" || strings.ContainsAny(name, " \t\n\r") {
			continue
		}
		ver := ""
		if len(parts) == 2 {
			ver = parts[1]
		}
		result[name] = ver
	}
	return result
}
