# Design: Shared Package-Dispatcher Adoption

## Context

`internal/provider/package_dispatcher.go` exposes:

- `PackageInstaller` interface — `Install(ctx, pkg string) error` + `Uninstall(ctx, pkg string) error`.
- `PackageDispatchOpts` struct — CLIName / InstallVerb / RemoveVerb / HamsTag.
- `AutoRecordInstall` + `AutoRecordRemove` — bundle the "validate → dry-run → lock → loop exec → load hamsfile+state → record → save" flow.

Seven of the 14 builtin providers have runners whose signatures match
`PackageInstaller` verbatim: cargo, npm, pnpm, uv, goinstall, mas,
vscodeext. The remaining providers either batch-install (apt takes a
slice), carry extra flags (brew's `isCask` bool), or don't install at
all (bash, git, duti, defaults, ansible).

Cargo is the simplest of the matching set and the best candidate for a
reference adopter: its runner signature is a perfect match, its
install/remove loops are textbook, and it already went through the
`i18n` wiring in the sibling change so any ripple effects there are
already absorbed.

## Goals

1. Prove `package_dispatcher` in production: one adopter, identical
   semantics pre- vs. post-migration.
2. Wire the dispatcher's remaining literal English strings through i18n
   so future adopters inherit SHALL compliance.
3. Ship a spec delta that requires future Package-class providers to
   use the dispatcher by default.

## Non-Goals

- Migrating the other 6 matching-signature providers in this change.
  Bundled migrations hide regressions; each provider's migration gets
  its own small atomic commit.
- Refactoring the dispatcher to accept variadic or batch-install
  signatures (apt, brew). Those need a second dispatcher variant,
  tracked separately.

## Shape After Cargo Migration

Before (cargo.handleInstall):

```go
func (p *Provider) handleInstall(ctx, args, hamsFlags, flags) error {
    if len(args) == 0 { … UsageRequiresResource … }
    crates := crateArgs(args)
    if len(crates) == 0 { … UsageRequiresAtLeastOne … }
    if flags.DryRun { … DryRunInstall … }

    release, lockErr := provider.AcquireMutationLockFromCfg(…)
    if lockErr != nil { return lockErr }
    defer release()

    for _, crate := range crates {
        if err := p.runner.Install(ctx, crate); err != nil { return err }
    }
    hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
    if err != nil { return err }
    sf, err := p.loadOrCreateStateFile(flags)
    if err != nil { return err }
    for _, crate := range crates {
        hf.AddApp(tagCLI, crate, "")
        sf.SetResource(crate, state.StateOK)
    }
    if writeErr := hf.Write(); writeErr != nil { return writeErr }
    return sf.Save(p.statePath(flags))
}
```

After:

```go
func (p *Provider) handleInstall(ctx, args, hamsFlags, flags) error {
    if len(args) == 0 { … UsageRequiresResource … }
    crates := crateArgs(args)
    if len(crates) == 0 { … UsageRequiresAtLeastOne … }

    hfPath, hfErr := p.hamsfilePath(hamsFlags, flags)
    if hfErr != nil { return hfErr }
    return provider.AutoRecordInstall(ctx, p.runner, crates,
        p.effectiveConfig(flags), flags,
        hfPath, p.statePath(flags),
        provider.PackageDispatchOpts{
            CLIName:     cliName,
            InstallVerb: "install",
            RemoveVerb:  "uninstall",
            HamsTag:     tagCLI,
        },
    )
}
```

27 lines → 13 lines. Every line removed is a line a future provider
does NOT need to re-derive.

## Test Plan

1. Existing cargo unit tests pass unchanged (the dispatcher's flow is
   byte-identical to what cargo inlined before).
2. `go test -race ./internal/provider/builtin/cargo/...` green.
3. `task check` green.
4. Manual smoke: `hams cargo install ripgrep --dry-run` shows
   `[dry-run] Would install: cargo install ripgrep` (dispatcher's
   preview format).

## Alternatives Considered

**A. Migrate all 7 matching-signature providers in one change.**
Faster in aggregate but any regression in one provider blocks the
whole commit — antagonistic to CLAUDE.md's "frequent atomic commits"
rule. Rejected.

**B. Refactor dispatcher to accept batch-install shape (apt).**
Out of scope. Would require either variadic signatures or a second
dispatcher (BatchInstaller interface). Tracked separately.

**Chosen: cargo-only proof + spec delta** (this doc).
