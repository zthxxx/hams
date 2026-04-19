# Tasks — cli-modularization

## 1. Extract autoinit.go

- [x] 1.1 Identified auto-init helpers in
  `internal/cli/apply.go`: `ensureProfileConfigured`,
  `statFile` (1559-line god-file shape).
  Note: local/loop's auto-init shape differs from dev's
  EnsureGlobalConfig/EnsureStoreReady split — loop's
  `ensureProfileConfigured` covers the same ground in one
  helper, and `storeinit.Bootstrap` (in `internal/storeinit`)
  handles the store-side scaffolding from
  `provider_cmd.go`.
- [x] 1.2 Created `internal/cli/autoinit.go` with the moved
  code. Package-private symbols stay private.
- [x] 1.3 Updated `apply.go` (drops unused `golang.org/x/term`
  import, drops 116 lines).
- [x] 1.4 `go vet ./...` + `golangci-lint run --timeout 5m`
  clean (`0 issues`).
- [x] 1.5 `go test -race -count=1 ./internal/cli/...` passes.
- [x] 1.6 Atomic commit: `refactor(cli): extract auto-init
  helpers into autoinit.go + 7 dedicated tests`.

## 2. Dedicated autoinit tests

- [x] 2.1 Renamed `internal/cli/apply_autoinit_test.go` to
  `autoinit_test.go` (preserves the existing 2 tests).
- [x] 2.2 Added 8 new direct unit tests:
  - [x] `TestEnsureProfileConfigured_CLITagSeedsConfigOnFreshMachine`
  - [x] `TestEnsureProfileConfigured_AutoInitSkippedWhenConfigExists`
  - [x] `TestEnsureProfileConfigured_NonTTYWithoutCLITagFails`
  - [x] `TestEnsureProfileConfigured_TagAliasMatchesProfile`
  - [x] `TestEnsureProfileConfigured_TagProfileConflictRejected`
  - [x] `TestStatFile_MissingReportsFalse`
  - [x] `TestStatFile_PresentReportsTrue`
  - [x] `TestStatFile_DirectoryReportsTrue`
- [x] 2.3 Each test uses `t.TempDir()` and `t.Setenv` to scope
  to the test runner.
- [x] 2.4 `task test:unit`-equivalent
  (`go test -race ./internal/cli/...`) passes with race
  detector.
- [x] 2.5 Bundled into the §1.6 commit.

## 3. Tag/profile validation broadening

- [x] 3.1 Found every entry point in `internal/cli/commands.go`
  that reads `flags.Tag` or `flags.Profile` and did not call
  `config.ResolveCLITagOverride`: refresh, list, config list,
  config get, store status, store init, store push, store pull
  (7 sites total).
- [x] 3.2 Added `enforceTagProfileConsistency(flags)` thin
  wrapper in `autoinit.go` and called it at every site
  immediately after `globalFlags(cmd)` and before
  `resolvePaths(flags)`.
- [x] 3.3 Added `TestEnforceTagProfileConsistency_TableDriven`
  (6 cases: nil, empty, tag-only, profile-only, matching,
  conflicting).
- [x] 3.4 `task test:unit`-equivalent passes.
- [x] 3.5 Atomic commit: `feat(cli): broaden --tag/--profile
  conflict detection to 7 command entry points`.

## 4. Spec updates and verification

- [x] 4.1 Wrote `specs/cli-architecture/spec.md` delta with
  two SHALLs (autoinit.go boundary + every-entry-point tag
  validation). `openspec validate
  --changes 2026-04-19-cli-modularization --strict` passes.
- [ ] 4.2 `task check` passes end-to-end (deferred to the
  end-of-workstream verification pass shared with Changes 1
  and 3).
- [ ] 4.3 Archive 2026-04-19-cli-modularization after task
  check is green.
