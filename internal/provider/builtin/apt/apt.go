// Package apt wraps the APT package manager for Debian-based Linux distributions.
package apt

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// AutoInjectFlags auto-adds -y if not present for non-interactive installs.
var AutoInjectFlags = map[string]string{"-y": ""}

// Provider implements the APT package manager provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new apt provider wired with a real CmdRunner.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the apt provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "apt",
		DisplayName:   "apt",
		Platforms:     []provider.Platform{provider.PlatformLinux},
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

		installed, version, err := p.runner.IsInstalled(ctx, id)
		if err != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
			continue
		}
		if !installed {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
			continue
		}
		results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Version: version})
	}
	return results, nil
}

// Plan computes actions for apt packages, then overlays drift detection
// for version-pinned resources: if state has a `requested_version` that
// doesn't match the observed dpkg version, promote the action to Update
// and rewrite its ID to the pinned form (`pkg=version`) so the executor
// re-invokes apt-get with the pin. Source-pinned resources promote
// similarly using `pkg/source`.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)

	for i, a := range actions {
		if a.Type != provider.ActionSkip {
			continue
		}
		r, ok := observed.Resources[a.ID]
		if !ok {
			continue
		}
		switch {
		case r.RequestedVersion != "" && r.Version != r.RequestedVersion:
			actions[i].Type = provider.ActionUpdate
			actions[i].ID = a.ID + "=" + r.RequestedVersion
		case r.RequestedSource != "" && r.Version == "":
			actions[i].Type = provider.ActionUpdate
			actions[i].ID = a.ID + "/" + r.RequestedSource
		}
	}
	return actions, nil
}

// Apply installs an apt package.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("apt install", "package", action.ID)
	return p.runner.Install(ctx, []string{action.ID})
}

// Remove uninstalls an apt package.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("apt remove", "package", resourceID)
	return p.runner.Remove(ctx, []string{resourceID})
}

// List returns installed packages with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for apt.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	default:
		return provider.WrapExecPassthrough(ctx, "apt-get", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "apt" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "apt" }

func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"apt install requires a package name",
			"Usage: hams apt install <package>",
		)
	}

	packages := packageArgs(args)
	if len(packages) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"apt install requires at least one package name",
			"Usage: hams apt install <package>",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would install: sudo apt-get install -y %s\n", strings.Join(args, " "))
		return nil
	}

	// Forward args verbatim so passthrough flags (e.g. --no-install-recommends)
	// reach apt-get AND the multi-package install runs as one transaction
	// (apt-get errors atomically if any package fails dependency resolution).
	if err := p.runner.Install(ctx, args); err != nil {
		return err
	}

	// Dry-run flags executed apt-get but didn't change host state, so
	// post-hoc bookkeeping cannot represent the invocation truthfully.
	// Skip the auto-record entirely and warn the user.
	if isComplexAptInvocation(args) {
		slog.Warn("hams apt install completed but did not auto-record (dry-run flag detected). To declare these resources, edit the apt hamsfile and run `hams apply`.", "args", args)
		return nil
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}

	sf := p.loadOrCreateStateFile(flags)

	for _, raw := range args {
		pkg, requestedVersion, requestedSource := parseAptInstallToken(raw)
		if pkg == "" {
			continue
		}
		// AddAppWithFields is a no-op on duplicate append at the YAML
		// level, so guard with FindApp to keep the hamsfile idempotent.
		if existingTag, _ := hf.FindApp(pkg); existingTag == "" {
			extra := map[string]string{
				"version": requestedVersion,
				"source":  requestedSource,
			}
			hf.AddAppWithFields(tagCLI, pkg, "", extra)
		}
		_, observed, probeErr := p.runner.IsInstalled(ctx, pkg)
		if probeErr != nil {
			slog.Warn("post-install version probe failed", "package", pkg, "error", probeErr)
		}
		opts := []state.ResourceOption{state.WithVersion(observed)}
		if requestedVersion != "" {
			opts = append(opts, state.WithRequestedVersion(requestedVersion))
		}
		if requestedSource != "" {
			opts = append(opts, state.WithRequestedSource(requestedSource))
		}
		sf.SetResource(pkg, state.StateOK, opts...)
	}

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"apt remove requires a package name",
			"Usage: hams apt remove <package>",
		)
	}

	packages := packageArgs(args)
	if len(packages) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"apt remove requires at least one package name",
			"Usage: hams apt remove <package>",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove: sudo apt-get remove -y %s\n", strings.Join(args, " "))
		return nil
	}

	// Forward args verbatim — preserves passthrough flags (e.g. --purge) and
	// runs the multi-package remove as one transaction.
	if err := p.runner.Remove(ctx, args); err != nil {
		return err
	}

	// Same dry-run guard as install: dry-run flags execute apt-get but
	// don't change host state, so the remove bookkeeping would lie.
	if isComplexAptInvocation(args) {
		slog.Warn("hams apt remove completed but did not auto-record (dry-run flag detected). To declare these resources, edit the apt hamsfile and run `hams apply`.", "args", args)
		return nil
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}

	sf := p.loadOrCreateStateFile(flags)

	for _, pkg := range packages {
		hf.RemoveApp(pkg)
		sf.SetResource(pkg, state.StateRemoved)
	}

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// statePath returns the absolute path to apt.state.yaml for the active machine.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads the apt state file or returns a fresh one when
// the file is absent or unreadable. Mirrors the lossy-on-error pattern used
// by internal/provider/probe.go's loadOrCreateState helper.
func (p *Provider) loadOrCreateStateFile(flags *provider.GlobalFlags) *state.File {
	cfg := p.effectiveConfig(flags)
	sf, err := state.Load(p.statePath(flags))
	if err != nil {
		sf = state.New(p.Name(), cfg.MachineID)
	}
	return sf
}

