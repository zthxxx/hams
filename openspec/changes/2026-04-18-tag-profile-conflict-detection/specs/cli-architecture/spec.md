# Spec delta: cli-architecture — `--tag` / `--profile` conflict detection

## ADDED Requirement: `--tag` and `--profile` SHALL NOT be urfave/cli aliases

`--tag` is the canonical flag for the active profile tag. `--profile` is the legacy alias, retained for back-compat. The two SHALL be registered as separate `cli.StringFlag` entries in `globalFlagDefs()` — NOT as `Aliases` of a single flag — so that a CLI invocation supplying both can be detected.

`provider.GlobalFlags` SHALL carry both fields:

- `Tag string` — populated from `--tag`.
- `Profile string` — populated from `--profile`.

Every CLI action that needs the effective tag value SHALL call `config.ResolveCLITagOverride(flags.Tag, flags.Profile)` before further processing. The resolver:

- Returns `""` when both are empty.
- Returns the single non-empty value when only one is supplied.
- Returns the shared value when both are supplied with the same string.
- Returns a `hamserr.UserFacingError` with `ExitUsageError` and the i18n key `cli.err.tag-profile-conflict` when both are supplied with different values.

#### Scenario: user types only `--tag=macOS`

- **Given** `hams apply --tag=macOS`
- **When** the CLI parses flags
- **Then** `flags.Tag == "macOS"`, `flags.Profile == ""`, and `ResolveCLITagOverride` returns `"macOS"`.

#### Scenario: user types only `--profile=linux` (legacy)

- **Given** `hams apply --profile=linux`
- **When** the CLI parses flags
- **Then** `flags.Tag == ""`, `flags.Profile == "linux"`, and `ResolveCLITagOverride` returns `"linux"`.

#### Scenario: user supplies both flags, same value

- **Given** `hams apply --tag=macOS --profile=macOS`
- **When** `ResolveCLITagOverride` runs
- **Then** it returns `"macOS"` with no error.

#### Scenario: user supplies conflicting flags

- **Given** `hams apply --tag=macOS --profile=linux`
- **When** `ResolveCLITagOverride` runs
- **Then** it returns a `UserFacingError` carrying the message localized from `cli.err.tag-profile-conflict`, exit code `ExitUsageError`, and the hint `"Remove either --tag=macOS or --profile=linux"`.

## ADDED Requirement: GlobalFlags stdio DI seams

`provider.GlobalFlags` SHALL expose `Out io.Writer` + `Err io.Writer` fields with corresponding `Stdout()` + `Stderr()` accessor methods. When the fields are nil (production), the accessors return `os.Stdout` / `os.Stderr`. Tests running under `t.Parallel()` SHALL inject `bytes.Buffer` sinks via these fields so subtests do not race on the process-global stdio.

#### Scenario: dry-run preview is captured in a test

- **Given** a test sets `flags.Out = &bytes.Buffer{}` before invoking a provider's HandleCommand in dry-run mode
- **When** the provider calls `fmt.Fprintln(flags.Stdout(), "[dry-run] Would install: ...")`
- **Then** the buffer receives the line; `os.Stdout` is untouched; other `t.Parallel()` subtests observe their own isolated buffers.
