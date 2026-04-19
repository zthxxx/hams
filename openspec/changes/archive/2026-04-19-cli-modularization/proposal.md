# CLI modularization

## Why

Per §9.1/§9.2 of `docs/notes/dev-vs-loop-implementation-analysis.md`,
`origin/dev` ships three CLI-package improvements that `local/loop`
lacks:

1. **Auto-init logic in a dedicated `autoinit.go`** rather than
   inlined across `apply.go`, `commands.go` and `provider_cmd.go`.
   Same behaviour, better single-responsibility.
2. **Dedicated auto-init test coverage** — currently 2 tests
   (`apply_autoinit_test.go`); dev has 11 in `autoinit_test.go`
   covering dry-run no-side-effects, identity seeding, pre-set
   identity respected, default-location store auto-init, etc.
3. **Broadened `--tag`/`--profile` conflict validation** at every
   command entry point. Loop currently validates at 2 sites (apply,
   provider_cmd); the upgrade/config/refresh short-paths are
   un-guarded.

## What changes

- New `internal/cli/autoinit.go` containing `EnsureGlobalConfig`,
  `EnsureStoreReady`, `seedIdentityIfMissing`, dry-run helpers,
  stderr/stdout sinks. Existing call sites in `apply.go`,
  `commands.go`, `provider_cmd.go` collapse to thin
  `autoinit.EnsureXxx(...)` calls.
- New `internal/cli/autoinit_test.go` with ≥9 tests covering the
  scenarios dev's coverage matrix already documents.
- New `ResolveCLITagOverride` invocations in `commands.go` at the
  upgrade/config/refresh entry points (validator + i18n message
  already exist).
- **`WarnIfDefaultsUsed` is preserved**. Loop's `--help`/`--version`
  warning hygiene (commit `3cafb27`) is the better of the two
  approaches; the autoinit module integrates with it as today.

## Impact

- **Affected specs:** `cli-architecture`.
- **Affected code:** `internal/cli/*`. Removing `apply_autoinit_test.go`
  in favour of the new `autoinit_test.go` (test file rename).
- **No user-visible change.** Same first-run UX, same warnings,
  same tag/profile semantics. Adds explicit error on
  `--tag X --profile Y` (X≠Y) at three additional entry points
  that previously silently accepted the conflict.
- **Rollback:** each task commits independently; the test rename
  is fully reversible.
