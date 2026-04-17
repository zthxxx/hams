# Tasks — 2026-04-18-provider-autoscaffold-store

## 1. Embedded templates

- [x] 1.1 Create `internal/cli/template/store/gitignore` with the
      hams-store baseline ignore rules (`.state/`, `*.local.yaml`,
      `*.local.*`, common editor cruft).
- [x] 1.2 Create `internal/cli/template/store/hams.config.yaml` with
      a commented header that points readers at the schema-design
      spec. Minimal, same shape as `hams store init` emits.
- [x] 1.3 Add `//go:embed template/store` declaration in
      `internal/cli/scaffold.go`. Files are renamed at write time
      (gitignore → .gitignore).

## 2. Scaffold helper

- [x] 2.1 Add `cli.EnsureStoreScaffolded(ctx, paths, flags)` in
      `internal/cli/scaffold.go` that resolves the store path,
      mkdir -p's it, runs `git init` via a DI-seam, writes missing
      template files, and persists `store_path` to the global config.
- [x] 2.2 Wire `gitInitExec` as a package-level func variable so
      tests swap in a fake that records the invocation without
      shelling out to the real git binary.
- [x] 2.3 Resolve the default store location: `$HAMS_STORE` env var
      > `$HAMS_DATA_HOME/store` default. Config-persisted `store_path`
      from an earlier scaffold wins over both via the earlier
      `cfg.StorePath` branch.
- [x] 2.4 `--dry-run` skips all side effects; just logs the intent.

## 3. CLI integration

- [x] 3.1 `routeToProvider` (internal/cli/provider_cmd.go) calls
      `EnsureStoreScaffolded` before `handler.HandleCommand` unless
      `--hams-no-scaffold` is set.
- [x] 3.2 On scaffold success, `flags.Store` is populated with the
      scaffolded path so every downstream provider's
      `effectiveConfig` overlay picks it up without re-implementing
      the resolution.
- [x] 3.3 Collapse `flags.Tag` / `flags.Profile` via
      `config.ResolveCLITagOverride` before `HandleCommand` runs so
      providers honor `--tag=<t>` without each needing an
      effectiveConfig change.
- [x] 3.4 parseProviderArgs + stripGlobalFlags now recognize `--tag`
      in addition to `--profile`, matching the root-level
      globalFlagDefs.

## 4. Tests

- [x] 4.1 `TestEnsureStoreScaffolded_CreatesDirAndTemplates` —
      happy path; assert all files present + git init called once.
- [x] 4.2 `TestEnsureStoreScaffolded_Idempotent` — hand-edit the
      .gitignore between calls; second call must not clobber it.
- [x] 4.3 `TestEnsureStoreScaffolded_RespectsHamsStoreEnv` — env
      override wins over `$HAMS_DATA_HOME/store` default.
- [x] 4.4 Full `go test -race ./internal/cli/` passes green, no
      regressions in existing `TestRouteToProvider` /
      `TestRunApply_*` tests.

## 5. Verification

- [x] 5.1 `task check` green through lint, unit, integration (e2e
      via act fails on artifact upload — known act limitation, not
      a code issue).
- [ ] 5.2 Manual smoke in dev sandbox (deferred — docker required;
      covered in full e2e pass).
- [x] 5.3 Archive this change once Current Tasks list is fully
      ticked off.
