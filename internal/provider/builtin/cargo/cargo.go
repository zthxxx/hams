// Package cargo wraps the Cargo package manager for Rust tool installation.
package cargo

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// Provider implements the cargo install provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new cargo provider wired with a real CmdRunner.
// cfg supplies the store/profile paths used by the CLI-first
// auto-record path (`hams cargo install <crate>` writes to the
// hamsfile after a successful passthrough). Apply-from-hamsfile
// does not read cfg — it goes through CmdRunner alone.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the cargo provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "cargo",
		DisplayName:   "cargo",
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		FilePrefix:    "cargo",
	}
}

// Bootstrap checks if cargo is available.
func (p *Provider) Bootstrap(_ context.Context) error {
	return p.runner.LookPath()
}

// Probe queries cargo for installed packages.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := p.runner.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := parseCargoList(output)
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

// Plan computes actions for cargo packages and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply installs a cargo package.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("cargo install", "package", action.ID)
	return p.runner.Install(ctx, action.ID)
}

// Remove uninstalls a cargo package.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("cargo uninstall", "package", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns installed cargo packages with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for cargo.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install", "i":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove", "uninstall", "rm":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	default:
		return provider.WrapExecPassthrough(ctx, "cargo", args, nil)
	}
}

// handleInstall runs `cargo install <crate>` via the CmdRunner seam and,
// on success, appends the crate to the cargo hamsfile so `hams apply`
// on another machine can restore it. Record is skipped on dry-run and
// on exec failure — any failure in the install loop aborts before the
// hamsfile is touched, matching the "Install exec failure leaves
// hamsfile untouched" scenario in the auto-record spec delta.
func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"cargo install requires a crate name",
			"Usage: hams cargo install <crate>",
			"To install all recorded crates, use: hams apply --only=cargo",
		)
	}
	crates := crateArgs(args)
	if len(crates) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"cargo install requires at least one crate name",
			"Usage: hams cargo install <crate>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would install: cargo install %s\n", strings.Join(args, " "))
		return nil
	}

	// Run every install first; only record once all succeed. This
	// mirrors apt's all-or-nothing auto-record semantics — partial
	// failures force the user to retry rather than leaving a mixed
	// hamsfile that drifts from the host.
	for _, crate := range crates {
		if err := p.runner.Install(ctx, crate); err != nil {
			return err
		}
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	for _, crate := range crates {
		hf.AddApp(tagCLI, crate, "")
	}
	return hf.Write()
}

// handleRemove runs `cargo uninstall <crate>` via the CmdRunner seam
// and, on success, removes the crate from the cargo hamsfile. Exec
// failure leaves the hamsfile untouched so a failed uninstall does
// not falsely de-record the crate (mirrors apt's U5 behavior).
func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"cargo remove requires a crate name",
			"Usage: hams cargo remove <crate>",
		)
	}
	crates := crateArgs(args)
	if len(crates) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"cargo remove requires at least one crate name",
			"Usage: hams cargo remove <crate>",
		)
	}
	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove: cargo uninstall %s\n", strings.Join(args, " "))
		return nil
	}

	for _, crate := range crates {
		if err := p.runner.Uninstall(ctx, crate); err != nil {
			return err
		}
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	for _, crate := range crates {
		hf.RemoveApp(crate)
	}
	return hf.Write()
}

// crateArgs filters the positional tokens from args: any token
// starting with `-` is a cargo flag and is excluded. All other tokens
// are treated as crate names. Mirrors apt.packageArgs.
func crateArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		out = append(out, a)
	}
	return out
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
