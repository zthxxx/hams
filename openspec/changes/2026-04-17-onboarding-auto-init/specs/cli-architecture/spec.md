# cli-architecture — Spec Delta (onboarding auto-init)

## ADDED Requirements

### Requirement: Apply accepts --tag flag

`hams apply` and `hams refresh` SHALL accept a `--tag <name>` flag that
selects the active profile tag for the current invocation. `--profile`
remains a recognized alias for back-compat with v1.0 scripts.

Precedence (highest first):

1. `--tag <name>` on the command line
2. The configured `tag:` (or legacy `profile_tag:`) value loaded from
   `${HAMS_CONFIG_HOME}/hams.config.yaml` (and store-level overrides)
3. Default literal value `"default"` when neither of the above is set

#### Scenario: --tag overrides the configured profile

- **WHEN** `~/.config/hams/hams.config.yaml` contains `tag: macOS`
  **AND** the user runs `hams apply --tag=linux`
- **THEN** apply SHALL use `linux` as the active profile tag
- **AND** the resolved profile dir SHALL be `<store>/linux/`

#### Scenario: --profile alias preserved

- **WHEN** the user runs `hams apply --profile=macOS`
- **THEN** apply SHALL behave identically to `hams apply --tag=macOS`

#### Scenario: default fallback when nothing configured

- **WHEN** no `--tag`, no `tag:` in the config, no `profile_tag:` in
  the config, and no `--profile` are present
- **THEN** apply SHALL use `"default"` as the active profile tag
- **AND** the resolved profile dir SHALL be `<store>/default/`

### Requirement: First-run auto-init for `hams apply` and `hams refresh`

When `hams apply` (or `hams refresh`) runs and resolves an empty store
path (no `--store`, no `--from-repo`, no configured `store_path`), the
CLI SHALL:

1. Write a default global config at `${HAMS_CONFIG_HOME}/hams.config.yaml`
   when the file is absent.
2. Bootstrap a hams store at `${HAMS_DATA_HOME}/store/` via
   `internal/storeinit.Bootstrap` (idempotent).
3. Persist the auto-init store path back into the global config's
   `store_path:` key for next-run discoverability.
4. Continue execution against the auto-init store. Apply / refresh on a
   freshly-init-ed store with zero hamsfiles SHALL exit successfully
   with the existing "No providers match" message.

#### Scenario: hams apply on a clean machine succeeds with zero providers

- **WHEN** the user runs `hams apply` on a machine with empty
  `${HAMS_CONFIG_HOME}` and `${HAMS_DATA_HOME}`
- **THEN** apply SHALL auto-create `~/.config/hams/hams.config.yaml`
- **AND** SHALL auto-init a store at `~/.local/share/hams/store/`
- **AND** SHALL exit 0 with "No providers match" rather than the legacy
  "no store directory configured" error.

### Requirement: First-run auto-init for `hams <provider>` invocations

When `hams <provider> ...` runs and the provider would otherwise
encounter an empty `cfg.StorePath` while loading its hamsfile, the CLI
dispatcher SHALL invoke the same auto-init path used by apply BEFORE
calling the provider's `HandleCommand`. The auto-init populates
`flags.Store` so the provider's `effectiveConfig` observes a real
store path.

#### Scenario: hams brew install jq on a clean machine creates the store

- **WHEN** the user runs `hams brew install jq` on a machine with
  empty `${HAMS_CONFIG_HOME}` and `${HAMS_DATA_HOME}`
- **THEN** the dispatcher SHALL auto-init the global config and the
  default store BEFORE invoking the brew provider's `HandleCommand`
- **AND** the provider SHALL record the install into
  `${HAMS_DATA_HOME}/store/default/Homebrew.hams.yaml` exactly as
  it would on a pre-configured machine.

### Requirement: Auto-init opt-out via HAMS_NO_AUTO_INIT

A power-user opt-out env var `HAMS_NO_AUTO_INIT` (recognized values:
`1`, `true`, `yes`, case-insensitive) SHALL disable the auto-init
path so callers who want the legacy "no store directory configured"
hard-fail keep getting it.

#### Scenario: Opt-out preserves legacy hard-fail

- **WHEN** the user sets `HAMS_NO_AUTO_INIT=1` and runs `hams apply`
  on a machine with empty `${HAMS_CONFIG_HOME}`
- **THEN** apply SHALL fail with a clear UFE explaining the opt-out
  and pointing at the remediation (`unset HAMS_NO_AUTO_INIT` or set
  `store_path` manually).
