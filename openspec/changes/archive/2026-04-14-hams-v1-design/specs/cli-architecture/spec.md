## ADDED Requirements

### Requirement: CLI framework bootstrap

The `hams` binary SHALL use `urfave/cli` (v3) for command routing with explicit dependency wiring. The `cmd/hams/main.go` entry point SHALL call `cli.Execute()` which constructs the provider registry, registers all builtin providers, and builds the CLI app. Core services (config loader, state manager, hamsfile SDK, provider registry, TUI, logger, OTel, i18n, sudo manager, lock manager, self-updater) SHALL be wired via explicit constructor calls in the CLI initialization path. Provider commands SHALL be registered dynamically from the provider registry so that adding a new provider requires only implementing the `Provider` interface and registering it.

#### Scenario: Application starts with explicit wiring

- **WHEN** the user runs `hams` with any valid command
- **THEN** the CLI framework SHALL construct all required services, register provider commands dynamically, and execute the matched command

#### Scenario: Missing dependency at initialization

- **WHEN** a required service cannot be constructed (e.g., config file unreadable)
- **THEN** the application SHALL fail to start with a clear error message printed to stderr

### Requirement: Command routing syntax

The CLI SHALL follow the routing pattern `hams [global-flags] <command> [args] [--hams-flags] [-- <passthrough>]` where `<command>` is either a builtin command (`apply`, `refresh`, `config`, `store`, `list`, `self-upgrade`) or a provider name followed by a provider verb (e.g., `hams brew install git`). Global flags MUST appear between `hams` and `<command>`. Provider-specific hams flags MUST use the `--hams-` prefix. The `--` separator SHALL force all subsequent arguments to be forwarded to the wrapped command without interpretation.

#### Scenario: Global flag before provider

- **WHEN** the user runs `hams --debug brew install git`
- **THEN** `--debug` SHALL be parsed as a global flag, and `brew install git` SHALL be routed to the Homebrew provider

#### Scenario: Global flag after provider is rejected

- **WHEN** the user runs `hams brew --debug install git`
- **THEN** the CLI SHALL treat `--debug` as part of the provider arguments, not as a hams global flag, and the Homebrew provider SHALL determine how to handle it

#### Scenario: Provider hams-prefixed flag

- **WHEN** the user runs `hams brew install git --hams-tag=dev,cli`
- **THEN** `--hams-tag=dev,cli` SHALL be intercepted by hams as a provider-self flag and SHALL NOT be forwarded to the underlying `brew` command

#### Scenario: Double-dash passthrough

- **WHEN** the user runs `hams brew install git -- --verbose --force`
- **THEN** the `--` separator itself and all subsequent arguments (`--verbose`, `--force`) SHALL be forwarded verbatim to the underlying `brew install` command, preserving the `--` so that the wrapped CLI can distinguish positional arguments from flags

#### Scenario: Provider verb without required argument

- **WHEN** the user runs `hams pnpm install` with no package name
- **THEN** the CLI SHALL reject the command with an error message stating that a package name is required and suggesting `hams apply` for bulk install

### Requirement: Provider self-parsing

Each provider SHALL self-parse its own subcommands, verb routing, and parameter forwarding. The urfave/cli root command SHALL delegate to the provider after extracting global flags and `--hams-` prefixed flags. The provider is responsible for deciding which verbs are supported (e.g., `install`, `remove`, `list`, `enrich`), how arguments map to the wrapped CLI, and which flags to auto-inject (e.g., `-g` for pnpm global, `-y` for apt non-interactive, `@latest` for npm).

#### Scenario: Provider receives only its arguments

- **WHEN** the user runs `hams brew install git --hams-tag=cli -- --verbose`
- **THEN** the Homebrew provider SHALL receive verb `install`, positional arg `git`, the `--` separator, and passthrough arg `--verbose` (i.e., `["install", "git", "--", "--verbose"]`). The provider SHALL NOT see `--hams-tag=cli` in its argument list (it is extracted by the hams framework).

#### Scenario: Provider list differs from native list

- **WHEN** the user runs `hams brew list`
- **THEN** the Homebrew provider SHALL display a diff between `Homebrew.hams.yaml` and the current state, NOT passthrough to `brew list`

### Requirement: Lock file for single-writer enforcement

