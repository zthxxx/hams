# Proposal: Adopt the Shared Package-Dispatcher Abstraction in Real Providers

## Why

CLAUDE.md → Current Tasks calls for shared abstractions so "extending
with a new provider is a matter of filling in a well-defined template,
not reimplementing the pattern from scratch." The codebase ships the
abstraction today (`internal/provider/package_dispatcher.go`:
`AutoRecordInstall`, `AutoRecordRemove`) but **no builtin provider
uses it** — 190 lines of untested framework code with zero adopters.

A shared abstraction with zero adopters is speculation, not engineering.
This change proves the dispatcher's contract against one real provider
(`cargo`) and extends it with the i18n-catalog routing just added in
the sibling `i18n-builtin-provider-catalog` change, making the
abstraction production-quality for further adopters.

## What Changes

- Route `AutoRecordInstall` / `AutoRecordRemove`'s remaining
  English literals through the i18n catalog
  (`UsageRequiresAtLeastOne`, `DryRunInstall`, `DryRunRemove`)
  so adopters inherit the SHALL-compliant strings for free.
- Migrate `internal/provider/builtin/cargo/` onto the dispatcher:
  - `handleInstall` / `handleRemove` delegate the lock + exec +
    hamsfile + state flow to `AutoRecord{Install,Remove}`.
  - Remove the now-dead inline `for { runner.Install }` + record loop.
  - Remove the unused `loadOrCreateHamsfile` / `loadOrCreateStateFile`
    helpers (dispatcher loads both internally).
- Document, via a spec delta, that **future Package-class providers
  SHALL go through the dispatcher unless they document why they
  can't in their own spec delta** (opt-out with visibility).

## Capabilities

### New Capabilities

None — builds on the existing `provider-system` capability.

### Modified Capabilities

- `provider-system` — add a SHALL mandating use of the shared
  dispatcher for Package-class providers going forward. Delta under
  `specs/provider-system/spec.md`.

## Impact

- Affected code:
  - `internal/provider/package_dispatcher.go` — strings routed through i18n helpers.
  - `internal/provider/builtin/cargo/cargo.go` — migrated.
  - `internal/provider/builtin/cargo/hamsfile.go` — delete unused `loadOrCreateHamsfile`.
- Affected tests:
  - Cargo's existing unit tests continue to pass unchanged (the
    dispatcher preserves semantics).
- The 8 remaining Package-class providers (`apt`, `brew`, `npm`,
  `pnpm`, `uv`, `goinstall`, `mas`, `vscodeext`) **remain on their
  inline install/uninstall loops for this change**. Each provider's
  migration is tracked as a separate change; bundling all 9 into one
  commit makes review harder and any regression in one provider
  blocks the whole batch. Reference implementation = cargo.
