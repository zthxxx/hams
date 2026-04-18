# Spec delta — cli-architecture (storeinit-package-with-gogit-fallback)

## ADDED Requirements

### Requirement: Store-scaffold boundary lives in `internal/storeinit`

The store-scaffold responsibility SHALL live in the `internal/storeinit/` package. The CLI package (`internal/cli/`) MUST delegate to `storeinit.Bootstrap(...)` when it discovers the store is missing; the CLI package MUST NOT own the embedded templates, the `git init` (or go-git fallback) call, or the `WriteConfigKey(store_path)` persistence step directly. "Store scaffold" here means: on a fresh machine, materialise a usable hams store at a known path before any command that needs one runs.

Rationale:

- Two orthogonal operations touch the same store responsibility:
  `hams apply --from-repo` (clone) and `hams <provider> …` first-run
  (scaffold). Both belong with other store-lifecycle code, not with the
  CLI dispatcher.
- Per `CLAUDE.md` Development Process Principles, code architecture MUST
  use dependency injection to isolate uncontrollable external boundaries
  (filesystem, exec, git). `storeinit` exposes DI seams
  (`ExecGitInit`, `GoGitInit`) so unit tests can exercise both the
  happy path and the go-git fallback without touching the host.
- The package boundary enables a future `storeinit.CloneFromRepo` that
  unifies the two operations without bloating `internal/cli/`.

#### Scenario: CLI package does not embed scaffold templates

- **WHEN** static analysis (`rg '//go:embed template/store' internal/cli/`)
  is run
- **THEN** the query SHALL return zero matches — the only embed of
  scaffold templates SHALL be in `internal/storeinit/`

#### Scenario: CLI handler delegates scaffold to storeinit

- **WHEN** the user runs `hams apt install htop` on a machine with no
  configured store
- **THEN** the call chain SHALL be:
  `internal/cli/provider_cmd.go:HandleProviderCmd` →
  `internal/cli/<apply|scaffold>.go:<helper>` →
  `internal/storeinit.Bootstrap(ctx, paths, flags)`
- **AND** the `storeinit.Bootstrap` function SHALL be the single point
  that creates the directory, initialises git (via `ExecGitInit` or the
  `GoGitInit` fallback), writes the templates, and persists
  `store_path` to the global config

#### Scenario: Unit tests for scaffold can run without touching the host

- **GIVEN** `t.TempDir()`-backed test fixtures
- **WHEN** `storeinit_test.go` rebinds the `ExecGitInit` and `GoGitInit`
  package-level seams to in-memory fakes
- **THEN** the tests SHALL run without spawning a child `git` process
- **AND** the tests SHALL run without requiring `git` to exist on the
  test host's `PATH`

### Requirement: First-run auto-scaffold preserves silent onboarding

The scaffold path SHALL seed `profile_tag` and `machine_id` into the global config when either field is empty at scaffold time, so that the very first provider invocation on a fresh machine does not emit the "using default profile" / "using 'unknown' machine" warnings. Source of values:

- `profile_tag`: fall back to `config.DefaultProfileTag` ("default").
- `machine_id`: fall back to `config.DeriveMachineID()` —
  `$HAMS_MACHINE_ID` → `os.Hostname()` → "default", with
  path-segment sanitisation.

This requirement mirrors the existing `cli-architecture/spec.md:654`
"non-interactive first-run" guarantee and extends it to the
provider-wrapped entry point (not just `hams apply --from-repo`).

#### Scenario: First `hams <provider>` on a fresh machine is fully silent

- **GIVEN** no `${HAMS_CONFIG_HOME}/hams.config.yaml` exists
- **AND** no store directory exists under `${HAMS_DATA_HOME}/`
- **WHEN** the user runs `hams apt install htop`
- **THEN** the auto-scaffold SHALL materialise the store
- **AND** the scaffold SHALL write `profile_tag: default` + `machine_id: <hostname>` into the global config
- **AND** stderr SHALL NOT contain the strings "using default profile" or "using 'unknown' machine"

#### Scenario: seedIfMissing does not clobber user-set values

- **GIVEN** the user has run `hams config set profile_tag macOS` before
  their first provider invocation
- **AND** no store directory exists yet
- **WHEN** the user runs `hams apt install htop`
- **THEN** the auto-scaffold SHALL NOT overwrite `profile_tag` — the
  stored value `macOS` SHALL persist
- **AND** `machine_id` SHALL still be seeded if empty
