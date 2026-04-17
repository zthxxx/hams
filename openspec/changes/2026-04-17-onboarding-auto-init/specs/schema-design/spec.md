# schema-design — Spec Delta (onboarding auto-init)

## ADDED Requirements

### Requirement: Config.Tag is the canonical profile-tag YAML key

The hams config file (`hams.config.yaml`) SHALL accept the profile tag
under either of two keys:

- `tag:` — canonical, written by all auto-init paths and `hams config
  set tag <X>` invocations.
- `profile_tag:` — legacy alias retained for back-compat with v1.0
  config files.

Reading SHALL recognize both. When both keys are present in the same
document, `tag:` SHALL win (last-wins semantics aligned with the CLI
`--tag` flag overriding the configured value). Writing SHALL prefer
the key already present in the existing file; on a fresh write the
canonical `tag:` form SHALL be used.

`hams config set` accepts both `tag` and `profile_tag` as the key
argument. Validation rules (path-segment safety, etc.) apply uniformly
to both forms.

#### Scenario: Reading honors both keys

- **WHEN** `hams.config.yaml` contains `profile_tag: macOS`
- **THEN** `cfg.ProfileTag` SHALL load as `"macOS"`

#### Scenario: tag wins over profile_tag on collision

- **WHEN** `hams.config.yaml` contains both `tag: linux` and `profile_tag: macOS`
- **THEN** `cfg.ProfileTag` SHALL load as `"linux"`

#### Scenario: Write preserves legacy key in legacy file

- **WHEN** `hams.config.yaml` already contains `profile_tag: macOS`
  **AND** the user runs `hams config set tag linux`
- **THEN** the file SHALL update the existing `profile_tag:` key
  (NOT add a duplicate `tag:` key)

#### Scenario: Fresh-write uses canonical form

- **WHEN** `hams.config.yaml` does not exist yet **AND** the user
  runs `hams config set tag linux`
- **THEN** the new file SHALL contain `tag: linux` (NOT
  `profile_tag: linux`)

### Requirement: Default store location is ${HAMS_DATA_HOME}/store/

When `hams` auto-initializes a store on a fresh machine (because no
`--store`, no `--from-repo`, and no configured `store_path` are
present), the store SHALL be created at:

- `${HAMS_DATA_HOME}/store/` — defaults to
  `~/.local/share/hams/store/` per XDG conventions.

The auto-init path SHALL persist the resulting store path into
`${HAMS_CONFIG_HOME}/hams.config.yaml`'s `store_path:` key so that
subsequent runs discover the store without re-init.

#### Scenario: Auto-init lands in $HAMS_DATA_HOME/store/

- **WHEN** a user with empty `${HAMS_DATA_HOME}` runs any hams
  command that triggers auto-init
- **THEN** the store SHALL be created at `${HAMS_DATA_HOME}/store/`
- **AND** the global config's `store_path:` SHALL be updated to that
  absolute path.

### Requirement: Auto-init writes embedded template + git init

The auto-init store SHALL be:

1. Created as a directory (with parents) if it does not exist.
2. Initialized as a git repository via `git init` (real binary first,
   in-process go-git fallback when `git` is missing from PATH).
3. Populated from a `//go:embed`-bundled template containing at least:
   - `.gitignore` — covering `.state/`, `*.local.yaml`, `*.local.*`
   - `hams.config.yaml` — placeholder project-level config
   - `default/` — empty default profile directory
4. Idempotent across re-runs: a directory that already passes
   `Bootstrapped(dir)` SHALL NOT be re-initialized, and existing
   files SHALL NOT be overwritten (so user edits persist).

#### Scenario: Bootstrap creates the four required artifacts

- **WHEN** `storeinit.Bootstrap(dir)` runs on an empty dir
- **THEN** the dir SHALL contain `.git`, `.gitignore`,
  `hams.config.yaml`, and `default/` after the call.

#### Scenario: Bootstrap preserves user edits on re-run

- **WHEN** the user hand-edits the auto-init `hams.config.yaml`
  **AND** a subsequent `hams` invocation triggers auto-init again
- **THEN** the hand-edited file SHALL remain untouched.
