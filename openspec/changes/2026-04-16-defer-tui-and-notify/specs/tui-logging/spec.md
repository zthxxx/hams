# tui-logging — Spec Delta (TUI + notify defer)

## MODIFIED Requirements

### Requirement: Alternate-Screen TUI Layout — Deferred to v1.1

The TUI scaffolding at `internal/tui/` (BubbleTea models for the apply progress view, picker, popup, collapsible log section — ~500 lines across 7 files) is **fully built and unit-tested in isolation but never invoked from the CLI in v1**.

A grep for `tui\.` across `internal/cli/` returns zero matches. `runApply` in `internal/cli/apply.go` writes to plain stderr/stdout via `slog`; no alternate-screen rendering occurs even when stdout is a TTY.

#### Scenario: v1 apply uses plain log lines, not alternate-screen TUI

- **WHEN** a user runs `hams apply` on a TTY in v1
- **THEN** progress SHALL be reported via plain `slog` lines to stderr
- **AND** no alternate-screen rendering, sticky header, or collapsible log section SHALL appear
- **AND** the apply SHALL complete normally with the same exit codes documented elsewhere in this spec.

### Requirement: Interactive Popup for Blocking Stdin Operations — Deferred to v1.1

Same status as the alternate-screen TUI. `internal/tui/popup.go` defines `PopupModel` and supporting types but has zero callers. The "providers SHALL NOT directly read from stdin" rule is not enforced — v1 providers that need stdin (currently none) would block the parent process.

#### Scenario: v1 has no provider needing the popup mechanism

- **WHEN** a v1 provider runs to completion
- **THEN** no popup overlay is displayed (none of the 15 builtins call the interactive API; the spec rule is forward-looking).

### Requirement: Notification System — Deferred to v1.1

`internal/notify/` defines the `Channel` interface, `Manager`, terminal-notifier + Bark channels. `Manager.Send` has zero callers in `internal/cli/`. No "apply complete" notification fires.

#### Scenario: v1 apply completion does not send a notification

- **WHEN** `hams apply` finishes (success, partial failure, or full failure) in v1
- **THEN** no terminal-notifier or Bark notification SHALL be sent
- **AND** the user is informed solely via stdout summary line (`hams apply complete: ...`) and the process exit code.

## Why deferred (not removed)

- TUI package: ~500 lines of BubbleTea models + tests (`internal/tui/`).
- Notify package: ~150 lines of multi-channel impl (`internal/notify/`).
- Both would need full re-implementation if deleted; documenting the gap matches the lucky/hooks/OTel deferral precedent.
- v1.1 will wire TUI into `runApply` (gated on TTY detection) and call `Manager.Send` at apply-completion + popup-trigger points.