The system SHALL maintain a lock file at `<store>/.state/<machine-id>/.lock` to enforce single-writer access during any mutating operation (`apply`, `refresh`, provider install/remove). The lock file SHALL contain the PID of the holding process, the full command string, and the acquisition timestamp in a structured format. Before acquiring the lock, the system SHALL check whether the PID in the existing lock file is still alive; if the process is dead, the stale lock SHALL be reclaimed with a warning. The lock SHALL be released on normal exit, signal handling (SIGINT, SIGTERM), and via Fx shutdown hooks.

#### Scenario: Concurrent apply is blocked

- **WHEN** a second `hams apply` is started while a first `hams apply` holds the lock
- **THEN** the second process SHALL exit immediately with a non-zero exit code and an error message that includes the PID, command, and start time of the holding process

#### Scenario: Stale lock from crashed process

- **WHEN** a lock file exists but the PID recorded in it is no longer running
- **THEN** the system SHALL reclaim the lock, emit a warning about the stale lock (including the dead PID and original command), and proceed normally

#### Scenario: Lock released on SIGINT

- **WHEN** the user presses Ctrl+C during `hams apply`
- **THEN** the SIGINT handler SHALL release the lock file before the process exits

### Requirement: Sudo management with startup prompt and heartbeat

When `hams apply` or any operation requiring elevated privileges is invoked, the system SHALL prompt for sudo credentials once at startup using the standard system password prompt. After acquiring credentials, a background goroutine SHALL run `sudo -v` at a regular interval (every 4 minutes) to keep the sudo ticket alive for the duration of the operation. The heartbeat SHALL stop when the Fx application shuts down. Operations that do not require sudo SHALL NOT prompt for credentials.

#### Scenario: Sudo acquired once for full apply

- **WHEN** the user runs `hams apply` and the profile contains providers that require sudo (e.g., apt)
- **THEN** the system SHALL prompt for the sudo password exactly once at the beginning, and all subsequent sudo-requiring operations SHALL execute without re-prompting

#### Scenario: Sudo heartbeat keeps ticket alive

- **WHEN** `hams apply` runs for longer than the system's default sudo timeout (typically 5 minutes)
- **THEN** the background heartbeat SHALL have refreshed the sudo ticket via `sudo -v` so that elevated operations later in the apply do not fail or re-prompt

#### Scenario: No sudo prompt when unnecessary

- **WHEN** the user runs `hams brew install git` (an operation that does not require sudo on macOS Homebrew)
- **THEN** the system SHALL NOT prompt for sudo credentials

#### Scenario: Sudo heartbeat failure

- **WHEN** the `sudo -v` heartbeat fails (e.g., sudo revoked externally)
- **THEN** the system SHALL log a warning and subsequent sudo-requiring operations SHALL fail with a clear error suggesting the user re-run with sudo

### Requirement: Internationalization via locale environment variables

The system SHALL detect the user's locale by reading environment variables in priority order: `LC_ALL`, `LC_CTYPE`, `LANG`. The locale string SHALL be parsed by stripping the encoding suffix (e.g., `.UTF-8`, `.ISO-8859-1`) first, then extracting the language and region components (e.g., `zh_TW.UTF-8` yields language `zh`, region `TW`). The system always enforces UTF-8 internally; the encoding suffix is ignored. If no supported locale is detected or the variables are unset, the system SHALL default to `en_US`. The i18n module SHALL provide a message catalog interface that all user-facing strings (errors, help text, prompts) go through. The v1 release SHALL ship with `en_US` and `zh_CN` translations; the architecture SHALL support additional locales without code changes.

#### Locale file resolution chain

The English locale file (`en.yaml`) SHALL always be loaded as the base. For non-English locales, the system SHALL resolve a single locale file using the following fallback chain (first match wins):

1. **Exact match**: `<lang>-<REGION>.yaml` (e.g., `zh-TW.yaml` for locale `zh_TW`)
2. **Base language**: `<lang>.yaml` (e.g., `zh.yaml`)
3. **Sibling match**: the first `<lang>-*.yaml` file alphabetically (e.g., `zh-CN.yaml` when only `zh-CN.yaml` exists and the locale is `zh_TW`)
4. **No match**: no additional locale file is loaded; rely on English only

At most one non-English locale file SHALL be loaded. The system loads either a full locale resource file or nothing — partial file loading is not supported.

