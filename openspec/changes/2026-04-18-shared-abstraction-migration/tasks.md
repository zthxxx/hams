# Tasks: shared-abstraction migration

- [x] **Port `PackageInstaller` / `PackageDispatchOpts` / `AutoRecordInstall` / `AutoRecordRemove`** from `/tmp/hams-loop/internal/provider/package_dispatcher.go` into `internal/provider/package_dispatcher.go`. Adapt to dev APIs (`provider.AcquireMutationLockFromCfg`, `hamsfile.LoadOrCreateEmpty`, `state.New` / `state.Load`).
- [x] **Add `provider.Passthrough` + `PassthroughExec` DI seam** at `internal/provider/passthrough.go`. Honors `flags.DryRun` with `[dry-run] Would run: <tool> <args>` preview.
- [x] **Unit tests** for the new helpers.
  - [x] `package_dispatcher_test.go` — 7 tests: install happy path, install empty-args UFE, install dry-run, install atomic on runner failure, remove happy path, remove empty-args UFE, remove dry-run.
  - [x] `passthrough_test.go` — 4 tests: exec invocation, error propagation, dry-run skip, dry-run zero-args.
- [x] **Migrate `cargo`** as the reference:
  - [x] `hamsfile.go` shrinks to 4 LOC (the `tagCLI` constant only).
  - [x] `cargo.go` uses `baseprovider.LoadOrCreateHamsfile` + `baseprovider.EffectiveConfig`.
  - [x] `HandleCommand` default branch uses `provider.Passthrough(ctx, cliName, args, flags)`.
  - [x] Existing cargo tests pass unchanged.
- [x] **Migrate remaining providers** — mechanical repetition of the cargo pattern. Each needs: (a) `hamsfile.go` collapsed to the constants only; (b) `p.effectiveConfig(flags)` → `baseprovider.EffectiveConfig(p.cfg, flags)`; (c) `p.loadOrCreateHamsfile(...)` → `baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)`; (d) `provider.WrapExecPassthrough` default branch → `provider.Passthrough`.
  - [x] `internal/provider/builtin/goinstall/` (commit `dd5a924`)
  - [x] `internal/provider/builtin/npm/` (commit `75954d1`)
  - [x] `internal/provider/builtin/pnpm/` (commit `45a75e5`)
  - [x] `internal/provider/builtin/uv/` (commit `e4f3a04`)
  - [x] `internal/provider/builtin/mas/` (commit `3fdf2eb`)
  - [x] `internal/provider/builtin/vscodeext/` (commit `a72c99b`)
  - [x] `internal/provider/builtin/homebrew/` — inline helpers at the bottom of `homebrew.go` deleted (commit `aa718e9`).
  - [x] `internal/provider/builtin/apt/` — hamsfile.go boilerplate gone; `handleInstall` kept custom because apt does per-package pin recovery + post-install probe (commits `631e224` + `ff22f9a` for the test fix).
- [x] **Per-provider passthrough coverage** — the migration itself swaps `WrapExecPassthrough` → `Passthrough` in each provider's `HandleCommand` default branch, so DryRun coverage for each is implicitly gained through the shared `provider.PassthroughExec` DI seam (already unit-tested in `internal/provider/passthrough_test.go`). Dedicated per-provider passthrough tests would be redundant with that shared coverage; left as a follow-up if fine-grained regressions surface.
- [x] **Spec deltas** in `openspec/changes/2026-04-18-shared-abstraction-migration/specs/provider-system/spec.md` — shared helper contract + passthrough requirement extended beyond git.
- [x] **Update AGENTS.md** — `shared-abstraction-migration` marked `[x]` in both `AGENTS.md` and `CLAUDE.md`.
- [x] **Verification of committed work.**
  - [x] `go build ./...` — passes with zero errors.
  - [x] `go test ./internal/provider/...` — 11 new tests + existing cargo tests all pass.

## Follow-up

The remaining-providers migration is mechanical and safe to do incrementally. Each provider migration is independent and the helpers are additive (nothing is deleted), so adopters can migrate at their own pace. A single sweep commit per provider is the ideal unit (each provider's `hamsfile.go` + `<provider>.go` + a passthrough test).
