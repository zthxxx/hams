# CLI Architecture — Spec Delta

## MODIFIED Requirements

### Requirement: Refresh command

The `hams refresh` command SHALL probe the current environment to update
state files with observed reality. Refresh SHALL only probe resources
already known in state; it SHALL NOT discover resources installed outside
of hams. For each resource in state, the provider's probe method SHALL be
called, and the state entry SHALL be updated with current values
(version, existence, config value). After refresh, the state SHALL reflect
the actual environment, enabling accurate diffing on the next `hams
apply`. Refresh SHALL apply provider filtering in two stages:

1. **Artifact-presence filter (stage 1)**: for each registered provider,
   include it only if EITHER of the following exists under the resolved
   store/profile/machine paths:
   - `<profile>/<FilePrefix>.hams.yaml` (or `<FilePrefix>.hams.local.yaml`)
   - `.state/<machine-id>/<FilePrefix>.state.yaml`
2. **User-supplied filter (stage 2)**: `--only` / `--except` flags narrow
   within the stage-1 result. `--only` does NOT bypass stage 1; it
   further restricts. If stage 1 yields an empty set, the command
   succeeds with a "no providers match" message and exit code 0.

Providers skipped at stage 1 SHALL NOT have `Bootstrap`, `Probe`, `Plan`,
or `Execute` called — they are treated as entirely uninstalled on this
machine. Debug-level logs SHALL record which providers were skipped and
why (no artifacts).

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

- **WHEN** the user runs `hams refresh --only=brew,pnpm` and both the Homebrew and pnpm providers have at least one of `<profile>/Homebrew.hams.yaml` / `<profile>/pnpm.hams.yaml` / `.state/<machine>/Homebrew.state.yaml` / `.state/<machine>/pnpm.state.yaml`
- **THEN** only the Homebrew and pnpm providers SHALL be probed; other providers' state SHALL remain unchanged.

#### Scenario: Refresh skips providers with no artifacts (stage 1)

- **WHEN** the machine has `apt.hams.yaml` + `apt.state.yaml` for the active profile/machine, but no Homebrew hamsfile and no Homebrew state file
- **AND** the user runs `hams refresh` (no `--only`)
- **THEN** the Homebrew provider's `Bootstrap` / `Probe` SHALL NOT be invoked — skipped at stage 1 with a debug-level log entry
- **AND** only the apt provider SHALL be probed, regardless of whether `brew` is installed on the host.

#### Scenario: Refresh with `--only` naming a provider with no artifacts

- **WHEN** the user runs `hams refresh --only=homebrew` on a machine where no Homebrew hamsfile and no Homebrew state file exist
- **THEN** the command SHALL exit 0 with a "no providers match" message, logging at debug level that Homebrew was filtered out at stage 1
- **AND** SHALL NOT invoke `homebrew.Bootstrap()` or attempt to install linuxbrew.

### Requirement: Apply with provider filtering

The `hams apply` command SHALL support `--only=<providers>` and
`--except=<providers>` flags to filter which providers are included in
the apply operation. Provider names SHALL be case-insensitive. Specifying
both `--only` and `--except` simultaneously SHALL be a usage error
(exit code 2). An unrecognized provider name SHALL produce an error.

Apply SHALL apply the same two-stage filter as refresh:

1. **Artifact-presence filter (stage 1)**: a provider is included only if
   at least one of `<profile>/<FilePrefix>.hams.yaml`,
   `<profile>/<FilePrefix>.hams.local.yaml`, or
   `.state/<machine-id>/<FilePrefix>.state.yaml` exists.
2. **User-supplied filter (stage 2)**: `--only` / `--except` narrow within
   the stage-1 result. `--only` does NOT bypass stage 1.

This ensures `hams apply` on a machine that only uses a subset of
providers does not attempt to `Bootstrap` providers whose upstream tool
(e.g., brew, cargo) may not even be installed on the host.

#### Scenario: Apply only specific providers

- **WHEN** the user runs `hams apply --only=brew,pnpm` and both providers have artifacts
- **THEN** only the Homebrew and pnpm providers SHALL execute; all other providers SHALL be skipped.

#### Scenario: Apply except specific providers

- **WHEN** the user runs `hams apply --except=apt` on a machine with apt, bash, and git-config artifacts
- **THEN** only the bash and git-config providers SHALL execute; apt SHALL be skipped.

#### Scenario: Apply skips providers with no artifacts

- **WHEN** the machine has `apt.hams.yaml` for the active profile but no Homebrew, pnpm, or cargo artifacts (neither hamsfile nor state file)
- **AND** the user runs `hams apply` (no filter flags)
- **THEN** only the apt provider SHALL be bootstrapped and executed — Homebrew / pnpm / cargo SHALL NOT have `Bootstrap` called and SHALL NOT produce "brew not found" / "cargo not found" errors.

#### Scenario: Apply with `--only` outside the stage-1 result

- **WHEN** the user runs `hams apply --only=homebrew` on a machine where no Homebrew hamsfile and no Homebrew state file exist
- **THEN** the command SHALL exit 0 with a "no providers match" message and take no further action.
