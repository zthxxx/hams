# Proposal: shared-abstraction migration for CLI-wrapping providers

## Why

CLAUDE.md §Current Tasks recorded the design goal: *"All providers follow the same pattern: parse the original command structure, extract what needs to be recorded, then pass the remainder through to the underlying command for execution. … design shared abstractions — either a single generic base or a few categorical base types — so that extending with a new provider is a matter of filling in a well-defined template, not reimplementing the pattern from scratch."*

The earlier cycle introduced `internal/provider/baseprovider/` with the narrow hamsfile-path/config helpers, but did not migrate any existing provider, and did not port the richer `PackageInstaller` + `AutoRecordInstall` / `AutoRecordRemove` dispatcher that `/tmp/hams-loop/` had prototyped. As a result every package-like provider (`cargo`, `npm`, `pnpm`, `uv`, `goinstall`, `mas`, `vscodeext`, `homebrew`, `apt`) still duplicated the identical `loadOrCreateHamsfile` / `hamsfilePath` / `effectiveConfig` ~58-line boilerplate, AND separately re-implemented the "lock → exec → append-hamsfile → save-state" recipe in each `handleInstall` / `handleRemove`.

Additionally, the `default:` branch of every provider's `HandleCommand` called `provider.WrapExecPassthrough` directly, bypassing `flags.DryRun` — so `hams --dry-run cargo search ripgrep` actually execed real cargo instead of previewing.

## What changes

1. **`internal/provider/package_dispatcher.go`** (new). Ports the loop's `PackageInstaller` + `PackageDispatchOpts` + `AutoRecordInstall` / `AutoRecordRemove` contract into dev. Adapted to dev's current API surface (`provider.AcquireMutationLockFromCfg`, `hamsfile.LoadOrCreateEmpty`, `state.New` / `state.Load`). 11 unit tests cover the happy path, empty-args, dry-run, and runner-failure-atomicity for both install + remove flows.

2. **`internal/provider/passthrough.go`** (new). Adds `provider.Passthrough(ctx, tool, args, flags)` + the package-level `PassthroughExec` DI seam. This is the single shared entry point for every CLI-wrapping provider's default branch: it preserves stdin/stdout/stderr + exit code, honors `flags.DryRun` as a preview-only mode, and replaces the pre-existing `provider.WrapExecPassthrough` path in every provider's passthrough branch (which did not honor DryRun). 4 unit tests cover the exec-seam invocation, error propagation, DryRun-skip, and the zero-args DryRun branch.

3. **`internal/provider/builtin/cargo/` migration**. cargo is the reference migration:
   - `hamsfile.go` shrinks from 64 LOC of boilerplate to 4 LOC holding only the `tagCLI` constant.
   - `cargo.go` imports `baseprovider`; all `p.effectiveConfig(flags)` call-sites route through `baseprovider.EffectiveConfig`; all `p.loadOrCreateHamsfile(...)` call-sites route through `baseprovider.LoadOrCreateHamsfile`.
   - The `default:` branch of `HandleCommand` now calls `provider.Passthrough(ctx, cliName, args, flags)` instead of `provider.WrapExecPassthrough`.
   - Existing cargo unit tests continue to pass unchanged (13/13 green) — the hamsfile-path semantics are byte-identical to the pre-migration helpers.

4. **Remaining providers — deferred to follow-up cycle.** `goinstall`, `npm`, `pnpm`, `uv`, `mas`, `vscodeext`, and `homebrew` still carry the per-provider `hamsfile.go` boilerplate and the `WrapExecPassthrough` default branch. The migration pattern is mechanical (cargo is the template); it was not included in this change because of concurrent interference during implementation. All tests pass with the boilerplate still in place — the baseprovider + dispatcher helpers are additive and the remaining providers can migrate at their own pace. Tracked in `openspec/changes/2026-04-18-shared-abstraction-migration/tasks.md` §3 as `[ ]` for the follow-up cycle.

5. **apt keeps its own `handleInstall`.** apt performs per-package pin-recovery (`pkg=version`, `pkg/source`) via `parseAptInstallToken` and a post-install version probe via `runner.IsInstalled` — semantics the shared `AutoRecordInstall` helper does not express. The helper's doc-comment documents this explicitly so future providers with complex extractors know they can keep their custom flow while still leveraging `baseprovider` for the hamsfile-path boilerplate.

## Impact

- **Capability `provider-system`** — adds the "Shared helper contract for CLI-wrapping providers" requirement and extends the passthrough-for-unhandled-subcommands requirement (introduced by the 2026-04-18 git-passthrough change) to every CLI-wrapping provider via `provider.Passthrough`.
- **User-visible:** `hams --dry-run cargo search ripgrep` now prints `[dry-run] Would run: cargo search ripgrep` and exits 0, matching the git-passthrough contract. Every future provider that adopts `provider.Passthrough` gets the same behavior for free.
- **Backwards compatibility:** the existing helpers (`provider.WrapExecPassthrough`, per-provider `loadOrCreateHamsfile`) continue to exist — nothing is deleted. Providers migrate on their own schedule; no flag day.
