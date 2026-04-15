# builtin-providers — Spec Delta

## MODIFIED Requirements

### Requirement: Homebrew self-install bootstrap

The Homebrew provider's `Bootstrap` step SHALL detect a missing `brew`
binary on `$PATH` and, rather than silently executing the declared
`DependsOn[].Script`, SHALL gate the bootstrap execution on **explicit
user consent**. hams SHALL NEVER auto-execute a remote install script
without consent, because:

- Auto-executing `curl | bash` from `raw.githubusercontent.com`
  changes the tool's security posture without the user's knowledge.
- Corporate firewalls commonly block `raw.githubusercontent.com`; a
  silent network failure would be indistinguishable from "brew is
  broken" for the user.
- Homebrew's `install.sh` on macOS can trigger the Xcode CLI Tools GUI
  dialog, which blocks stdin — the installer appears hung while a
  modal dialog waits behind the user's IDE. The user MUST be warned
  about this explicitly before execution.

Consent SHALL be expressible in three ways:

1. **`--bootstrap` flag on `hams apply`** (non-interactive consent).
2. **Affirmative answer to an interactive TTY prompt** shown when
   `--bootstrap` is not set but stdin is a terminal.
3. **`--no-bootstrap` flag** explicitly opts OUT of the prompt (used
   in CI or by users who want fail-fast behavior).

When consent is NOT given (default, non-TTY context, or explicit
`--no-bootstrap`), hams SHALL emit a `UserFacingError` containing the
missing binary name, the exact script that would run, and the
copy-pasteable remedy (`hams apply --bootstrap ...`).

When consent IS given, hams SHALL resolve the
`DependsOn[].Provider` entry (typically `bash`), locate that provider
via the provider registry, delegate execution via the Bash provider's
`RunScript(ctx, script)` boundary (honoring the existing DI seam), and
stream the script's stdout/stderr to the user's terminal. Interactive
prompts from `install.sh` SHALL be forwarded unchanged to the TTY.

Bootstrap failure SHALL be terminal: hams SHALL NOT retry, SHALL
surface the script's exit code + last 50 lines of output, and SHALL
abort the apply run with a non-zero exit code.

#### Scenario: Bootstrap emits actionable error when `--bootstrap` is not set

- **WHEN** the Homebrew provider is needed (after the two-stage filter includes it), `brew` is NOT on `$PATH`, AND the user did NOT pass `--bootstrap`
- **AND** stdin is NOT a TTY (e.g., CI, pipe, script)
- **THEN** hams SHALL emit a `UserFacingError` whose body names the missing binary (`brew`), the exact script text from `Manifest().DependsOn[0].Script`, and the remedy `hams apply --bootstrap <original-args>`
- **AND** hams SHALL exit non-zero WITHOUT executing the script, without touching the network, and without modifying state files.

#### Scenario: Bootstrap runs with `--bootstrap` flag

- **WHEN** the Homebrew provider is needed, `brew` is NOT on `$PATH`, AND the user passed `--bootstrap`
- **THEN** hams SHALL resolve the `DependsOn[0]` entry (`Provider: "bash"`)
- **AND** hams SHALL locate the Bash provider in the registry, delegate the script via `BashScriptRunner.RunScript(ctx, script)`, and stream stdout/stderr to the user's terminal
- **AND** after the script exits 0, hams SHALL re-check `exec.LookPath("brew")`; on success, Homebrew operations SHALL proceed as if brew had been present from the start.

#### Scenario: Bootstrap prompts on TTY without `--bootstrap`

- **WHEN** the Homebrew provider is needed, `brew` is NOT on `$PATH`, the user did NOT pass `--bootstrap`, stdin IS a TTY, AND the user did NOT pass `--no-bootstrap`
- **THEN** hams SHALL display a prompt listing: the missing binary, the exact script to run, its documented side effects (sudo password prompt, Xcode Command Line Tools install, install location), and accept `[y/N/s]` input
- **AND** on `y`, the bootstrap proceeds as in "Bootstrap runs with `--bootstrap` flag"
- **AND** on `N` or EOF, hams SHALL emit the same UserFacingError as the no-TTY case and exit non-zero
- **AND** on `s` (skip-this-provider), hams SHALL skip the Homebrew provider for this run (as if `--except=brew` were set) and continue with other providers.

#### Scenario: Bootstrap failure is terminal

- **WHEN** the bootstrap script exits non-zero OR `brew` is still not on `$PATH` after the script completes
- **THEN** hams SHALL NOT retry
- **AND** hams SHALL surface the script's exit code and the last 50 lines of its stderr
- **AND** hams SHALL abort the apply run with a non-zero exit code
- **AND** state files SHALL NOT be modified (no partial progress recorded).

#### Scenario: `--no-bootstrap` disables the interactive prompt

- **WHEN** the Homebrew provider is needed, `brew` is NOT on `$PATH`, stdin IS a TTY, AND the user passed `--no-bootstrap`
- **THEN** hams SHALL skip the interactive prompt entirely
- **AND** hams SHALL emit the same UserFacingError as the no-TTY case and exit non-zero.

## ADDED Requirements

### Requirement: Provider framework executes DependOn.Script on explicit consent

The provider framework SHALL expose a `RunBootstrap(ctx, p, registry)`
function (in `internal/provider/bootstrap.go`) that:

1. Iterates `p.Manifest().DependsOn`, skipping entries whose `Platform`
   doesn't match the current OS.
2. For each entry with a non-empty `Script`, looks up the target
   `Provider` name in the registry.
3. Type-asserts the looked-up provider to a `BashScriptRunner`
   interface (new, tiny): `RunScript(ctx context.Context, script
   string) error`.
4. Delegates the script execution to that runner, which encapsulates
   the exec boundary (and is DI-injected in unit tests).

The Bash builtin provider (`internal/provider/builtin/bash`) SHALL
implement `BashScriptRunner` by shelling out to `/bin/bash -c <script>`
via its existing command runner. Unit tests SHALL exercise
`RunBootstrap` with an in-memory `BashScriptRunner` fake and assert
(a) the correct script was delegated, (b) platform-gated entries are
skipped, (c) registry-misses surface as typed errors, (d) script
failures propagate.

The CLI layer SHALL thread user consent through
`context.Context` via a `provider.WithBootstrapAllowed(ctx, bool)`
helper, so that `Bootstrap` implementations can query consent without
the CLI reaching into each provider's struct.

#### Scenario: RunBootstrap delegates a registered script

- **GIVEN** a provider manifest with `DependsOn: [{Provider: "bash", Script: "echo hi"}]`
- **AND** the Bash provider is registered
- **WHEN** `RunBootstrap(ctx, p, registry)` is called
- **THEN** the registered Bash provider's `RunScript(ctx, "echo hi")` SHALL be invoked exactly once
- **AND** its error (or nil) SHALL be returned.

#### Scenario: RunBootstrap skips platform-gated entries

- **GIVEN** a provider manifest with `DependsOn: [{Provider: "bash", Script: "...", Platform: "darwin"}]`
- **AND** the current OS is `linux`
- **WHEN** `RunBootstrap(ctx, p, registry)` is called
- **THEN** the Bash runner SHALL NOT be invoked
- **AND** the function SHALL return nil.

#### Scenario: RunBootstrap errors on missing host provider

- **GIVEN** a provider manifest with `DependsOn: [{Provider: "bash", Script: "..."}]`
- **AND** the Bash provider is NOT registered
- **WHEN** `RunBootstrap(ctx, p, registry)` is called
- **THEN** the function SHALL return an error whose message names the missing provider
- **AND** no script SHALL be executed.
