# CLI Architecture — cli-modularization deltas

## ADDED Requirements

### Requirement: Auto-init helpers MUST live in autoinit.go

The first-run auto-init helpers MUST live in `internal/cli/autoinit.go`,
not be inlined into the apply pipeline or other command orchestration
files. The `internal/cli/apply.go`,
`internal/cli/commands.go`, and `internal/cli/provider_cmd.go`
orchestration files MUST NOT inline auto-init logic; they MUST
call into the autoinit module.

Rationale: pre-extraction `apply.go` had grown to 1559 lines and
mixed apply orchestration with first-run scaffolding. Splitting
the first-run module out:

1. Makes the auto-init UX grep-locatable by name (`grep -rn
   ensureProfileConfigured` resolves to one file).
2. Lets a future "apply on a fresh machine" UX change touch only
   `autoinit.go` + its dedicated test file, never the
   apply-pipeline orchestration.
3. Mirrors the analogous extraction on `origin/dev`'s
   `internal/cli/autoinit.go` (commit `7469b73`), but preserves
   `local/loop`'s superior `WarnIfDefaultsUsed` separation
   (commit `3cafb27`) so `--help` / `--version` stay fully silent.

#### Scenario: New auto-init UX change touches one file

WHEN a developer needs to add a new first-run prompt (e.g.,
"choose between zsh and bash for hooks")
THEN the change SHALL land in `internal/cli/autoinit.go` and
`internal/cli/autoinit_test.go` only — the apply pipeline,
provider_cmd, and refresh entry points SHALL NOT need edits.

#### Scenario: Auto-init test coverage is complete

WHEN `go test -race ./internal/cli/...` runs
THEN `autoinit_test.go` SHALL execute at minimum 8 dedicated
unit tests covering: fresh-machine seed, no-op-when-config-exists,
non-TTY without CLI tag fails, --tag and --profile alias
equivalence, conflict rejection, statFile missing/present/
directory branches.

### Requirement: Every --tag/--profile entry point MUST validate conflict

Every CLI command entry point MUST validate `--tag` and
`--profile` flags MUST validate the conflict via
`config.ResolveCLITagOverride` (or the
`enforceTagProfileConsistency` thin wrapper) BEFORE the first
`config.Load` call. Silent precedence ("--profile wins, --tag
ignored") is forbidden.

The currently-required entry points are: `apply`, `refresh`,
`list`, `config list`, `config get`, `store status`, `store
init`, `store push`, `store pull`, plus every `hams <provider>`
shortcut (already covered by `provider_cmd.go`). New command
verbs MUST add the validator before reading either flag.

#### Scenario: hams refresh --tag X --profile Y rejects the conflict

WHEN a user runs `hams refresh --tag macOS --profile linux`
THEN refresh SHALL return the same UsageError text as
`hams apply --tag macOS --profile linux` and SHALL NOT load
config or execute the refresh.

#### Scenario: Adding a new command verb gates the conflict

WHEN a developer adds a new command verb that reads `flags.Tag`
or `flags.Profile`
THEN the implementation SHALL call
`enforceTagProfileConsistency(flags)` before any config load,
and a regression test SHALL cover the conflict path.
