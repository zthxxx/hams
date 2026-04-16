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

## 2. npm

- [ ] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/npm/` — tag: `"cli"` (npm defaults to global packages; matches current passthrough semantics)

## 3. pnpm

- [ ] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/pnpm/` — tag: `"cli"`

## 4. uv

- [ ] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/uv/` — tag: `"cli"`

## 5. goinstall

- [ ] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/goinstall/` — tag: `"cli"`
- [ ] Preserve `injectLatest` semantics — stripped version suffix is what goes into `AddApp`

## 6. mas

- [ ] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/mas/` — tag: `"cli"`
- [ ] Validate the app-ID is numeric before recording (mas install takes an Apple App Store numeric ID)

## 7. vscodeext (code-ext)

- [ ] Mirror cargo's 1.1-1.14 on `internal/provider/builtin/vscodeext/` — tag: `"cli"`
- [ ] Record per-extension (`--install-extension <ext>` → one hamsfile entry per ext)

## 8. Spec delta

- [ ] Add CP-1 auto-record Requirement + Scenarios to `openspec/specs/builtin-providers/spec.md` once all 7 providers are implemented
- [ ] Include the `apt` U1-U5 contract as canonical scenarios so future Package providers are held to the same bar

## 9. Close-out

- [ ] Final `task check` across the full change
- [ ] Update AGENTS.md progress log with the cycle summary
- [ ] Archive this change folder after deploy
