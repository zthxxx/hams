# Spec delta — builtin-providers (2026-04-16-package-provider-auto-record-gap)

## ADDED Requirements

### Requirement: Package-class CLI install/remove auto-records the hamsfile

The CLI-first `install` and `remove` subcommands for every Package-class provider (homebrew, apt, pnpm, npm, uv, goinstall, cargo, code-ext, mas) SHALL, upon the underlying package-manager exec returning exit code 0, append/remove the resource from the provider's hamsfile (`<profile>/<FilePrefix>.hams.yaml`) before returning. Failure of the exec SHALL leave the hamsfile untouched; dry-run SHALL leave it untouched; the CLI path SHALL NOT write the state file (state writes happen via `hams apply` / `hams refresh`).

This locks in the "auto-record" promise in `CLAUDE.md` and the CP-1 contract that currently only `apt` fully satisfies.

#### Scenario: Install an uninstalled resource adds it to the hamsfile

- **GIVEN** the hamsfile `<profile>/<FilePrefix>.hams.yaml` does NOT contain entry `<pkg>`
- **AND** the underlying package-manager install succeeds (exit 0)
- **WHEN** the user runs `hams <provider> install <pkg>`
- **THEN** the provider SHALL append `{app: <pkg>, tags: [cli]}` to `apps:` in the hamsfile
- **AND** the hamsfile SHALL be re-written atomically to disk

#### Scenario: Re-install is idempotent on the hamsfile

- **GIVEN** the hamsfile already contains entry `<pkg>`
- **WHEN** the user runs `hams <provider> install <pkg>` a second time
- **THEN** the hamsfile SHALL NOT grow a duplicate entry
- **AND** any existing user-added fields (tags, intro) on the entry SHALL be preserved

#### Scenario: Install exec failure leaves hamsfile untouched

- **GIVEN** the hamsfile does NOT contain entry `<pkg>`
- **WHEN** the user runs `hams <provider> install <pkg>`
- **AND** the underlying package-manager exec returns a non-zero exit code
- **THEN** the provider SHALL NOT modify the hamsfile
- **AND** the error SHALL propagate to the caller

#### Scenario: Remove deletes the hamsfile entry

- **GIVEN** the hamsfile contains entry `<pkg>`
- **AND** the underlying package-manager uninstall succeeds
- **WHEN** the user runs `hams <provider> remove <pkg>`
- **THEN** the provider SHALL delete the entry from `apps:` and re-write the hamsfile

#### Scenario: Remove exec failure leaves hamsfile untouched

- **GIVEN** the hamsfile contains entry `<pkg>`
- **WHEN** the user runs `hams <provider> remove <pkg>`
- **AND** the underlying package-manager exec returns a non-zero exit code
- **THEN** the provider SHALL NOT modify the hamsfile

#### Scenario: Dry-run skips the auto-record side effect

- **GIVEN** the user passes `--dry-run`
- **WHEN** the user runs `hams <provider> install <pkg>` (or `remove <pkg>`)
- **THEN** the provider SHALL NOT invoke the package manager
- **AND** the provider SHALL NOT modify the hamsfile
- **AND** the provider SHALL print a `[dry-run] Would …` message to stdout
