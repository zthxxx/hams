# Tasks — 2026-04-17-onboarding-auto-init

Each numbered group ends with a verification step. The change is not
"done" until all groups are verified.

## 1. internal/storeinit package (auto-init core)

- [x] 1.1 Create `internal/storeinit/` package with `doc.go`.
- [x] 1.2 Add `template/.gitignore` (`.state/\n*.local.*\n`).
- [x] 1.3 Add `template/hams.config.yaml` placeholder.
- [x] 1.4 Implement `Bootstrap(path string) error` that runs `git init`
      (real binary first, go-git fallback) and walks the embedded
      template idempotently.
- [x] 1.5 Property-based test: bootstrap is idempotent across rapid
      randomized re-runs against `t.TempDir()`.
- [x] 1.6 Verification: `go test ./internal/storeinit/...` passes
      (coverage 70.9%).

## 2. Config layer

- [x] 2.1 Reading honors both `tag:` (canonical) and legacy
      `profile_tag:` via `Config.UnmarshalYAML`. `tag:` wins on collision.
- [x] 2.2 `WriteConfigKey` accepts both keys; new files write `tag:`,
      legacy files keep `profile_tag:` to avoid duplicate-key drift.
- [x] 2.3 `MarshalYAML` emits `tag:` as the canonical key for
      Go-side full-Config writes.
- [x] 2.4 Property-style table test: `TestUnmarshalYAML_TagAlias`
      covers legacy_only / new_only / both_tag_wins / empty_both.
- [x] 2.5 `TestWriteConfigKey_TagAndProfileTagAlias` asserts new files
      write `tag:`, legacy `profile_tag:` files get updated in place.
- [x] 2.6 Verification: `go test ./internal/config/...` passes.

## 3. CLI surface (--tag flag, default home auto-init)

- [x] 3.1 `--tag` flag added at the global flag level (`globalFlagDefs`)
      with `--profile` as the alias.
- [x] 3.2 `globalFlags(cmd)` reads `cmd.String("tag")` (alias-aware).
- [x] 3.3 `parseProviderArgs` and `stripGlobalFlags` recognize both
      `--tag=` and `--tag <value>` forms.
- [x] 3.4 `EnsureGlobalConfig(paths)` writes the default global config
      when missing — `tag: default` + hostname-derived `machine_id`.
- [x] 3.5 `EnsureStoreReady(paths, cfg, override)` bootstraps the
      default store at `${HAMS_DATA_HOME}/store/` and persists the
      path back into the global config.
- [x] 3.6 `runApply` replaces the "no store directory configured"
      hard-fail with auto-init (preserves the legacy error when
      `HAMS_NO_AUTO_INIT=1`).
- [x] 3.7 `runRefresh` mirrors the same auto-init pattern.
- [x] 3.8 `routeToProvider` calls `autoInitForProvider` BEFORE
      dispatch so providers see a populated `flags.Store`.
- [x] 3.9 Verification: `go test ./internal/cli/...` passes.

## 4. Test isolation

- [x] 4.1 `TestMain` defaults `HAMS_NO_AUTO_INIT=1` so existing CLI
      tests don't pollute `$HOME` on auto-init paths.
- [x] 4.2 `TestRouteToProvider_AutoInitFiresWhenStoreMissing` exercises
      the dispatch-side auto-init with isolated `HAMS_CONFIG_HOME` /
      `HAMS_DATA_HOME` temp dirs.
- [x] 4.3 `TestRouteToProvider_AutoInitSkippedWhenStoreOverridden`
      covers the `--store` override path.
- [x] 4.4 Manual host-pollution audit: `ls ~/.config/hams ~/.local/share/hams`
      after the test suite shows zero new files. **Verified.**

## 5. Docs

- [ ] 5.1 Update `docs/content/en/docs/quickstart.mdx` to one-command
      form.
- [ ] 5.2 Update `docs/content/en/docs/cli/apply.mdx` with `--tag`.
- [ ] 5.3 Mirror to `docs/content/zh-CN/`.
- [ ] 5.4 Verification: docs build (`cd docs && pnpm build`) passes.

## 6. Integration tests

- [ ] 6.1 New helper `e2e/base/lib/auto_init.sh` — asserts a clean
      container with empty `$HOME` produces a working store after a
      single `hams brew install <pkg>` invocation.
- [ ] 6.2 Wire helper into a sample provider's
      `internal/provider/builtin/<provider>/integration/integration.sh`.
- [ ] 6.3 Verification: `task ci:itest:run PROVIDER=<picked>` passes.

## 7. Manual smoke

- [x] 7.1 Build the binary, set `HAMS_CONFIG_HOME` and `HAMS_DATA_HOME`
      to throwaway temp dirs, run `bin/hams apply --tag=default`.
      **Verified 2026-04-17**: auto-creates global config at
      `<tmp>/cfg/hams.config.yaml`, auto-inits store at
      `<tmp>/data/store/`, exits 0 with "No providers match".
- [x] 7.2 Same setup with `bin/hams cargo notreal-cmd` — confirms
      provider dispatch fires `autoInitForProvider` before the
      provider's `HandleCommand`. **Verified 2026-04-17.**

## 8. Final verification

- [x] 8.1 `task lint` passes (0 issues).
- [x] 8.2 `task test:unit` passes (32/32 packages).
- [x] 8.3 Spec verify: every new `## ADDED` requirement has at least
      one `#### Scenario:` block.
- [ ] 8.4 Archive this change after Phase E (provider unification),
      Phase F (integration log assertions), Phase G (i18n) and the
      docs sub-task land. The change folder stays open until the
      onboarding promise is fully shipped.
