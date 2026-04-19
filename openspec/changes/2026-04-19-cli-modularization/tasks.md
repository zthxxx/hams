# Tasks — cli-modularization

## 1. Extract autoinit.go

- [x] 1.1 Identify auto-init helpers currently in
  `internal/cli/apply.go`, `commands.go`, `provider_cmd.go`:
  `EnsureGlobalConfig`, `EnsureStoreReady`, `seedIdentityIfMissing`,
  dry-run helpers, stdout/stderr sinks.
- [x] 1.2 Create `internal/cli/autoinit.go` with the moved code.
  Keep package-private symbols private; export only what other
  files in `internal/cli` need.
- [x] 1.3 Update call sites in `apply.go`, `commands.go`,
  `provider_cmd.go` to call the moved symbols.
- [x] 1.4 Verify `go vet ./...` + `golangci-lint run` clean.
- [x] 1.5 Verify `task test:unit` still passes (coverage unchanged
  by the move).
- [x] 1.6 Atomic commit: `refactor(cli): extract auto-init helpers
  into dedicated autoinit.go`.

## 2. Dedicated autoinit tests

- [x] 2.1 Rename `internal/cli/apply_autoinit_test.go` to
  `autoinit_test.go` (preserves the existing 2 tests).
- [x] 2.2 Add tests:
  - [x] `TestEnsureGlobalConfig_CreatesWhenMissing`
  - [x] `TestEnsureGlobalConfig_IsIdempotent`
  - [x] `TestEnsureGlobalConfig_DryRunSkipsWrite`
  - [x] `TestEnsureStoreReady_AutoInitsAtDefaultLocation`
  - [x] `TestEnsureStoreReady_DryRunHasNoSideEffects`
  - [x] `TestEnsureStoreReady_SeedsIdentity`
  - [x] `TestEnsureStoreReady_RespectsPreSetIdentity`
  - [x] `TestEnsureStoreReady_HonoursExplicitStorePath`
  - [x] `TestEnsureStoreReady_FailsLoudlyOnPermissionDenied`
- [x] 2.3 Each test uses `t.TempDir()` and DI seams from
  `storeinit` (Change 1 §3) to avoid touching the host.
- [x] 2.4 `task test:unit` passes with race detector.
- [x] 2.5 Atomic commit: `test(cli): add dedicated autoinit_test.go
  covering ≥9 first-run scenarios`.

## 3. Tag/profile validation broadening

- [x] 3.1 Find every entry point in `internal/cli/commands.go`
  that reads `flags.Tag` or `flags.Profile` and currently does
  not call `config.ResolveCLITagOverride`. Expected: upgrade,
  config, refresh short-paths.
- [x] 3.2 Add `ResolveCLITagOverride(flags.Tag, flags.Profile)`
  call at each, returning the same `ExitUsageError` on conflict.
- [x] 3.3 Add a single `TestCommandsRejectTagProfileConflict`
  table-driven test in `internal/cli/commands_test.go` (or
  closest existing test file) iterating every newly-guarded
  command verb.
- [x] 3.4 `task test:unit` passes.
- [x] 3.5 Atomic commit: `feat(cli): broaden --tag/--profile conflict
  detection to upgrade/config/refresh entry points`.

## 4. Spec updates and verification

- [x] 4.1 Write `specs/cli-architecture/spec.md` delta with SHALLs:
  "Auto-init helpers (`EnsureGlobalConfig`, `EnsureStoreReady`,
  `seedIdentityIfMissing`) MUST live in `internal/cli/autoinit.go`",
  "Every command entry point that accepts `--tag` and `--profile`
  flags MUST validate the conflict via
  `config.ResolveCLITagOverride` before proceeding".
- [x] 4.2 `task check` passes end-to-end.
- [x] 4.3 Run `/opsx:archive 2026-04-19-cli-modularization`.
