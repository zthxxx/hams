# cli-architecture — Spec Delta

## MODIFIED Requirements

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

**State-only providers (no hamsfile, only a state file present):**

A provider SHALL be considered "state-only" when neither
`<FilePrefix>.hams.yaml` nor `<FilePrefix>.hams.local.yaml` exist for
the active profile but a `.state/<machine-id>/<FilePrefix>.state.yaml`
DOES exist. This typically happens when the user previously installed
resources via `hams <provider> install <pkg>` and later deleted the
hamsfile to "stop tracking" them.

`hams apply` SHALL skip state-only providers by default. The state file
is preserved unchanged; no bootstrap, probe, plan, or execute step
runs. This is the principle-of-least-surprise default — a missing
hamsfile is interpreted as "no declared intent", not "intent: zero
resources".

`hams apply --prune-orphans` SHALL opt into destructive reconciliation:
state-only providers are processed using an empty desired-state, so
Plan computes "remove every resource currently in state". The flag
SHALL be off by default to prevent partial git checkouts or accidental
file deletions from triggering mass uninstalls.

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

#### Scenario: Apply skips state-only providers by default

- **WHEN** the user previously ran `hams apt install htop`, then deleted `apt.hams.yaml`, then runs `hams apply` (no flags)
- **THEN** the apt provider SHALL be skipped (a debug log SHALL be emitted naming the provider as state-only)
- **AND** `apt.state.yaml` SHALL remain unchanged with `htop.state == ok`
- **AND** `htop` SHALL remain installed on the host.

#### Scenario: Apply with --prune-orphans removes orphaned state resources

- **WHEN** the user previously ran `hams apt install htop`, then deleted `apt.hams.yaml`, then runs `hams apply --prune-orphans`
- **THEN** the apt provider SHALL be processed with an empty desired-state
- **AND** Plan SHALL compute one Remove action for `htop`
- **AND** Execute SHALL run `runner.Remove(ctx, ["htop"])` and update `apt.state.yaml` so `htop.state == removed` with `removed_at` set
- **AND** `htop` SHALL no longer be installed on the host.

#### Scenario: --prune-orphans does not affect providers with hamsfiles

- **WHEN** the user runs `hams apply --prune-orphans` and the apt provider has both `apt.hams.yaml` (declaring `htop`) and `apt.state.yaml` (with `htop.state == ok`)
- **THEN** the flag SHALL be a no-op for apt — Plan compares the declared desired-state against observed; `htop` stays installed because it is still declared.
