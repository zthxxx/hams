# Tasks — 2026-04-16-defer-hooks-and-otel

Status: the deferral framing is no longer accurate. Both features got
WIRED (not deferred) in subsequent cycles, and the main specs now
document the actual shipped behavior.

- `internal/hamsfile/hooks.go` + `internal/provider/hooks_parse.go`
  populate `Action.Hooks` from hamsfile `hooks:` blocks. Every builtin
  provider's `Plan()` ends with `PopulateActionHooks(...)`. Integration
  test: `internal/provider/hooks_integration_test.go`.
- `runApply` / `runRefresh` wrap operations in root OTel spans when
  `HAMS_OTEL=1`. `LocalFileExporter` writes under
  `${HAMS_DATA_HOME}/otel/{traces,metrics}/` on session shutdown.
- Cycle 200 added a `slog.Warn` for unwired `defer: true` hooks so
  copy-pasted deferred blocks no longer silently no-op (commit
  `13afec4`).

The final recorded state lives at `openspec/specs/cli-architecture/spec.md`
lines 699–756 ("CLI Architecture — Spec Delta (hooks implemented; OTel
still deferred)"). Note: the heading text is mildly misleading — OTel is
no longer deferred either; both the hooks section (lines 703–726) and
the OTel section (lines 728–756) describe the lifted/wired behavior.

- [x] Spec deltas for hooks deferral — obviated by wiring; replaced with
      "lifted" language in the main cli-architecture spec.
- [x] Spec deltas for OTel deferral — obviated by `HAMS_OTEL` opt-in
      implementation; replaced with opt-in requirement + scenarios in
      the main cli-architecture spec.
- [x] Manual spec edits — already applied (see cli-architecture/spec.md
      lines 699–756).
- [x] `task check` passes — verified 2026-04-17, 32/32 packages PASS.
- [x] Archive this change folder at release time — archived 2026-04-17 in the v1 cleanup pass.
