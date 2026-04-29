package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// PackageInstaller is the narrow interface the shared package-provider
// dispatcher needs from a provider's CmdRunner. Every package-like
// builtin already exposes Install / Uninstall methods on its runner;
// wrapping them via this interface lets multiple providers share the
// AutoRecordInstall / AutoRecordRemove helpers below without each
// provider reinventing the "lock → exec → record" boilerplate.
type PackageInstaller interface {
	Install(ctx context.Context, pkg string) error
	Uninstall(ctx context.Context, pkg string) error
}

// PackageDispatchOpts bundles the per-provider identity that the
// shared helpers need to build error messages, lock names, and
// hamsfile/state writes.
type PackageDispatchOpts struct {
	// CLIName is the provider's `hams <name> …` verb, e.g. "cargo".
	// Used in the dry-run preview and lock label only.
	CLIName string
	// InstallVerb is the native tool's install sub-verb, e.g.
	// "install" for cargo / apt, "add" for pnpm. Appears in the
	// dry-run preview.
	InstallVerb string
	// RemoveVerb mirrors InstallVerb for the uninstall side.
	RemoveVerb string
	// HamsTag is the tag under which records are recorded in the
	// hamsfile. Conventionally "cli".
	HamsTag string

	// IntroFn, when non-nil, is called for each successfully-installed
	// package to fetch a short human-readable description that is
	// then written to the hamsfile's `intro:` field. Providers that
	// have a cheap native metadata source (brew's `desc`, apt's
	// description, npm's `description`, ...) SHOULD populate this so
	// users get a self-explanatory Hamsfile instead of bare `- app:`
	// entries. Errors MUST be swallowed inside the closure — a
	// failed lookup falls back to "" and the install still succeeds.
	// An empty return is the documented "no intro available" signal.
	IntroFn func(ctx context.Context, pkg string) string
}

// AutoRecordInstall is the shared package-provider install flow.
// Every builtin that wraps a package-manager's install verb (apt,
// brew, pnpm, npm, cargo, goinstall, uv, mas, vscodeext, …) needs
// this same sequence:
//
//  1. Dry-run short-circuit (print "[dry-run] Would install: …").
//  2. Single-writer state lock.
//  3. Call runner.Install per package, failing fast on first error.
//  4. Load hamsfile + state, append each package, write both.
//
// Today every provider inlines steps 2–4. Collapsing them behind a
// single function means:
//
//   - Adding a new package provider is writing an extractor (what
//     does my CLI call a package name?) + a constructor. No
//     re-implementation of the lock / auto-record dance.
//   - Bugs fixed in the shared helper (e.g. the cycle-96
//     state-write omission) propagate to every provider without a
//     per-provider patch.
//
// Callers still OWN:
//
//   - Argument extraction (what does `cargo install ripgrep` vs.
//     `npm install -g foo bar` parse into?).
//   - Complex-invocation detection (apt-get --simulate, brew --cask,
//     etc. — these change what to record).
//   - Any post-install probe (e.g. apt's version-fetch after
//     install).
//
// The helper exposes three small hooks rather than one monolith:
// see DispatchPackageInstall / DispatchPackageRemove.
func AutoRecordInstall(
	ctx context.Context,
	runner PackageInstaller,
	pkgs []string,
	cfg *config.Config,
	flags *GlobalFlags,
	hfPath, statePath string,
	opts PackageDispatchOpts,
) error {
	if len(pkgs) == 0 {
		return UsageRequiresAtLeastOne(opts.CLIName, opts.InstallVerb, "package name", "package")
	}
	if flags.DryRun {
		DryRunInstall(flags, opts.CLIName+" "+opts.InstallVerb+" "+strings.Join(pkgs, " "))
		return nil
	}

	release, lockErr := AcquireMutationLockFromCfg(cfg, flags, opts.CLIName+" "+opts.InstallVerb)
	if lockErr != nil {
		return lockErr
	}
	defer release()

	for _, pkg := range pkgs {
		slog.Info(opts.CLIName+" "+opts.InstallVerb, "package", pkg)
		if err := runner.Install(ctx, pkg); err != nil {
			return err
		}
	}

	hf, err := hamsfile.LoadOrCreateEmpty(hfPath)
	if err != nil {
		return fmt.Errorf("loading hamsfile %s: %w", hfPath, err)
	}
	sf := state.New(opts.CLIName, cfg.MachineID)
	if loaded, loadErr := state.Load(statePath); loadErr == nil {
		sf = loaded
	}
	for _, pkg := range pkgs {
		intro := ""
		if opts.IntroFn != nil {
			intro = opts.IntroFn(ctx, pkg)
		}
		hf.AddApp(opts.HamsTag, pkg, intro)
		sf.SetResource(pkg, state.StateOK)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return fmt.Errorf("writing hamsfile %s: %w", hfPath, writeErr)
	}
	if saveErr := sf.Save(statePath); saveErr != nil {
		return fmt.Errorf("saving state %s: %w", statePath, saveErr)
	}
	return nil
}

