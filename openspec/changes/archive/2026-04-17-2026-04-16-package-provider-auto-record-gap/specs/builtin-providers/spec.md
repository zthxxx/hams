# Spec delta — builtin-providers (2026-04-16-package-provider-auto-record-gap)

## ADDED Requirements

### Requirement: Package-class CLI install/remove auto-records the hamsfile AND state file

The CLI-first `install` and `remove` subcommands for every Package-class provider (homebrew, apt, pnpm, npm, uv, goinstall, cargo, code-ext, mas) SHALL, upon the underlying package-manager exec returning exit code 0, update BOTH the provider's hamsfile (`<profile>/<FilePrefix>.hams.yaml`) AND the state file (`<store>/.state/<machine-id>/<FilePrefix>.state.yaml`) before returning. Failure of the exec SHALL leave both files untouched; dry-run SHALL leave both files untouched.

The state-write half was added in cycles 96 (homebrew) and 202–208 (mas, cargo, npm, pnpm, uv, goinstall, vscodeext) after `hams list --only=<provider>` was observed to return empty immediately after a successful CLI install — the user had to run `hams refresh` to materialize the state row. Writing state as part of the CLI handler matches the "auto-record" promise in `CLAUDE.md`: one command, fully recorded on both serialization surfaces.

`goinstall` is the only provider without a symmetric remove path because `go install` has no uninstall verb — binaries must be removed manually (`Remove` is a no-op warn).

This locks in the full CP-1 contract (`apt`'s reference behavior) for every Package-class provider.

#### Scenario: Install an uninstalled resource adds it to the hamsfile AND state file

- **GIVEN** the hamsfile `<profile>/<FilePrefix>.hams.yaml` does NOT contain entry `<pkg>`
- **AND** the state file `<store>/.state/<machine-id>/<FilePrefix>.state.yaml` does NOT contain entry `<pkg>`
- **AND** the underlying package-manager install succeeds (exit 0)
- **WHEN** the user runs `hams <provider> install <pkg>`
- **THEN** the provider SHALL append `{app: <pkg>, tags: [cli]}` to `apps:` in the hamsfile
- **AND** the provider SHALL write `resources[<pkg>].state = ok` to the state file
- **AND** both files SHALL be re-written atomically to disk before the command returns
- **AND** the next `hams list --only=<provider>` SHALL return the package without requiring an intervening `hams refresh`

#### Scenario: Re-install is idempotent on the hamsfile

- **GIVEN** the hamsfile already contains entry `<pkg>`
- **WHEN** the user runs `hams <provider> install <pkg>` a second time
- **THEN** the hamsfile SHALL NOT grow a duplicate entry
- **AND** any existing user-added fields (tags, intro) on the entry SHALL be preserved

#### Scenario: Install exec failure leaves hamsfile AND state file untouched

- **GIVEN** the hamsfile does NOT contain entry `<pkg>`
- **AND** the state file does NOT contain entry `<pkg>`
- **WHEN** the user runs `hams <provider> install <pkg>`
- **AND** the underlying package-manager exec returns a non-zero exit code
- **THEN** the provider SHALL NOT modify the hamsfile
- **AND** the provider SHALL NOT write a state entry for `<pkg>`
- **AND** the error SHALL propagate to the caller

#### Scenario: Remove deletes the hamsfile entry AND tombstones the state entry

- **GIVEN** the hamsfile contains entry `<pkg>`
- **AND** the underlying package-manager uninstall succeeds
- **WHEN** the user runs `hams <provider> remove <pkg>` (except `goinstall`, which has no uninstall verb)
- **THEN** the provider SHALL delete the entry from `apps:` and re-write the hamsfile
- **AND** the provider SHALL write `resources[<pkg>].state = removed` (tombstone) to the state file so subsequent Probe and Plan passes skip the resource
- **AND** the next `hams apply` SHALL NOT attempt to re-install `<pkg>` purely from stale state

#### Scenario: Remove exec failure leaves hamsfile AND state file untouched

- **GIVEN** the hamsfile contains entry `<pkg>` with state `ok`
- **WHEN** the user runs `hams <provider> remove <pkg>`
- **AND** the underlying package-manager exec returns a non-zero exit code
- **THEN** the provider SHALL NOT modify the hamsfile
- **AND** the provider SHALL NOT mutate the existing state entry for `<pkg>` (no tombstone on failure)

#### Scenario: Dry-run skips all side effects including state file creation

- **GIVEN** the user passes `--dry-run`
- **WHEN** the user runs `hams <provider> install <pkg>` (or `remove <pkg>`)
- **THEN** the provider SHALL NOT invoke the package manager
- **AND** the provider SHALL NOT modify the hamsfile
- **AND** the provider SHALL NOT create OR modify the state file
- **AND** the provider SHALL print a `[dry-run] Would …` message to stdout
