# Tasks — 2026-04-17-onboarding-auto-init

Each numbered group ends with a verification step. The change is not
"done" until all groups are verified.

## 1. internal/storeinit package (auto-init core)

- [ ] 1.1 Create `internal/storeinit/` package with `doc.go`.
- [ ] 1.2 Add `template/.gitignore` (`.state/\n*.local.*\n`).
- [ ] 1.3 Add `template/hams.config.yaml` placeholder.
- [ ] 1.4 Implement `Bootstrap(path string) error` that runs `git init`
      (real binary first, go-git fallback) and walks the embedded
      template idempotently.
- [ ] 1.5 Property-based test: bootstrap is idempotent across rapid
      randomized re-runs against `t.TempDir()`.
- [ ] 1.6 Verification: `go test ./internal/storeinit/...` passes.

## 2. Config layer

- [ ] 2.1 Add `Tag` field to `internal/config/Config` aliased to YAML
      key `tag`. Reading honors both `tag:` and legacy `profile_tag:`.
- [ ] 2.2 `Config.Validate` defaults `Tag` to `"default"` when empty
      after merge.
- [ ] 2.3 Add `Config.EffectiveTag()` accessor that returns the resolved
      tag for the active load.
- [ ] 2.4 Update `Config.ProfileDir()` to read from `Tag` instead of
      `ProfileTag`. Keep `ProfileTag` as a struct field for read-back
      compat — `Tag` is the canonical accessor.
- [ ] 2.5 Property-based test: `Tag` and `profile_tag` parsing forms
      round-trip + last-wins on collision.
- [ ] 2.6 Verification: `go test ./internal/config/...` passes.

## 3. CLI surface (--tag flag, default home auto-init)

- [ ] 3.1 Add `--tag` flag to `applyCmd` and `refreshCmd` (urfave/cli
      v3 alias for `--profile`).
- [ ] 3.2 Update `globalFlags(cmd)` to read `--tag` and write into
      `flags.Profile` (single source of truth).
- [ ] 3.3 Update `provider_cmd.go::parseProviderArgs` to recognize
      `--tag` alongside `--profile`.
- [ ] 3.4 Add `internal/cli/autoinit.go::EnsureGlobalConfig(paths)` —
      writes a default global config when the file is missing. Uses
      hostname for `machine_id` and `default` for `tag`.
- [ ] 3.5 Add `internal/cli/autoinit.go::EnsureDefaultStore(paths)` —
      calls `storeinit.Bootstrap` at `${HAMS_DATA_HOME}/store/` and
      updates the global config's `store_path`.
- [ ] 3.6 In `runApply` and `runRefresh`, replace the
      "no store directory configured" hard-fail with auto-init when
      no `--from-repo` was given and config has no `store_path`.
- [ ] 3.7 In every provider's `loadOrCreateHamsfile` path, replace
      the same hard-fail with auto-init.
- [ ] 3.8 Verification: `go test ./internal/cli/... ./internal/provider/builtin/...` passes.

## 4. Provider integration (auto-init on first provider call)

- [ ] 4.1 Replace per-provider `effectiveConfig` empty-store guards
      with a shared seam in `internal/provider/store_resolver.go` that
      auto-inits if needed.
- [ ] 4.2 Add a unit test per provider asserting auto-init fires on
      empty config + records the install in the new store.
- [ ] 4.3 Verification: `go test ./internal/provider/...` passes.

## 5. Test refactor (kill "no store" assertions that block auto-init)

- [ ] 5.1 Audit `git grep "no store directory configured"` — there
      are ~12 callsites across the test files.
- [ ] 5.2 For each, decide: keep negative case (with explicit
      `--no-auto-init` or read-only HOME) OR convert to assert auto-init
      fires.
- [ ] 5.3 Verification: `task check` passes.

## 6. Docs

- [ ] 6.1 Update `docs/content/en/docs/quickstart.mdx` to one-command
      form.
- [ ] 6.2 Update `docs/content/en/docs/cli/apply.mdx` with `--tag`.
- [ ] 6.3 Mirror to `docs/content/zh-CN/`.
- [ ] 6.4 Verification: docs build (`cd docs && pnpm build`) passes.

## 7. Integration tests

- [ ] 7.1 New helper `e2e/base/lib/auto_init.sh` — asserts a clean
      container with empty `$HOME` produces a working store after a
      single `hams brew install <pkg>` invocation.
- [ ] 7.2 Wire helper into `internal/provider/builtin/{apt,homebrew}/integration/integration.sh`.
- [ ] 7.3 Verification: `task ci:itest:run PROVIDER=apt` and
      `PROVIDER=homebrew` pass.

## 8. Final verification

- [ ] 8.1 `task check` (golangci-lint + unit tests + race) passes.
- [ ] 8.2 Manual smoke: build the binary, set `HAMS_CONFIG_HOME` and
      `HAMS_DATA_HOME` to throwaway temp dirs, run `bin/hams apt
      install ` (or simulate via the apt fake runner).
- [ ] 8.3 Spec verify: every new `## ADDED` requirement has at least
      one `#### Scenario:` block.
- [ ] 8.4 Archive this change after the git commit + push lands on
      `dev`.