// packageArgs filters out flag-looking arguments so that passthrough flags
// (e.g., --no-install-recommends) do not get treated as package names when
// adding entries to the hamsfile.
func packageArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// aptDryRunFlags lists apt-get flags that mean "don't actually install" —
// commands that succeed without changing the package set on disk. The
// auto-record path refuses to bookkeep these because dpkg-state cannot
// distinguish "this invocation installed it" from "it was already there".
var aptDryRunFlags = map[string]bool{
	"--download-only": true,
	"--simulate":      true,
	"-s":              true,
	"--just-print":    true,
	"--no-act":        true,
	"--recon":         true,
}

// isComplexAptInvocation returns true if any arg invokes apt-get in a
// mode where post-hoc bookkeeping cannot determine what this invocation
// did. Currently only dry-run flags trip this — they execute apt-get
// without changing host state, so neither hamsfile nor state writes
// would be meaningful.
//
// Version pinning (`pkg=version`) and release pinning (`pkg/release`)
// are NOT complex anymore: parseAptInstallToken recovers the structured
// pin and the bookkeeping loop records it as a structured hamsfile
// entry + state row. See `apt-cli-complex-invocations` archive.
func isComplexAptInvocation(args []string) bool {
	for _, a := range args {
		if aptDryRunFlags[a] {
			return true
		}
	}
	return false
}

// debianPkgName matches a valid Debian package name per Policy §5.6.7:
// must start with [a-z0-9] and contain only [a-z0-9.+-]. This guards
// the parser against arg tokens that LOOK like `pkg=value` but are
// really apt option values (e.g., `Debug::NoLocking=true` from `-o`)
// — those don't match the regex and get rejected.
var debianPkgName = regexp.MustCompile(`^[a-z0-9][a-z0-9+\-.]*$`)

// parseAptInstallToken splits a single install arg into (pkg, version,
// source). Recognized forms:
//
//   - `nginx`               → ("nginx", "", "")
//   - `nginx=1.24.0`        → ("nginx", "1.24.0", "")
//   - `nginx/bookworm-bp`   → ("nginx", "", "bookworm-bp")
//   - flag-prefixed (`-y`)  → ("", "", "")  (caller filters first)
//   - non-package token     → ("", "", "")  (e.g., `Debug::NoLocking=true`
//     from an apt `-o` value)
//
// The combined `pkg=ver/release` form that apt-get accepts is rare and
// out of scope; it parses as ("pkg", "ver/release", "") which is
// unhelpful but harmless — the version pin still flows through to
// apt-get on re-install.
func parseAptInstallToken(arg string) (pkg, version, source string) {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return "", "", ""
	}
	if name, ver, ok := strings.Cut(arg, "="); ok {
		if !debianPkgName.MatchString(name) {
			return "", "", ""
		}
		return name, ver, ""
	}
	if name, src, ok := strings.Cut(arg, "/"); ok {
		if !debianPkgName.MatchString(name) {
			return "", "", ""
		}
		return name, "", src
	}
	if !debianPkgName.MatchString(arg) {
		return "", "", ""
	}
	return arg, "", ""
}

func parseDpkgVersion(output string) string {
	for line := range strings.SplitSeq(output, "\n") {
		if v, ok := strings.CutPrefix(line, "Version: "); ok {
			return v
		}
	}
	return ""
}
