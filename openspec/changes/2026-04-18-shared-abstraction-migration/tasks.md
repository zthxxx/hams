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
- [ ] **Migrate remaining providers (deferred)** — mechanical repetition of the cargo pattern. Each needs: (a) `hamsfile.go` collapsed to the constants only; (b) `p.effectiveConfig(flags)` → `baseprovider.EffectiveConfig(p.cfg, flags)`; (c) `p.loadOrCreateHamsfile(...)` → `baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)`; (d) `provider.WrapExecPassthrough` default branch → `provider.Passthrough`.
  - [ ] `internal/provider/builtin/goinstall/`
  - [ ] `internal/provider/builtin/npm/`
  - [ ] `internal/provider/builtin/pnpm/`
  - [ ] `internal/provider/builtin/uv/`
  - [ ] `internal/provider/builtin/mas/`
  - [ ] `internal/provider/builtin/vscodeext/`
  - [ ] `internal/provider/builtin/homebrew/` (inline helpers — see homebrew.go lines 682–749 for the same boilerplate)
  - [ ] `internal/provider/builtin/apt/` — keep custom `handleInstall` (pin recovery + post-install probe); only migrate the hamsfile.go boilerplate.
- [ ] **Per-provider passthrough tests** (one minimal DI-seam test per migrated provider, mirroring the cargo template).
- [x] **Spec deltas** in `openspec/changes/2026-04-18-shared-abstraction-migration/specs/provider-system/spec.md` — shared helper contract + passthrough requirement extended beyond git.
- [ ] **Update AGENTS.md** — mark `shared-abstraction-migration` `[x]` once the remaining providers are migrated. Current state: partial migration (cargo is complete; the others still carry boilerplate but the shared helpers are available for adoption).
- [x] **Verification of committed work.**
  - [x] `go build ./...` — passes with zero errors.
  - [x] `go test ./internal/provider/...` — 11 new tests + existing cargo tests all pass.

## Follow-up

The remaining-providers migration is mechanical and safe to do incrementally. Each provider migration is independent and the helpers are additive (nothing is deleted), so adopters can migrate at their own pace. A single sweep commit per provider is the ideal unit (each provider's `hamsfile.go` + `<provider>.go` + a passthrough test).
