## ADDED Requirements

### Requirement: Alternate-Screen TUI Layout — Deferred to v1.1

> **v1 status (as of 2026-04-16):** The TUI scaffolding at `internal/tui/` (~500 lines of BubbleTea models) is fully built and unit-tested but **never invoked from the CLI in v1**. A grep for `tui\.` across `internal/cli/` returns zero matches; `runApply` writes plain log lines via `slog` even on a TTY. The scenarios below describe v1.1 behavior. v1 ships with plain log-line output. See `openspec/changes/2026-04-16-defer-tui-and-notify/` for the deferral rationale.

The system SHALL render a BubbleTea-based alternate-screen terminal UI during `hams apply` and other long-running operations when stdout is a TTY.

The TUI layout SHALL consist of the following regions, top to bottom:

1. **Sticky header**: displays the active log file path using `~/` prefix (never the full username path).
2. **Provider step progress**: shows current provider index and total (e.g., `[3/12] Homebrew`).
3. **Current operation line**: displays the resource currently being processed (e.g., `Installing git...`).
4. **Collapsible log output section**: shows scrollable log output from the current provider/resource. The section SHALL be collapsible via a keyboard shortcut. When collapsed, only the last N lines (configurable, default 3) SHALL be visible.

The TUI SHALL occupy the full alternate screen and restore the original terminal state on exit.

#### Scenario: TUI renders on interactive TTY

- **WHEN** `hams apply` is invoked and stdout is a TTY
- **THEN** the system SHALL enter the alternate screen and render the sticky header with the log file path using `~/` prefix, provider step progress, current operation, and collapsible log output section

#### Scenario: Sticky header shows log file path

- **WHEN** the TUI is active
- **THEN** the sticky header SHALL display the path to the current log file using the `~/` prefix (e.g., `~/.local/share/hams/2026-04/hams.202604.log`), never the expanded home directory path

#### Scenario: Provider step progress updates

- **WHEN** hams transitions from one provider to the next during apply
- **THEN** the progress indicator SHALL update to reflect the current provider index and total count (e.g., `[2/12] pnpm` changing to `[3/12] npm`)

#### Scenario: Log output section toggles collapse

- **WHEN** the user presses the collapse keyboard shortcut while the log output section is expanded
- **THEN** the log output section SHALL collapse to show only the last 3 lines of output
- **WHEN** the user presses the collapse keyboard shortcut again
- **THEN** the log output section SHALL expand to show the full scrollable log output

### Requirement: Interactive Popup for Blocking Stdin Operations — Deferred to v1.1

> **v1 status (as of 2026-04-16):** Same status as the alternate-screen TUI deferral above. `internal/tui/popup.go` defines `PopupModel` but has zero callers. Forward-looking — no v1 provider needs interactive stdin.

The system SHALL provide a tmux-popup-style overlay within the alternate-screen TUI for operations that require interactive stdin (e.g., signin flows, OAuth authorization, manual confirmation prompts).

Providers SHALL NOT directly read from stdin. Instead, a provider SHALL call the hams interactive API to request a popup session. The hams runtime SHALL then display the popup, route stdin/stdout to the subprocess within the popup, and dismiss the popup when the subprocess exits.

All log output and user interaction for the blocking operation SHALL be contained within the popup. The main TUI behind the popup SHALL remain visible but non-interactive while the popup is active.

#### Scenario: Provider triggers interactive popup for OAuth

- **WHEN** a provider needs user interaction (e.g., `mas signin` requiring Apple ID credentials) and calls the hams interactive API
- **THEN** the system SHALL display a popup overlay within the TUI, route stdin to the interactive subprocess, and show the subprocess output within the popup bounds

#### Scenario: Popup dismissal on subprocess exit

- **WHEN** the interactive subprocess within a popup exits (success or failure)
- **THEN** the system SHALL dismiss the popup overlay and resume normal TUI rendering and provider execution

#### Scenario: Notification fires on blocking interactive action

- **WHEN** a popup is triggered for a blocking interactive action
- **THEN** the system SHALL send a notification (via the notification system) to alert the user that manual input is required

### Requirement: Notification System — Deferred to v1.1

> **v1 status (as of 2026-04-16):** `internal/notify/` defines the multi-channel notification scaffolding (terminal-notifier + Bark) but `Manager.Send` has zero callers in `internal/cli/`. v1 `hams apply` completion does NOT trigger any notification. v1.1 will wire `Manager.Send` at apply-completion + popup-trigger points.

The system SHALL provide a notification system with the following channels:

1. **terminal-notifier**: mandatory, always enabled, not user-configurable. SHALL be used on macOS. On Linux, the system SHALL use `notify-send` or equivalent.
2. **Bark app**: optional iOS push notification. Tokens SHALL be stored in `hams.config.local.yaml` (gitignored) or OS keychain via the keyring library. Token management SHALL be available via `hams config set bark-token <token>`.
3. **Discord** (future): optional webhook-based notification. Not implemented in v1 but the notification channel interface SHALL support adding it.

