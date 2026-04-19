# Tasks — i18n + log-assertion expansion

## 1. i18n catalog expansion

- [x] 1.1 Diffed `internal/i18n/keys.go` between `local/loop`
  and the dev worktree at `~/.claude/hams-worktrees/dev-analysis`;
  identified 175 dev-only typed constants spanning the full
  CLI lifecycle (autoinit, ufe, store, config, list, upgrade,
  sudo, TUI, expanded apply/refresh, git-dispatcher, errors
  prefix, provider help).
- [x] 1.2 Appended the 175 new constants to loop's `keys.go`
  under a clearly-marked `Imported from origin/dev` section
  block, preserving loop's existing comment-section structure
  for the original 118 constants. Total constants now: 293.
- [x] 1.3 Cherry-picked the corresponding `id: ...` entries
  from dev's `internal/i18n/locales/en.yaml` into loop's copy
  (verbatim — 762 lines added, including section comment
  headers).
- [x] 1.4 Cherry-picked the matching `id: ...` entries from
  dev's `zh-CN.yaml` into loop's copy (verbatim — 762 lines).
- [x] 1.5 `task test:unit`-equivalent
  (`go test -race ./internal/i18n/...`) passes;
  `TestLocalesAreInParity` confirms en ↔ zh-CN remains in
  lockstep.
- [x] 1.6 Atomic commit: `feat(i18n): expand catalog by 175
  keys covering full CLI lifecycle …`.

## 2. Coherence test + call-site wiring

- [x] 2.1 Added
  `internal/i18n/catalog_coherence_test.go` with
  `TestCatalogCoherence_EveryTypedKeyResolves` —
  hand-maintains a slice of every exported constant in
  `keys.go` (now 293 entries) and asserts each appears as
  `id: <key>` in BOTH `en.yaml` and `zh-CN.yaml`. Best-of-
  three coverage with the existing dynamic-parity test and
  Provider-template-interpolation test.
- [-] 2.2 Wiring of the new keys into call sites is **deferred
  to follow-up work**. The catalog is in place and importable
  today; rewiring 175 hardcoded English strings spread across
  `internal/cli/**` and `internal/provider/builtin/**` is
  separate from "absorb dev's catalog breadth" — it is a
  multi-PR mechanical refactor in its own right. Tracked as
  archive follow-up of this change.
- [x] 2.3 Verified `0 issues` from
  `golangci-lint run --timeout 5m`.
- [x] 2.4 `task test:unit`-equivalent passes for the i18n
  package (4.052s including the new coherence test).
- [x] 2.5 Bundled into the §1.6 commit.

## 3. File-based log assertions

- [x] 3.1 Appended `assert_log_contains` and
  `assert_log_records_session` to
  `e2e/base/lib/assertions.sh`, verbatim from dev's copy
  (≈40 lines).
- [x] 3.2 In
  `internal/provider/builtin/apt/integration/integration.sh`:
  - Set `HAMS_DATA_HOME=/tmp/test-apt-data` near the env
    block (with `mkdir -p`) for log-file isolation.
  - After `standard_cli_flow` + the existing E0 / E1–E7
    scenarios, added a short canonical-reference block that
    triggers `hams apply --only=apt` then runs
    `assert_log_records_session` and `assert_log_contains`.
- [-] 3.3 Container exercise via `task ci:itest:run
  PROVIDER=apt` is deferred to the end-of-workstream
  verification pass — requires Docker, runs as part of `task
  check`. Shell syntax check (`bash -n`) on the script is
  green.
- [x] 3.4 Atomic commit: `test(integration): add file-based
  log assertions and wire into apt as canonical reference`.

## 4. Spec updates and verification

- [x] 4.1 Wrote `specs/cli-architecture/spec.md` delta with
  SHALL: "i18n catalog MUST cover the full CLI lifecycle"
  (lists every category: autoinit, ufe, apply, refresh,
  store, config, list, upgrade, sudo, TUI, bootstrap,
  errors, provider help, git dispatcher).
- [x] 4.2 Wrote `specs/code-standards/spec.md` delta with two
  SHALLs: "Every typed i18n constant MUST resolve in every
  locale" + "Integration tests MUST verify file-based slog
  emission".
  `openspec validate
  --changes 2026-04-19-i18n-and-log-assertion-expansion
  --strict` passes.
- [ ] 4.3 `task check` passes end-to-end (deferred to the
  end-of-workstream verification pass shared with Changes 1
  and 2).
- [ ] 4.4 Archive
  2026-04-19-i18n-and-log-assertion-expansion after task
  check is green.