#### i18n key fallback

For any i18n key lookup, if the key is missing from the loaded non-English locale, the system SHALL fall back to the English locale. If the key is also missing from English, the system SHALL return the key string itself as the message (fail-open for robustness).

#### Scenario: English locale detected

- **WHEN** the user's environment has `LANG=en_US.UTF-8`
- **THEN** all CLI output SHALL use English message strings

#### Scenario: Unsupported locale falls back to English

- **WHEN** the user's environment has `LANG=ja_JP.UTF-8` and no Japanese translation exists
- **THEN** the system SHALL fall back to `en_US` and all CLI output SHALL use English

#### Scenario: LC_ALL overrides LANG

- **WHEN** `LC_ALL=zh_CN.UTF-8` and `LANG=en_US.UTF-8`
- **THEN** the system SHALL use the `zh_CN` locale (if supported), as `LC_ALL` takes precedence

#### Scenario: No locale environment variables

- **WHEN** `LC_ALL`, `LC_CTYPE`, and `LANG` are all unset
- **THEN** the system SHALL default to `en_US`

#### Scenario: Non-CN Chinese locale falls back to zh-CN

- **WHEN** the user's environment has `LANG=zh_TW.UTF-8`
- **AND** no `zh-TW.yaml` or `zh.yaml` locale file exists
- **AND** `zh-CN.yaml` exists as the only `zh-*.yaml` file
- **THEN** the system SHALL load `zh-CN.yaml` via the sibling match rule and use Chinese translations

#### Scenario: Encoding suffix is stripped

- **WHEN** the user's environment has `LANG=zh_CN.UTF-8` or `LANG=zh_CN.GB2312`
- **THEN** the system SHALL strip the encoding suffix, parse `zh_CN`, and load `zh-CN.yaml` regardless of the original encoding

### Requirement: Exit code semantics

The CLI SHALL use standardized exit codes to communicate result status to callers (scripts, CI, AI agents). Exit code 0 SHALL indicate success. Exit code 1 SHALL indicate a general error. Exit code 2 SHALL indicate a usage/argument error (invalid flags, missing arguments). Exit code 3 SHALL indicate a lock conflict (another hams process holds the lock). Exit code 4 SHALL indicate a partial failure (some resources succeeded, some failed during apply). Exit code 10 SHALL indicate that sudo is required but was not granted or timed out. Exit codes 11-19 SHALL be reserved for provider-specific errors. Exit codes 126 and 127 SHALL follow POSIX convention (command not executable / command not found).

#### Scenario: Successful apply

- **WHEN** `hams apply` completes with all resources in `ok` state
- **THEN** the process SHALL exit with code 0

#### Scenario: Invalid arguments

- **WHEN** the user runs `hams --nonexistent-flag`
- **THEN** the process SHALL exit with code 2

#### Scenario: Partial failure during apply

- **WHEN** `hams apply` completes with 8 resources `ok` and 2 resources `failed`
- **THEN** the process SHALL exit with code 4

#### Scenario: Lock conflict

- **WHEN** `hams apply` cannot acquire the lock because another process holds it
- **THEN** the process SHALL exit with code 3

#### Scenario: Sudo not granted

- **WHEN** the user cancels the sudo prompt or sudo times out during startup
- **THEN** the process SHALL exit with code 10

### Requirement: AI-agent friendly error format

All error messages SHALL be structured to be parseable by AI agents. Each error SHALL include: (1) a machine-readable error code string (e.g., `LOCK_CONFLICT`, `PROVIDER_NOT_FOUND`, `PACKAGE_NOT_SPECIFIED`), (2) a human-readable description of what went wrong, (3) zero or more suggested fix commands that the user (or AI agent) can run to resolve the issue. Error output SHALL be written to stderr. When `--json` global flag is set, errors SHALL be output as JSON objects with fields `code`, `message`, and `suggestions` (array of strings).

#### Scenario: Error with suggested fix in text mode

- **WHEN** the user runs `hams pnpm install` (missing package name) without `--json`
- **THEN** stderr SHALL contain the error code, a description like "Package name is required for pnpm install", and a suggestion like "Try: hams pnpm install <package-name>"

#### Scenario: Error in JSON mode

