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
		hf.AddApp(opts.HamsTag, pkg, "")
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
