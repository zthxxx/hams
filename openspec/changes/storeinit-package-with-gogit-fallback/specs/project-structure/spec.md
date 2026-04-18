# Spec delta — project-structure (storeinit-package-with-gogit-fallback)

## MODIFIED Requirements

### Requirement: Bundled go-git fallback for fresh machines

The hams binary SHALL bundle [go-git](https://github.com/go-git/go-git) as a
compiled-in dependency for Git operations. The go-git dependency SHALL be used
as a fallback when system `git` is not available. The fallback SHALL apply to
every hams code path that initialises or clones a git repository, including
but not limited to:

1. `hams apply --from-repo=<repo>` — clone path (already covered).
2. `hams <provider> …` first-run auto-scaffold — the `git init` that seeds a
   fresh store directory.

An `INFO`-level log line SHALL fire whenever the fallback is used, naming the
operation (`git init` / `git clone`) and the target directory, so operators
can distinguish "we used bundled go-git" from "we shelled out to /usr/bin/git".

#### Scenario: Fresh machine without git (clone)

- **WHEN** `hams apply --from-repo=zthxxx/hams-store` is executed on a machine without `git` in PATH
- **THEN** hams SHALL clone the repository using the bundled go-git library
- **AND** the clone SHALL succeed for HTTPS URLs
- **AND** an informational log message SHALL indicate that bundled go-git is being used

#### Scenario: System git is preferred when available (clone)

- **WHEN** `git` is found in PATH
- **THEN** hams SHALL prefer the system `git` command for clone/pull operations
- **AND** go-git SHALL only be used as a fallback when system git is unavailable

#### Scenario: Fresh machine without git (auto-scaffold)

- **GIVEN** a machine with no `git` binary on `PATH`
- **AND** no existing hams store directory
- **WHEN** the user runs any provider-wrapped command, e.g. `hams apt install htop`
- **THEN** hams SHALL auto-scaffold the store at `${HAMS_DATA_HOME}/store/` via the bundled go-git library's `PlainInit`
- **AND** the scaffold SHALL produce a functional `.git/` directory (contains at minimum `HEAD` and `config`)
- **AND** an `INFO`-level log line containing the phrase "bundled go-git" SHALL be emitted
- **AND** the scaffolded templates (`.gitignore`, `hams.config.yaml`) SHALL be written from the embedded FS

#### Scenario: System git is preferred when available (auto-scaffold)

- **GIVEN** a machine with `git` on `PATH`
- **AND** no existing hams store directory
- **WHEN** the user runs `hams apt install htop`
- **THEN** hams SHALL shell out to the system `git init --quiet <store-path>` to initialise the repo
- **AND** go-git SHALL NOT be used for this invocation
- **AND** no "bundled go-git" log line SHALL be emitted

#### Scenario: System git failure is not masked by the fallback

- **GIVEN** a machine with `git` on `PATH`
- **AND** the configured global git hook (`core.hooksPath`) terminates with a non-zero exit
- **WHEN** the user runs `hams apt install htop` on a fresh store path
- **THEN** hams SHALL propagate the `git init` error unchanged
- **AND** hams SHALL NOT silently retry via the go-git fallback (failure mode is "git misconfigured", not "git missing")