All notification channel tokens/keys MUST follow the universal secret decoupling principle: actual secret values SHALL never appear in git-tracked config files. They MUST be stored either in `*.local.*` config files (gitignored) or in the OS keychain via keyring (`kind: application password`).

The notification trigger points SHALL be abstracted behind an internal notification event interface for future extensibility. The set of trigger events is NOT user-configurable.

#### Scenario: Notification on apply completion

- **WHEN** `hams apply` finishes (success, partial failure, or full failure)
- **THEN** the system SHALL send a notification via terminal-notifier (and Bark if configured) indicating the result status and a summary (e.g., `hams apply: 45 ok, 2 failed`)

#### Scenario: Notification on blocking interactive action

- **WHEN** a provider triggers a blocking interactive popup requiring user attention
- **THEN** the system SHALL send a notification via terminal-notifier (and Bark if configured) indicating that user input is required, including the provider name and operation

#### Scenario: Bark token configuration

- **WHEN** the user runs `hams config set bark-token <token>`
- **THEN** the system SHALL store the token in `hams.config.local.yaml` or OS keychain and enable Bark push notifications for subsequent trigger events

#### Scenario: Bark not configured

- **WHEN** no Bark token is configured
- **THEN** the system SHALL send notifications only via terminal-notifier without error or warning about missing Bark configuration

### Requirement: Log to File

The system SHALL always log to a file. File logging MUST NOT be disableable by the user.

The log file path SHALL be `${HAMS_DATA_HOME}/<YYYY-MM>/hams.YYYYMM.log` where `<YYYY-MM>` and `YYYYMM` correspond to the current month. This provides monthly log rotation by directory and filename convention.

The log file SHALL contain structured log entries with timestamp, log level, provider name (if applicable), resource identifier (if applicable), and message.

#### Scenario: Log file created on first invocation of the month

- **WHEN** `hams apply` is invoked and no log file exists for the current month
- **THEN** the system SHALL create the directory `${HAMS_DATA_HOME}/<YYYY-MM>/` and the log file `hams.YYYYMM.log` within it

#### Scenario: Log file appended on subsequent invocations

- **WHEN** `hams apply` is invoked and a log file for the current month already exists
- **THEN** the system SHALL append to the existing log file, never overwrite it

#### Scenario: Log file path displayed with tilde prefix

- **WHEN** the log file path is displayed anywhere in the TUI or CLI output
- **THEN** the path SHALL use the `~/` prefix (e.g., `~/.local/share/hams/2026-04/hams.202604.log`), never the expanded `/Users/<username>/` or `/home/<username>/` form

### Requirement: Third-Party Tool Session Logs

The system SHALL capture stdout and stderr from third-party tool invocations (e.g., `brew install`, `pnpm add`) into separate session log files.

The session log path SHALL be `${HAMS_DATA_HOME}/<YYYY-MM>/provider/<provider>.YYYYMMDDTHHmmss.session.log` where the timestamp reflects the session start time.

Each session log SHALL be linked from the main hams log file by a unique session ID. The main log SHALL contain an entry with the session ID and the session log file path when a provider session starts.

#### Scenario: Third-party tool output captured to session log

- **WHEN** hams invokes a provider's underlying CLI tool (e.g., `brew install git`)
- **THEN** the system SHALL write the tool's stdout and stderr to a session log file at `${HAMS_DATA_HOME}/<YYYY-MM>/provider/<provider>.YYYYMMDDTHHmmss.session.log`

#### Scenario: Session log linked from main log

- **WHEN** a provider session starts
- **THEN** the main hams log SHALL contain a reference entry with the session ID and the path to the session log file (using `~/` prefix)

#### Scenario: Multiple sessions for the same provider

- **WHEN** a provider is invoked multiple times within the same apply run (e.g., multiple `brew install` calls)
- **THEN** each invocation SHALL produce a distinct session log file with a unique timestamp in the filename

### Requirement: Output Path Tilde Prefix

All file paths displayed to the user (in TUI, CLI output, log messages, notifications, error messages) SHALL use the `~/` prefix to represent the user's home directory. The system SHALL NOT display the fully expanded home directory path (e.g., `/Users/john/` or `/home/john/`).

This requirement applies to:
- Log file paths in the TUI sticky header
- Session log paths in the main log
- Error messages referencing file paths
- Notification messages containing file paths
- `--help` output examples containing file paths

#### Scenario: Path displayed in TUI header

- **WHEN** the TUI renders the sticky header with the log file path
- **THEN** the path SHALL begin with `~/` (e.g., `~/.local/share/hams/2026-04/hams.202604.log`)

#### Scenario: Path in error message

- **WHEN** the system reports an error involving a file path under the user's home directory
- **THEN** the error message SHALL display the path with the `~/` prefix

### Requirement: Debug Log Level

The system SHALL support a `--debug` global flag that enables verbose debug-level logging.

