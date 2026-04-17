# Tasks — 2026-04-16-package-provider-auto-record-gap

Each provider gets an atomic commit wiring auto-record + a regression test suite following `internal/provider/builtin/apt/apt_test.go` U1-U7 as the reference. Order: smallest surface first so the pattern settles before the larger providers.

## 1. cargo (pilot) — DONE in commit `39f8f4c`

- [x] 1.1 Add `cfg *config.Config` field to `cargo.Provider` (`internal/provider/builtin/cargo/cargo.go`)
- [x] 1.2 Update `New(cfg *config.Config, runner CmdRunner)` signature
- [x] 1.3 Create `internal/provider/builtin/cargo/hamsfile.go` mirroring `apt/hamsfile.go` (with `tagCLI = "cli"`, `loadOrCreateHamsfile`, `hamsfilePath`, `effectiveConfig`)
- [x] 1.4 Update `HandleCommand` install branch to `loadOrCreateHamsfile` → `AddApp` → `Write` after successful runner.Install (switched from `WrapExecPassthrough` to runner seam for DI testability)
- [x] 1.5 Update `HandleCommand` remove branch to `loadOrCreateHamsfile` → `RemoveApp` → `Write` after successful runner.Uninstall
- [x] 1.6 Update `internal/cli/register.go` to pass `builtinCfg` into `cargo.New`
- [x] 1.7 Update `internal/cli/bootstrap_invariant_test.go` to pass `cfg` to `cargo.New`
- [x] 1.8 Add `TestHandleCommand_U1_InstallAddsCrateToHamsfile` matching `apt`'s U1
- [x] 1.9 Add `TestHandleCommand_U2_InstallIsIdempotent` matching apt's U2
- [x] 1.10 Add `TestHandleCommand_U3_InstallFailureLeavesHamsfileUntouched` matching apt's U3
- [x] 1.11 Add `TestHandleCommand_U4_RemoveDeletesFromHamsfile` matching apt's U4
- [x] 1.12 Add `TestHandleCommand_U5_RemoveFailureLeavesHamsfileUntouched` matching apt's U5
- [x] 1.13 Add `TestHandleCommand_U6_DryRunSkipsRunnerAndHamsfile` matching apt's U7
- [x] 1.14 Add U7 multi-crate install + U8 atomic-failure + U9 flag-filter + U10 flags-only-usage for cargo-specific edges
- [x] 1.15 Verify `task check` passes (0 issues, all 33 packages PASS). Coverage 70.6% → 81.0%.

## 2. npm — DONE in commit `4c89814`

- [x] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/npm/` — tag: `"cli"`. 10 TestHandleCommand_U tests. Coverage 69.6% → 79.2%.

## 3. pnpm — DONE in commit `e24e12b`

- [x] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/pnpm/` — tag: `"cli"`. 10 TestHandleCommand_U tests. Coverage 73.2% → 81.7%.

## 4. uv — DONE in commit `76022d6`

- [x] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/uv/` — tag: `"cli"`. 10 TestHandleCommand_U tests. Coverage 71.8% → 80.4%.

## 5. goinstall — DONE in commit `7caeb3f`

- [x] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/goinstall/` — tag: `"cli"`, `injectLatest` pins bare module paths before runner AND hamsfile write. 9 TestHandleCommand_U tests (goinstall has no `uninstall` verb). Coverage 64.2% → 76.4%.

## 6. mas — DONE in commit `7de53aa`

- [x] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/mas/` — tag: `"cli"`, numeric App Store IDs recorded verbatim. 10 TestHandleCommand_U tests. Coverage 74.4% → 82.3%.

## 7. vscodeext (code-ext) — DONE in commit `ba3bb3e`

- [x] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/vscodeext/` — tag: `"cli"`, per-extension recording. 10 TestHandleCommand_U tests. Coverage 69.1% → 80.6%.

## 8. State-file parity (added after cycles 96/202–208 exposed the `hams list --only=<provider>` gap)

- [x] homebrew — CLI install/remove now writes state (cycle 96).
- [x] mas — CLI install/remove now writes state (cycle 202, commit `ff138f9`).
- [x] cargo — CLI install/remove now writes state (cycle 203, commit `2a51372`).
- [x] npm — CLI install/remove now writes state (cycle 204, commit `f5952c7`).
- [x] pnpm — CLI install/remove now writes state (cycle 205, commit `9eafebb`).
- [x] uv — CLI install/remove now writes state (cycle 206, commit `cb7ebee`).
- [x] goinstall — CLI install writes state; no symmetric remove (cycle 207, commit `16612ce`).
- [x] vscodeext — CLI install/remove now writes state (cycle 208, commit `e376fbf`).
- [x] Spec delta rewritten to require BOTH hamsfile AND state writes on the CLI path.

## 9. Spec delta archive

- [x] Deploy-time step: archive this change folder into `openspec/changes/archive/`. The CP-1 auto-record Requirement is already merged into `openspec/specs/builtin-providers/spec.md` (auto-record scope sections at lines 964/1026/1140/1170/1180); folder moved to archive 2026-04-17 in the v1 cleanup pass.

## 10. Close-out

- [x] Final `task check` across the full change (each cycle verified independently; final green after cycle 83).
- [x] Update AGENTS.md progress log with the cycle summary.
- [x] Archive this change folder after deploy — archived 2026-04-17 in the v1 cleanup pass.
