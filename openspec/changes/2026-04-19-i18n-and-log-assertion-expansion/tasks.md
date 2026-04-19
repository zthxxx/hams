# Tasks — i18n + log-assertion expansion

## 1. i18n catalog expansion

- [ ] 1.1 Diff `internal/i18n/keys.go` between `local/loop` and
  the dev worktree at `~/.claude/hams-worktrees/dev-analysis`;
  compile the list of ~57 new constants to copy
  (categories: autoinit.*, ufe.*, store.init/pull/commit,
  config.set/unset/open, list.*, upgrade.*, sudo.prompt,
  tui.fallback, expanded apply/refresh, git.dispatcher).
- [ ] 1.2 Append the new constants to loop's `keys.go`,
  preserving loop's existing comment-section structure.
- [ ] 1.3 Cherry-pick the corresponding `id: ...` entries from
  dev's `internal/i18n/locales/en.yaml` into loop's copy
  (verbatim).
- [ ] 1.4 Cherry-pick the matching `id: ...` entries from dev's
  `zh-CN.yaml` into loop's copy (verbatim).
- [ ] 1.5 Run `task test:unit` to confirm `TestLocalesAreInParity`
  still passes (en ↔ zh-CN key set is identical).
- [ ] 1.6 Atomic commit: `feat(i18n): expand catalog by 57 keys
  covering autoinit/ufe/store/config/list/upgrade lifecycle`.

## 2. Coherence test + call-site wiring

- [ ] 2.1 Add `TestCatalogCoherence_EveryTypedKeyResolves` to
  `internal/i18n/locale_parity_test.go` (or new
  `i18n_coherence_test.go`) hand-listing every typed constant
  and asserting `id: <key>` appears in both locale files.
- [ ] 2.2 Wire new keys into call sites:
  - autoinit.go (from Change 2): replace hardcoded
    "auto-init: created store at" etc. with `i18n.Tf(...)`
  - commands.go upgrade flow: brew dry-run line, success, etc.
  - commands.go config flow: set/unset/open output
  - commands.go list flow: header, no-resources-filter
  - sudo flow: prompt template
  - git dispatcher header
  - Errors family (ufe.*) wherever applicable.
- [ ] 2.3 Confirm `rg 'fmt\.Print|slog\.Info' internal/cli/ |
  grep -v _test` shows only operational logs (key present in
  catalog), not user-facing English literals that should be
  i18n'd.
- [ ] 2.4 `task test:unit` passes.
- [ ] 2.5 Atomic commit: `feat(i18n): wire 57 new keys into
  CLI call sites + add catalog-coherence test`.

## 3. File-based log assertions

- [ ] 3.1 Append `assert_log_contains` and
  `assert_log_records_session` to `e2e/base/lib/assertions.sh`,
  verbatim from dev's copy.
- [ ] 3.2 In `internal/provider/builtin/apt/integration/
  integration.sh`, immediately after the canonical
  `standard_cli_flow` invocation, add:
  - `assert_log_records_session "apt integration"`
  - `assert_log_contains "apt provider records applied actions"
    "provider"`
  Set `HAMS_DATA_HOME=/tmp/test-apt-data` near the env block
  for log-file isolation.
- [ ] 3.3 Run `task ci:itest:run PROVIDER=apt` to confirm the
  apt container exercises the new helpers green.
- [ ] 3.4 Atomic commit: `test(integration): add file-based log
  assertions and wire into apt as canonical reference`.

## 4. Spec updates and verification

- [ ] 4.1 Write `specs/cli-architecture/spec.md` delta with SHALL:
  "Every user-facing string emitted by `internal/cli/**` MUST
  resolve through `internal/i18n` for both en and zh-CN".
- [ ] 4.2 Write `specs/code-standards/spec.md` delta with SHALL:
  "Every typed i18n constant declared in
  `internal/i18n/keys.go` MUST resolve in every locale file
  (`internal/i18n/locales/*.yaml`); enforced by
  `TestCatalogCoherence_EveryTypedKeyResolves`".
- [ ] 4.3 `task check` passes end-to-end.
- [ ] 4.4 Run `/opsx:archive 2026-04-19-i18n-and-log-assertion-expansion`.