// AutoRecordInstallFn is the closure-based variant of AutoRecordInstall.
// Use it for Package-class providers whose runner.Install signature
// carries extra arguments the `PackageInstaller` interface cannot
// capture (e.g., homebrew's `Install(ctx, pkg, isCask bool)` where the
// cask flag is derived from the raw args before the install loop).
//
// The caller supplies `installFn` — a per-package closure that curries
// in any extra context (isCask, flags, per-pkg options) before
// delegating to the real runner. The dispatcher handles the rest of
// the shared "validate → dry-run → lock → loop → load → record → save"
// sequence identically to AutoRecordInstall.
//
// Use AutoRecordInstall (not this function) when the runner's Install
// signature matches the PackageInstaller interface — that's the
// simpler call site and doesn't require a closure allocation.
//
// Follow-up 5.8 from 2026-04-18-provider-shared-abstraction-adoption:
// homebrew was previously exempt because its Install carried isCask;
// AutoRecordInstallFn lifts that exemption.
func AutoRecordInstallFn(
	ctx context.Context,
	installFn func(ctx context.Context, pkg string) error,
	pkgs []string,
	cfg *config.Config,
	flags *GlobalFlags,
	hfPath, statePath string,
	opts PackageDispatchOpts,
) error {
	if len(pkgs) == 0 {
		return UsageRequiresAtLeastOne(opts.CLIName, opts.InstallVerb, "package name", "package")
	}
	if flags.DryRun {
		DryRunInstall(flags, opts.CLIName+" "+opts.InstallVerb+" "+strings.Join(pkgs, " "))
		return nil
	}

	release, lockErr := AcquireMutationLockFromCfg(cfg, flags, opts.CLIName+" "+opts.InstallVerb)
	if lockErr != nil {
		return lockErr
	}
	defer release()

	for _, pkg := range pkgs {
		slog.Info(opts.CLIName+" "+opts.InstallVerb, "package", pkg)
		if err := installFn(ctx, pkg); err != nil {
			return err
		}
	}

	hf, err := hamsfile.LoadOrCreateEmpty(hfPath)
	if err != nil {
		return fmt.Errorf("loading hamsfile %s: %w", hfPath, err)
	}
	sf := state.New(opts.CLIName, cfg.MachineID)
	if loaded, loadErr := state.Load(statePath); loadErr == nil {
		sf = loaded
	}
	for _, pkg := range pkgs {
		intro := ""
		if opts.IntroFn != nil {
			intro = opts.IntroFn(ctx, pkg)
		}
		hf.AddApp(opts.HamsTag, pkg, intro)
		sf.SetResource(pkg, state.StateOK)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return fmt.Errorf("writing hamsfile %s: %w", hfPath, writeErr)
	}
	if saveErr := sf.Save(statePath); saveErr != nil {
		return fmt.Errorf("saving state %s: %w", statePath, saveErr)
	}
	return nil
}