- **WHEN** the user runs `hams --json pnpm install`
- **THEN** stderr SHALL contain a JSON object: `{"code": "PACKAGE_NOT_SPECIFIED", "message": "Package name is required for pnpm install", "suggestions": ["hams pnpm install <package-name>", "hams apply (for bulk install from Hamsfile)"]}`

#### Scenario: Provider not found error

- **WHEN** the user runs `hams foo install bar` and no provider named `foo` is registered
- **THEN** the error SHALL include code `PROVIDER_NOT_FOUND`, a message naming the unknown provider, and suggestions listing available providers

### Requirement: Self-upgrade command

The `hams self-upgrade` command SHALL detect the installation channel and perform an upgrade accordingly. Channel detection SHALL first check for a channel marker file at `${HAMS_DATA_HOME}/install-channel` (containing `binary` or `homebrew`). If no marker exists, the system SHALL infer the channel from the binary path: if the path contains `/homebrew/` or `/Cellar/`, the channel is `homebrew`; otherwise, it is `binary`. For the `binary` channel, self-upgrade SHALL fetch the latest release from the GitHub Releases API, download the appropriate platform binary, verify its checksum, and atomically replace the running binary. For the `homebrew` channel, self-upgrade SHALL invoke `brew upgrade hams`. The command SHALL display the current version, the available version, and a confirmation prompt before proceeding (skippable with `--yes`).

#### Scenario: Binary channel upgrade

- **WHEN** the user runs `hams self-upgrade` and the install channel is `binary`
- **THEN** the system SHALL query GitHub Releases for the latest version, display current vs. available version, download the binary matching the current OS/architecture, verify the checksum, and replace the current binary

#### Scenario: Homebrew channel upgrade

- **WHEN** the user runs `hams self-upgrade` and the install channel is `homebrew`
- **THEN** the system SHALL invoke `brew upgrade hams` and report the result

#### Scenario: Already up to date

- **WHEN** the user runs `hams self-upgrade` and the installed version matches the latest release
- **THEN** the system SHALL print a message indicating that hams is already at the latest version and exit with code 0

#### Scenario: Channel detection from binary path

- **WHEN** the hams binary is located at `/opt/homebrew/bin/hams` and no channel marker file exists
- **THEN** the system SHALL infer the `homebrew` channel from the path

### Requirement: Help flag highest priority

The `--help` flag SHALL have the highest priority in command processing. When `--help` is present anywhere in the argument list, no subcommand or provider operation SHALL execute; only the relevant help text SHALL be displayed. The position of `--help` determines which help is shown.

#### Scenario: Help before provider shows hams help

- **WHEN** the user runs `hams --help brew install`
- **THEN** hams SHALL display its own top-level help text (listing global flags, available commands, available providers) and SHALL NOT invoke the Homebrew provider

#### Scenario: Help after provider shows provider help

- **WHEN** the user runs `hams brew --help`
- **THEN** hams SHALL display the Homebrew provider's help text (listing supported verbs, hams-prefixed flags) and SHALL NOT execute any brew operation

#### Scenario: Help after provider verb shows verb help

- **WHEN** the user runs `hams brew install --help`
- **THEN** hams SHALL display help specific to the `brew install` verb (arguments, passthrough flags, hams-prefixed flags) and SHALL NOT execute the install

#### Scenario: Help does not require valid arguments

- **WHEN** the user runs `hams brew install --help` without specifying a package name
- **THEN** help text SHALL be displayed without an error about missing package name

### Requirement: Dry-run mode

The `--dry-run` global flag SHALL cause hams to compute and display all actions it would take without executing any mutations. Dry-run SHALL display each action with its type (install, remove, update, skip), the resource identity, and the command that would be executed. No Hamsfile writes, state file writes, lock acquisitions, or wrapped CLI invocations SHALL occur during dry-run. Dry-run SHALL still perform read-only operations (config loading, Hamsfile parsing, state reading, probe/refresh if needed for diffing).

#### Scenario: Dry-run apply shows planned actions

- **WHEN** the user runs `hams --dry-run apply` and there are 3 packages to install and 1 to remove
- **THEN** the output SHALL list all 4 actions with their types and target resources, and no actual install/remove commands SHALL be executed

#### Scenario: Dry-run does not modify state

- **WHEN** the user runs `hams --dry-run brew install git`
- **THEN** no state file SHALL be written, no Hamsfile SHALL be modified, and `brew install` SHALL NOT be invoked

