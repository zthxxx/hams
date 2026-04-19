# i18n catalog + log-assertion expansion

## Why

Per §9.1/§9.2 of `docs/notes/dev-vs-loop-implementation-analysis.md`:

1. `local/loop` ships **119** i18n keys (provider errors + apply
   status); `origin/dev` ships **176** keys covering the entire CLI
   lifecycle (autoinit, ufe, store, config, list, upgrade, sudo,
   TUI, expanded apply/refresh, git-dispatcher). 57 keys' worth
   of user-facing strings are still hardcoded in English on
   `local/loop`.
2. `origin/dev` ships **file-based log assertions**
   (`assert_log_contains`, `assert_log_records_session`) that
   complement the existing stderr-based gates. They greps the
   rolling log file under `${HAMS_DATA_HOME}/<YYYY-MM>/hams.*.log`
   to verify the persistent-log code path actually wrote something.
   Loop's stderr-only gate cannot catch handler regressions where
   stderr emits but the file write silently fails.

## What changes

- **i18n catalog expansion.** Mirror `origin/dev`'s 57 new typed
  constants in `internal/i18n/keys.go` and the corresponding
  entries in `internal/i18n/locales/{en,zh-CN}.yaml`. Translations
  are taken **verbatim** from `origin/dev` to avoid wording drift.
  Wire each new key into its call site (autoinit module from
  Change 2, commands.go upgrade/config/list, sudo prompts, git
  dispatcher).
- **Coherence test.** Add `TestCatalogCoherence_EveryTypedKeyResolves`
  that hand-lists every typed constant and asserts each appears
  as `id: <key>` in both locale files. Coexists with the existing
  `TestLocalesAreInParity` (dynamic, catches orphans) and
  `TestProviderKeysResolve{English,Chinese}` (catches unresolved
  template placeholders) — best of three worlds.
- **File-based log assertions.** Cherry-pick
  `assert_log_contains` and `assert_log_records_session` from
  `origin/dev`'s `e2e/base/lib/assertions.sh` into loop's copy.
  Wire them into `internal/provider/builtin/apt/integration/
  integration.sh` as the canonical reference (alongside the
  existing E0 + stderr gates — additive, not replacing).

## Impact

- **Affected specs:** `cli-architecture`, `code-standards`.
- **Affected code:** `internal/i18n/`, `internal/cli/*` (call-site
  rewrites), provider commands.go (upgrade/list/config),
  `e2e/base/lib/assertions.sh`, apt integration.sh.
- **User-visible change:** all currently-English-only first-run,
  config, list, and upgrade output gains zh-CN translations.
  Same exit codes, same semantics, same logging fields.
- **Rollback:** the catalog expansion and the log-helper port
  commit independently.
