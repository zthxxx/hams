# 2026-04-16-defer-tui-and-notify

## Why

A cycle-7 architectural audit (continuing the pattern started in
`2026-04-16-defer-hooks-and-otel`) surfaced **two more scaffolded-
but-unwired features** matching the shape of the already-fixed
hooks/OTel deferrals:

### 1. TUI alternate-screen rendering is dead plumbing

- `internal/tui/run.go:38` defines `RunApplyTUI(logPath, providers)`. ✓
- `internal/tui/popup.go:23` defines `PopupModel` for interactive provider stdin. ✓
- `internal/tui/picker.go` defines the tag-picker model. ✓
- `internal/tui/collapsible.go` defines collapsible log sections. ✓
- **`runApply` in `internal/cli/apply.go` never calls `tui.RunApplyTUI`.** A grep across `internal/cli/` finds zero references to the `tui` package.
- **`PopupModel` has zero callers.** The "interactive popup for blocking stdin operations" requirement (`tui-logging/spec.md` line 34) is unimplemented at the integration boundary.
- **User impact**: a user who runs `hams apply` on a TTY sees plain stderr/stdout output, not the documented alternate-screen TUI with sticky header / collapsible log section / popup overlay. The shipped UX is "log lines" — fine for v1 but does not match the spec.

### 2. Notification system is dead plumbing

- `internal/notify/notify.go` defines `Channel` interface, `Manager`, `NewManager(barkToken)`, etc. ✓
- Bark + terminal-notifier channels implemented (~150 lines). ✓
- **`Manager.Send` has zero callers in `internal/cli/`.** No "apply complete" notification fires.
- **User impact**: spec requires `hams apply` finish to send a notification (terminal-notifier + Bark if configured). v1 ships without this — users on long applies who switch away from the terminal do not get notified when the run finishes.

## What Changes

Both features are **deferred to v1.1**, mirroring the architect call
for `--hams-lucky` (commit `f4c0f20`), hooks (commit `1479129`
later un-deferred), and OTel (commit `1cfd54e` later un-deferred).

This change does NOT remove any code. The TUI + notify scaffolding
stays in place exactly as shipped — v1.1 will plug into it. Spec
deltas only.

### Spec deltas (this change)

- `openspec/specs/tui-logging/spec.md` — add a v1-status note to the
  alternate-screen + popup + notification requirements explaining
  the silent-no-op shipped behavior.

### Code changes (this change)

**None.** The scaffolding is preserved for v1.1 to wire in.

### Why deferred (not removed)

- TUI package: ~500 lines of working BubbleTea code across 7 files.
- Notify package: ~150 lines of multi-channel implementation.
- Both would need full re-implementation if deleted.
- Documenting the gap matches the precedent set by lucky/hooks/OTel.

## Impact

- Affected specs: `tui-logging` (alternate-screen + popup + notification deltas).
- Affected code: none.
- Affected tests: existing `internal/tui/tui_test.go` + `internal/notify/*_test.go` keep passing — they test in isolation, which is correctly scoped.
- User-visible: no behavior change in v1. Spec now honestly documents the gap.