#### Scenario: Dry-run exits with code 0

- **WHEN** `hams --dry-run apply` completes displaying the plan
- **THEN** the process SHALL exit with code 0 regardless of what the plan contains

### Requirement: Bootstrap flow with apply from repository

The `hams apply --from-repo=<repo>` command SHALL bootstrap a fresh machine by cloning a store repository and applying its configuration. The `<repo>` argument SHALL accept a GitHub `owner/repo` shorthand (auto-prefixed with `https://github.com/`) or a full git URL. The repository SHALL be cloned to `${HAMS_DATA_HOME}/repo/<owner>/<repo>/` using the bundled go-git library (no system git dependency). If the store has no profile matching the current machine, the system SHALL prompt the user interactively to initialize a profile (profile tag and machine-id). After cloning and profile resolution, the system SHALL proceed with a normal `hams apply` flow. If the repository was previously cloned, the system SHALL pull the latest changes before applying.

#### Scenario: First-time bootstrap from GitHub shorthand

- **WHEN** the user runs `hams apply --from-repo=zthxxx/hams-store` on a fresh machine with no prior hams configuration
- **THEN** the system SHALL clone `https://github.com/zthxxx/hams-store` to `${HAMS_DATA_HOME}/repo/zthxxx/hams-store/`, prompt for profile tag and machine-id, and apply the resulting profile

#### Scenario: Bootstrap from full URL

- **WHEN** the user runs `hams apply --from-repo=https://gitlab.com/user/dotfiles.git`
- **THEN** the system SHALL clone the full URL as-is without GitHub prefix modification

#### Scenario: Subsequent apply from same repo pulls latest

- **WHEN** the user runs `hams apply --from-repo=zthxxx/hams-store` and the repo was previously cloned
- **THEN** the system SHALL pull the latest changes from the remote before applying

#### Scenario: Profile initialization prompt

- **WHEN** the cloned store has no profile matching the current machine-id
- **THEN** the system SHALL interactively prompt for a profile tag (e.g., "macOS") and a machine-id (e.g., "MacbookProM5X"), create the profile directory, and proceed with apply

### Requirement: Refresh command

The `hams refresh` command SHALL probe the current environment to update state files with observed reality. Refresh SHALL only probe resources already known in state; it SHALL NOT discover resources installed outside of hams. For each resource in state, the provider's probe method SHALL be called, and the state entry SHALL be updated with current values (version, existence, config value). After refresh, the state SHALL reflect the actual environment, enabling accurate diffing on the next `hams apply`. Refresh SHALL respect `--only` and `--except` flags for provider filtering.

#### Scenario: Refresh updates stale state

- **WHEN** the user runs `hams refresh` and a package recorded in state has been manually upgraded outside of hams
- **THEN** the state file SHALL be updated to reflect the new version, and `checked-at` SHALL be set to the current timestamp

#### Scenario: Refresh detects removed package

- **WHEN** the user runs `hams refresh` and a package in state has been manually uninstalled
- **THEN** the state entry SHALL be updated to reflect that the package is no longer present

#### Scenario: Refresh does not discover new packages

- **WHEN** the user has installed a package via `brew install` directly (not through hams) and runs `hams refresh`
- **THEN** the directly-installed package SHALL NOT appear in the state file

#### Scenario: Refresh with provider filter

- **WHEN** the user runs `hams refresh --only=brew,pnpm`
- **THEN** only the Homebrew and pnpm providers SHALL be probed; other providers' state SHALL remain unchanged

### Requirement: Apply with provider filtering

The `hams apply` command SHALL support `--only=<providers>` and `--except=<providers>` flags to filter which providers are included in the apply operation. The `--only` flag SHALL restrict apply to only the comma-separated list of provider names. The `--except` flag SHALL exclude the comma-separated list of provider names. Specifying both `--only` and `--except` simultaneously SHALL be a usage error (exit code 2). Provider names in these flags SHALL be case-insensitive. An unrecognized provider name SHALL produce an error.

#### Scenario: Apply only specific providers

- **WHEN** the user runs `hams apply --only=brew,pnpm`
- **THEN** only the Homebrew and pnpm providers SHALL execute; all other providers SHALL be skipped

#### Scenario: Apply except specific providers

- **WHEN** the user runs `hams apply --except=apt`
- **THEN** all providers except apt SHALL execute

