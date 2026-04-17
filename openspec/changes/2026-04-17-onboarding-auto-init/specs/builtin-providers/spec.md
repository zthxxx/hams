# builtin-providers — Spec Delta (onboarding auto-init)

## ADDED Requirements

### Requirement: Provider CLI invocations auto-init the default store

When the user runs `hams <provider> <verb> ...` against a clean
machine (empty `${HAMS_CONFIG_HOME}` AND empty `${HAMS_DATA_HOME}`),
the dispatcher SHALL auto-init the default store BEFORE handing
control to the provider's `HandleCommand`. Providers SHALL NOT need
to handle the empty-store case directly — `flags.Store` is
guaranteed to point at a valid store directory by the time
`HandleCommand` is called.

The auto-init lives at `internal/cli/autoinit.go::autoInitForProvider`
and is invoked from `routeToProvider` immediately after argument
parsing. It is a no-op when:

- `flags.Store` is already non-empty (the user passed `--store=<path>`).
- `cfg.StorePath` (loaded from the global config) is already set.
- `HAMS_NO_AUTO_INIT=1` (or `=true`, `=yes`) is in the environment.

#### Scenario: Brew install on a clean machine records into the auto-init store

- **WHEN** the user runs `hams brew install jq` on a machine with
  empty `${HAMS_CONFIG_HOME}` and `${HAMS_DATA_HOME}`
- **THEN** the dispatcher SHALL create the global config + auto-init
  the store
- **AND** the brew provider's `HandleCommand` SHALL receive a
  populated `flags.Store` pointing at the auto-init store
- **AND** the install SHALL be recorded at
  `${HAMS_DATA_HOME}/store/default/Homebrew.hams.yaml`.

#### Scenario: Provider with --store override skips auto-init

- **WHEN** the user runs `hams brew install jq --store=/elsewhere`
  on the same clean machine
- **THEN** the dispatcher SHALL NOT auto-create the
  `${HAMS_DATA_HOME}/store/` directory
- **AND** the brew provider SHALL record into `/elsewhere/...`.

#### Scenario: HAMS_NO_AUTO_INIT preserves legacy error

- **WHEN** the user sets `HAMS_NO_AUTO_INIT=1` and runs
  `hams brew install jq` on a clean machine
- **THEN** dispatch SHALL skip auto-init
- **AND** the provider SHALL surface the legacy "no store directory
  configured" UFE so existing scripts / CI continue to fail loudly
  rather than silently auto-initing.
