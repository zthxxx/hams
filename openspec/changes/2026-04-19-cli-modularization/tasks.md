# Tasks — cli-modularization

## 1. Extract autoinit.go

- [ ] 1.1 Identify auto-init helpers currently in
  `internal/cli/apply.go`, `commands.go`, `provider_cmd.go`:
  `EnsureGlobalConfig`, `EnsureStoreReady`, `seedIdentityIfMissing`,
  dry-run helpers, stdout/stderr sinks.
- [ ] 1.2 Create `internal/cli/autoinit.go` with the moved code.
  Keep package-private symbols private; export only what other
  files in `internal/cli` need.
- [ ] 1.3 Update call sites in `apply.go`, `commands.go`,
  `provider_cmd.go` to call the moved symbols.
- [ ] 1.4 Verify `go vet ./...` + `golangci-lint run` clean.
- [ ] 1.5 Verify `task test:unit` still passes (coverage unchanged
  by the move).
- [ ] 1.6 Atomic commit: `refactor(cli): extract auto-init helpers
  into dedicated autoinit.go`.

## 2. Dedicated autoinit tests

- [ ] 2.1 Rename `internal/cli/apply_autoinit_test.go` to
  `autoinit_test.go` (preserves the existing 2 tests).
- [ ] 2.2 Add tests:
  - [ ] `TestEnsureGlobalConfig_CreatesWhenMissing`
  - [ ] `TestEnsureGlobalConfig_IsIdempotent`
  - [ ] `TestEnsureGlobalConfig_DryRunSkipsWrite`
  - [ ] `TestEnsureStoreReady_AutoInitsAtDefaultLocation`
  - [ ] `TestEnsureStoreReady_DryRunHasNoSideEffects`
  - [ ] `TestEnsureStoreReady_SeedsIdentity`
  - [ ] `TestEnsureStoreReady_RespectsPreSetIdentity`
  - [ ] `TestEnsureStoreReady_HonoursExplicitStorePath`
  - [ ] `TestEnsureStoreReady_FailsLoudlyOnPermissionDenied`
- [ ] 2.3 Each test uses `t.TempDir()` and DI seams from
  `storeinit` (Change 1 §3) to avoid touching the host.
- [ ] 2.4 `task test:unit` passes with race detector.
- [ ] 2.5 Atomic commit: `test(cli): add dedicated autoinit_test.go
  covering ≥9 first-run scenarios`.

## 3. Tag/profile validation broadening

- [ ] 3.1 Find every entry point in `internal/cli/commands.go`
  that reads `flags.Tag` or `flags.Profile` and currently does
  not call `config.ResolveCLITagOverride`. Expected: upgrade,
  config, refresh short-paths.
- [ ] 3.2 Add `ResolveCLITagOverride(flags.Tag, flags.Profile)`
  call at each, returning the same `ExitUsageError` on conflict.
- [ ] 3.3 Add a single `TestCommandsRejectTagProfileConflict`
  table-driven test in `internal/cli/commands_test.go` (or
  closest existing test file) iterating every newly-guarded
  command verb.
- [ ] 3.4 `task test:unit` passes.
- [ ] 3.5 Atomic commit: `feat(cli): broaden --tag/--profile conflict
  detection to upgrade/config/refresh entry points`.

## 4. Spec updates and verification

- [ ] 4.1 Write `specs/cli-architecture/spec.md` delta with SHALLs:
  "Auto-init helpers (`EnsureGlobalConfig`, `EnsureStoreReady`,
  `seedIdentityIfMissing`) MUST live in `internal/cli/autoinit.go`",
  "Every command entry point that accepts `--tag` and `--profile`
  flags MUST validate the conflict via
  `config.ResolveCLITagOverride` before proceeding".
- [ ] 4.2 `task check` passes end-to-end.
- [ ] 4.3 Run `/opsx:archive 2026-04-19-cli-modularization`.