#### Scenario: Both only and except is an error

- **WHEN** the user runs `hams apply --only=brew --except=apt`
- **THEN** the CLI SHALL exit with code 2 and an error message indicating that `--only` and `--except` are mutually exclusive

#### Scenario: Unknown provider name in filter

- **WHEN** the user runs `hams apply --only=nonexistent`
- **THEN** the CLI SHALL exit with an error naming the unknown provider and listing available providers

#### Scenario: Provider names are case-insensitive

- **WHEN** the user runs `hams apply --only=Brew,PNPM`
- **THEN** the system SHALL match providers `brew` and `pnpm` regardless of case

### Requirement: Config command

The `hams config` command SHALL provide subcommands for reading and writing hams configuration values. `hams config get <key>` SHALL print the current value of the specified configuration key. `hams config set <key> <value>` SHALL write the value to the appropriate config file. Keys that contain sensitive values (e.g., `bark-token`) SHALL be written to `hams.config.local.yaml` or OS keychain, never to git-tracked config files. `hams config list` SHALL display all current configuration values with their sources (global, project, local override). `hams config edit` SHALL open the config file in the user's `$EDITOR`.

#### Scenario: Set a sensitive config value

- **WHEN** the user runs `hams config set bark-token abc123`
- **THEN** the value SHALL be written to `hams.config.local.yaml` (or OS keychain), NOT to `hams.config.yaml`

#### Scenario: Get a config value

- **WHEN** the user runs `hams config get profile-tag`
- **THEN** the system SHALL print the resolved value of `profile-tag` considering all config layers (global, project, local)

#### Scenario: List all config values

- **WHEN** the user runs `hams config list`
- **THEN** the output SHALL show each config key, its value, and which source file it comes from (e.g., "global", "project", "local")

#### Scenario: Edit config in editor

- **WHEN** the user runs `hams config edit` and `$EDITOR` is set to `vim`
- **THEN** the system SHALL open the project-level `hams.config.yaml` in `vim`

### Requirement: Store command

The `hams store` command SHALL provide subcommands for managing the hams store repository. `hams store init` SHALL initialize a new store directory with the standard structure (profile directories, `.gitignore`, `hams.config.yaml`). `hams store status` SHALL display the current store path, profile tag, machine-id, and git status of the store repo. `hams store push` SHALL commit and push pending changes in the store repo to the remote. `hams store pull` SHALL pull the latest changes from the remote.

#### Scenario: Initialize a new store

- **WHEN** the user runs `hams store init` in an empty directory
- **THEN** the system SHALL create `hams.config.yaml`, `.gitignore` (with `.state/` and `*.local.*` patterns), and prompt for an initial profile tag to create the profile directory

#### Scenario: Store status shows current state

- **WHEN** the user runs `hams store status`
- **THEN** the output SHALL display the store path, active profile tag, machine-id, and any uncommitted changes to Hamsfiles

#### Scenario: Push store changes

- **WHEN** the user runs `hams store push` and there are modified Hamsfiles
- **THEN** the system SHALL stage all Hamsfile changes, create a commit with a descriptive message, and push to the remote

### Requirement: List command

The `hams list` command SHALL display all resources managed by hams across all providers (or filtered providers). Each entry SHALL show the provider name, resource identity (package name or URN), current state status (`ok`, `failed`, `pending`, `removed`), and version (if applicable). The output SHALL be grouped by provider. The `--only` and `--except` flags SHALL filter by provider. A `--status=<status>` flag SHALL filter by resource status. When `--json` is set, the output SHALL be a JSON array of resource objects.

#### Scenario: List all managed resources

- **WHEN** the user runs `hams list`
- **THEN** the output SHALL display all resources from all providers, grouped by provider, with status and version information

#### Scenario: List with provider filter

- **WHEN** the user runs `hams list --only=brew`
- **THEN** only Homebrew-managed resources SHALL be displayed

#### Scenario: List failed resources only

- **WHEN** the user runs `hams list --status=failed`
- **THEN** only resources with `failed` status SHALL be displayed across all providers

#### Scenario: List in JSON format

- **WHEN** the user runs `hams list --json`
- **THEN** the output SHALL be a JSON array where each element contains `provider`, `name`, `status`, `version`, and other relevant fields

### Requirement: Global flags definition