When `--debug` is active:

1. The log file SHALL include debug-level entries in addition to the default info-level entries. Debug entries SHALL include: provider internal decision traces, state diff details, Hamsfile parse events, DAG resolution steps, hook lifecycle events, LLM subprocess invocations and responses, and notification dispatch results.
2. The TUI log output section SHALL display debug-level messages inline, visually distinguished from info-level messages (e.g., dimmed text color or `[DEBUG]` prefix).
3. In non-TUI mode, debug-level messages SHALL be printed to stderr with a `[DEBUG]` prefix.

The default log level (without `--debug`) SHALL be info. Info level SHALL include: operation start/completion, provider transitions, resource status changes, errors, and warnings.

#### Scenario: Debug flag enables verbose file logging

- **WHEN** the user runs `hams apply --debug`
- **THEN** the log file SHALL contain debug-level entries including provider decision traces, state diff details, DAG resolution steps, and hook lifecycle events, in addition to all info-level entries

#### Scenario: Debug output in TUI mode

- **WHEN** `--debug` is active and the TUI is rendering
- **THEN** debug-level messages SHALL appear in the log output section with visual distinction (dimmed color or `[DEBUG]` prefix) from info-level messages

#### Scenario: Debug output in non-TUI mode

- **WHEN** `--debug` is active and stdout is not a TTY
- **THEN** debug-level messages SHALL be printed to stderr with a `[DEBUG]` prefix

#### Scenario: Default log level excludes debug entries from file

- **WHEN** `hams apply` is run without `--debug`
- **THEN** the log file SHALL contain only info-level and above entries (no debug-level entries)

### Requirement: Non-TUI Fallback Mode

When stdout is not a TTY (e.g., piped output, CI environment, redirected to file), the system SHALL NOT enter the alternate screen or render the BubbleTea TUI.

Instead, the system SHALL output plain text log lines to stdout. Each line SHALL include a timestamp, log level, provider name (if applicable), and message. The format SHALL be machine-parseable (structured text, one event per line).

Progress updates SHALL be emitted as log lines rather than in-place terminal updates. The system SHALL NOT emit ANSI escape codes or cursor movement sequences in non-TUI mode.

#### Scenario: Piped output falls back to plain text

- **WHEN** `hams apply` is invoked with stdout piped (e.g., `hams apply | tee output.log`)
- **THEN** the system SHALL output plain text log lines to stdout without ANSI escape codes or alternate screen sequences

#### Scenario: CI environment uses plain text mode

- **WHEN** `hams apply` is invoked in a non-interactive environment (no TTY on stdout)
- **THEN** the system SHALL emit one structured log line per event to stdout, including timestamp, level, provider, and message

#### Scenario: Interactive popup unavailable in non-TUI mode

- **WHEN** a provider requests an interactive popup in non-TUI mode
- **THEN** the system SHALL log a warning that interactive input is required but no TTY is available, and the provider operation SHALL fail with a clear error message indicating the operation requires an interactive terminal

### Requirement: Graceful Ctrl+C Shutdown

The system SHALL handle SIGINT (Ctrl+C) gracefully during all operations.

On receiving SIGINT:

1. The currently executing provider operation SHALL be cancelled via Go context cancellation.
2. The system SHALL wait for the current operation to reach a safe stopping point (with a timeout of 5 seconds, after which it force-terminates).
3. The state file SHALL be updated to reflect the status of each resource: resources that completed successfully SHALL be marked `ok`, the interrupted resource SHALL be marked `pending`, and unprocessed resources SHALL retain their prior state.
4. The system SHALL print a summary to the TUI (or stdout in non-TUI mode) showing: how many resources were completed successfully, which resource was interrupted, and how many resources remain unprocessed.
5. The TUI SHALL exit the alternate screen and restore the terminal to its original state before the summary is printed.

A second SIGINT received during the graceful shutdown period SHALL force-terminate the process immediately.

#### Scenario: Single Ctrl+C during apply

- **WHEN** the user presses Ctrl+C during `hams apply`
- **THEN** the system SHALL cancel the current operation, save state (marking the interrupted resource as `pending`), exit the alternate screen, and print a summary of completed/interrupted/remaining resources

#### Scenario: State preserved on interrupt

- **WHEN** `hams apply` is interrupted by Ctrl+C after completing 5 of 10 resources
- **THEN** the state file SHALL record the 5 completed resources as `ok`, the interrupted resource as `pending`, and the remaining 4 resources SHALL retain their prior state

#### Scenario: Double Ctrl+C force terminates

- **WHEN** the user presses Ctrl+C twice within the graceful shutdown period
- **THEN** the system SHALL force-terminate immediately without waiting for the current operation to complete

#### Scenario: Terminal restored on interrupt

- **WHEN** Ctrl+C is pressed while the TUI is active
- **THEN** the system SHALL exit the alternate screen, restore terminal settings (cursor visibility, raw mode disabled), and print the shutdown summary to the restored terminal