// AutoRecordRemoveFn is the closure-based variant of AutoRecordRemove.
// Use it when a provider's remove flow dispatches differently per
// package (e.g., homebrew routes tap-format IDs to `runner.Untap` and
// ordinary formulae to `runner.Uninstall` — one static interface
// method cannot express that branching, but a closure can).
func AutoRecordRemoveFn(
	ctx context.Context,
	uninstallFn func(ctx context.Context, pkg string) error,
	pkgs []string,
	cfg *config.Config,
	flags *GlobalFlags,
	hfPath, statePath string,
	opts PackageDispatchOpts,
) error {
	if len(pkgs) == 0 {
		return UsageRequiresAtLeastOne(opts.CLIName, opts.RemoveVerb, "package name", "package")
	}
	if flags.DryRun {
		DryRunRemove(flags, opts.CLIName+" "+opts.RemoveVerb+" "+strings.Join(pkgs, " "))
		return nil
	}

	release, lockErr := AcquireMutationLockFromCfg(cfg, flags, opts.CLIName+" "+opts.RemoveVerb)
	if lockErr != nil {
		return lockErr
	}
	defer release()

	for _, pkg := range pkgs {
		slog.Info(opts.CLIName+" "+opts.RemoveVerb, "package", pkg)
		if err := uninstallFn(ctx, pkg); err != nil {
			return err
		}
	}

	hf, err := hamsfile.LoadOrCreateEmpty(hfPath)
	if err != nil {
		return fmt.Errorf("loading hamsfile %s: %w", hfPath, err)
	}
	sf := state.New(opts.CLIName, cfg.MachineID)
	if loaded, loadErr := state.Load(statePath); loadErr == nil {
		sf = loaded
	}
	for _, pkg := range pkgs {
		hf.RemoveApp(pkg)
		sf.SetResource(pkg, state.StateRemoved)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return fmt.Errorf("writing hamsfile %s: %w", hfPath, writeErr)
	}
	if saveErr := sf.Save(statePath); saveErr != nil {
		return fmt.Errorf("saving state %s: %w", statePath, saveErr)
	}
	return nil
}

// AutoRecordRemove mirrors AutoRecordInstall for the uninstall
// flow. Removes each named package via runner.Uninstall, then
// deletes the corresponding entry from the hamsfile and tombstones
// it in the state file (state=removed, removed_at=now).
//
// Dry-run short-circuits, just like AutoRecordInstall; the lock +
// runner-exec + record sequence is byte-symmetric between the two
// flows.
func AutoRecordRemove(
	ctx context.Context,
	runner PackageInstaller,
	pkgs []string,
	cfg *config.Config,
	flags *GlobalFlags,
	hfPath, statePath string,
	opts PackageDispatchOpts,
) error {
	if len(pkgs) == 0 {
		return UsageRequiresAtLeastOne(opts.CLIName, opts.RemoveVerb, "package name", "package")
	}
	if flags.DryRun {
		DryRunRemove(flags, opts.CLIName+" "+opts.RemoveVerb+" "+strings.Join(pkgs, " "))
		return nil
	}

	release, lockErr := AcquireMutationLockFromCfg(cfg, flags, opts.CLIName+" "+opts.RemoveVerb)
	if lockErr != nil {
		return lockErr
	}
	defer release()

	for _, pkg := range pkgs {
		slog.Info(opts.CLIName+" "+opts.RemoveVerb, "package", pkg)
		if err := runner.Uninstall(ctx, pkg); err != nil {
			return err
		}
	}

	hf, err := hamsfile.LoadOrCreateEmpty(hfPath)
	if err != nil {
		return fmt.Errorf("loading hamsfile %s: %w", hfPath, err)
	}
	sf := state.New(opts.CLIName, cfg.MachineID)
	if loaded, loadErr := state.Load(statePath); loadErr == nil {
		sf = loaded
	}
	for _, pkg := range pkgs {
		hf.RemoveApp(pkg)
		sf.SetResource(pkg, state.StateRemoved)
	}
	if writeErr := hf.Write(); writeErr != nil {
		return fmt.Errorf("writing hamsfile %s: %w", hfPath, writeErr)
	}
	if saveErr := sf.Save(statePath); saveErr != nil {
		return fmt.Errorf("saving state %s: %w", statePath, saveErr)
	}
	return nil
}