The following global flags SHALL be recognized when placed between `hams` and the command: `--debug` (enable debug-level logging), `--dry-run` (show planned actions without executing), `--json` (output in JSON format for machine parsing), `--no-color` (disable ANSI color codes), `--config=<path>` (override config file path), `--store=<path>` (override store directory path), `--profile=<tag>` (override active profile tag), `--help` (display help), `--version` (display version). Global flags SHALL NOT be forwarded to providers or wrapped commands under any circumstances.

#### Scenario: Version flag

- **WHEN** the user runs `hams --version`
- **THEN** the system SHALL print the version string (including git commit hash and build date) and exit with code 0

#### Scenario: No-color flag

- **WHEN** the user runs `hams --no-color apply`
- **THEN** all output SHALL be free of ANSI escape codes

#### Scenario: Config path override

- **WHEN** the user runs `hams --config=/tmp/test-config.yaml apply`
- **THEN** the system SHALL load configuration from `/tmp/test-config.yaml` instead of the default location

#### Scenario: Store path override

- **WHEN** the user runs `hams --store=/tmp/test-store apply`
- **THEN** the system SHALL use `/tmp/test-store` as the store directory for Hamsfile and state resolution

#### Scenario: Profile override

- **WHEN** the user runs `hams --profile=openwrt apply`
- **THEN** the system SHALL use the `openwrt` profile directory within the store, regardless of the configured default profile

### Requirement: Provider-level `--hams-local` flag

When `--hams-local` is specified after a provider command (e.g., `hams brew install htop --hams-local`), the resource entry SHALL be written to `<Provider>.hams.local.yaml` instead of the main `<Provider>.hams.yaml`. This flag applies only to write operations (install/remove). All read operations (`hams apply`, `hams <provider> list`, etc.) SHALL always read and merge BOTH `.hams.yaml` and `.hams.local.yaml` regardless of whether `--hams-local` is specified.

#### Scenario: Install with --hams-local writes to local file

- **WHEN** the user runs `hams brew install htop --hams-local`
- **THEN** `htop` SHALL be written to `Homebrew.hams.local.yaml` (not `Homebrew.hams.yaml`)
- **AND** `brew install htop` SHALL execute normally

#### Scenario: Remove with --hams-local writes to local file

- **WHEN** the user runs `hams brew remove htop --hams-local`
- **THEN** `htop` SHALL be removed from `Homebrew.hams.local.yaml` (not `Homebrew.hams.yaml`)
- **AND** `brew uninstall htop` SHALL execute normally

#### Scenario: List always reads both files

- **WHEN** the user runs `hams brew list` (without `--hams-local`)
- **THEN** the output SHALL include entries from both `Homebrew.hams.yaml` and `Homebrew.hams.local.yaml`, merged according to the provider's merge strategy

#### Scenario: Apply reads both files

- **WHEN** `hams apply` runs
- **THEN** for each provider, the system SHALL merge `<Provider>.hams.yaml` with `<Provider>.hams.local.yaml` to compute the full desired state

### Requirement: Apply command default behavior

The `hams apply` command without additional flags SHALL perform the following sequence: (1) acquire the single-writer lock, (2) prompt for sudo if any provider requires it, (3) load all Hamsfiles for the active profile, (4) perform a refresh (probe all resources in state) unless `--no-refresh` is specified, (5) diff desired state (Hamsfiles) against observed state, (6) execute the resulting action plan (installs, removes, updates) in provider priority order, (7) update state files, (8) release the lock. By default, apply SHALL include refresh (`--refresh` is the default; `--no-refresh` skips it).

#### Scenario: Apply with implicit refresh

- **WHEN** the user runs `hams apply` without `--no-refresh`
- **THEN** the system SHALL probe all resources in state before computing the diff

#### Scenario: Apply with no-refresh

- **WHEN** the user runs `hams apply --no-refresh`
- **THEN** the system SHALL skip the probe phase and diff against the last known state

#### Scenario: Apply retries previously failed resources

- **WHEN** `hams apply` runs and state contains resources with `failed` status from a previous run
- **THEN** the system SHALL include those failed resources in the action plan for retry

#### Scenario: Apply full sequence completes

- **WHEN** `hams apply` runs successfully to completion
- **THEN** the lock SHALL be released, all state files SHALL be updated, and the exit code SHALL be 0
