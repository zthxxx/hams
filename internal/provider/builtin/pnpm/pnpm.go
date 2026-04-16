// Package pnpm wraps the pnpm package manager for global Node.js package management.
package pnpm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// cliName is the pnpm provider's manifest + CLI name.
const cliName = "pnpm"

// AutoInjectFlags are flags automatically added if not present (used
// by the HandleCommand passthrough; apply/remove paths use the
// CmdRunner which always passes -g).
var AutoInjectFlags = map[string]string{"--global": ""}

// Provider implements the pnpm package manager provider.
type Provider struct {
	runner CmdRunner
}

// New creates a new pnpm provider wired with a real CmdRunner.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(runner CmdRunner) *Provider { return &Provider{runner: runner} }

// pnpmInstallScript is the consent-gated install command. npm is the
// host (already on PATH by the time this runs, since pnpm depends on
// npm in the DAG). Extracted so unit tests can assert Script-matches-
// manifest invariants without duplicating the string.
const pnpmInstallScript = "npm install -g pnpm"

// pnpmBinaryLookup is the PATH-check seam Bootstrap uses. Swapped in
// tests to simulate "pnpm missing" / "pnpm present" without mutating
// the host's real PATH. Production value is exec.LookPath.
var pnpmBinaryLookup = exec.LookPath

// Manifest returns the pnpm provider metadata.
//
// Two DependsOn entries, each with a single purpose:
//
//   - `{Provider: "npm"}` — DAG ordering only (no Script). Ensures
//     npm is processed before pnpm across the apply pipeline.
//   - `{Provider: "bash", Script: ...}` — script host. `bash` is the
//     only provider that implements `provider.BashScriptRunner`, so
//     any DependsOn entry with a `.Script` MUST target bash; the
//     script's own invocation (here `npm install -g pnpm`) is what
//     calls into npm. Separating these avoids the conflation that
//     would otherwise make RunBootstrap type-assert an npm provider
//     to BashScriptRunner and fail.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   cliName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		DependsOn: []provider.DependOn{
			{Provider: "npm", Package: cliName},
			{Provider: "bash", Script: pnpmInstallScript},
		},
		FilePrefix: cliName,
	}
}

// Bootstrap reports whether pnpm is installed. A missing binary is
// signaled via provider.BootstrapRequiredError (which wraps
// provider.ErrBootstrapRequired); the CLI orchestrator decides whether
// to run the manifest-declared install script based on --bootstrap /
// TTY prompt. Bootstrap itself NEVER executes a network install.
//
// LookPath is delegated to the CmdRunner so unit tests can simulate
// "missing pnpm" via WithLookPathError without mutating the host's
// real PATH.
func (p *Provider) Bootstrap(_ context.Context) error {
	if err := p.runner.LookPath(); err == nil {
		return nil
	}
	return &provider.BootstrapRequiredError{
		Provider: "pnpm",
		Binary:   "pnpm",
		Script:   pnpmInstallScript,
	}
}

// Probe queries pnpm for globally installed packages.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	output, err := p.runner.List(ctx)
	if err != nil {
		return nil, err
	}

	installed := parsePnpmList(output)
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

// Plan computes actions for pnpm packages and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply installs a pnpm package globally.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("pnpm add", "package", action.ID)
	return p.runner.Install(ctx, action.ID)
}

// Remove uninstalls a pnpm package globally.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("pnpm remove", "package", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns installed packages with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for pnpm.
func (p *Provider) HandleCommand(_ context.Context, args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "add", "install", "i":
		if len(remaining) == 0 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"pnpm install requires a package name",
				"Usage: hams pnpm add <package>",
				"To install all recorded packages, use: hams apply --only=pnpm",
			)
		}
		if flags.DryRun {
			fmt.Printf("[dry-run] Would install: pnpm add -g %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "pnpm", append([]string{"add"}, remaining...), AutoInjectFlags)
	case "remove", "rm", "uninstall":
		if flags.DryRun {
			fmt.Printf("[dry-run] Would remove: pnpm remove -g %s\n", strings.Join(remaining, " "))
			return nil
		}
		return provider.WrapExecPassthrough(context.Background(), "pnpm", append([]string{"remove"}, remaining...), AutoInjectFlags)
	default:
		return provider.WrapExecPassthrough(context.Background(), "pnpm", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return cliName }

func parsePnpmList(output string) map[string]string {
	result := make(map[string]string)
	var data struct {
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return result
	}
	for name := range data.Dependencies {
		result[name] = ""
	}
	return result
}
