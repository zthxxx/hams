# Tasks — 2026-04-18-cli-i18n-catalog

## 1. Message catalog skeleton

- [x] 1.1 Create `internal/i18n/keys.go` with message-ID constants
      grouped by capability (`cli.err.*`, `apply.status.*`,
      `bootstrap.*`, `store.status.label.*`). 18 keys.
- [x] 1.2 Document the naming convention
      (`<capability>.<component>.<short-id>`) and the closed
      component set (err | usage | prompt | status | suggestion) in
      the file-level comment.

## 2. Locale coverage

- [x] 2.1 Expand `internal/i18n/locales/en.yaml` from 1 entry to
      19 entries, one per key in keys.go plus the legacy
      `app.title` entry.
- [x] 2.2 Translate all 19 entries into
      `internal/i18n/locales/zh-CN.yaml`. Labels for
      `hams store status` in particular render naturally
      (Store 路径 / Profile 标签 / 机器 ID).

## 3. Call-site migration (proof of pipeline)

- [x] 3.1 Migrate 3 flag-conflict UsageErrors in
      `internal/cli/apply.go` (bootstrap / from-repo / only-except).
- [x] 3.2 Migrate the `--tag` / `--profile` conflict error in
      `internal/config/resolve.go`.
- [x] 3.3 Migrate the dry-run apply preview in
      `internal/cli/apply.go` to `i18n.T(ApplyStatusDryRunPreview)`.
- [x] 3.4 Migrate the profile-missing bootstrap prompt
      (`Not Found Profile in config, init it at first`) to
      `i18n.T(BootstrapStatusProfileMissing)`.
- [x] 3.5 Migrate the non-TTY missing-fields error to
      `i18n.Tf(BootstrapErrNotConfigured, {"Missing": …})` for the
      template-data path.
- [x] 3.6 Migrate the seven `hams store status` labels in
      `internal/cli/commands.go` to the corresponding
      `StoreStatusLabel*` keys.

## 4. Lazy init

- [x] 4.1 Wrap `i18n.Init` in a `sync.Once` via `ensureInitialized`;
      call it from every `T` / `Tf` entry. Tests and short-lived
      commands that don't route through `Execute()` now get
      translated output without manual `Init()` calls.

## 5. Logs stay English (explicit exclusion)

- [x] 5.1 Keep `slog.Info("auto-initialized global config", …)` in
      `internal/cli/apply.go` as a bare literal with a comment
      documenting the "log records do not require i18n" stance.
- [x] 5.2 Keep `slog.Info("hams session started", …)` bare for the
      same reason.

## 6. Verification

- [x] 6.1 `task check` passes (lint + unit + integration). E2E
      via act fails on artifact upload (known act limitation,
      unrelated to this change).
- [x] 6.2 All 13 migrated sites exercised by existing unit tests;
      the English fallback via lazy init keeps test assertions
      byte-identical to pre-change output.

## 7. Follow-ups (out of scope for this change)

- [ ] Migrate the remaining ~100 `fmt.Print*` sites in
      `internal/cli/` that emit user-facing English. Same pattern:
      add a key, translate in en.yaml + zh-CN.yaml, replace the
      literal with `i18n.T(key)`. Incremental PRs.
- [ ] Migrate the remaining ~38 `NewUserError` sites in
      `internal/cli/`. Same pattern.
- [ ] Reach into provider-layer user-facing errors (usage errors
      returned from `HandleCommand`). The provider-layer migration
      is coupled to the shared-abstractions change; migrating
      provider errors alongside the dispatcher helpers avoids
      doing the same strings twice.
