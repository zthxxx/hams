# Schema Design — Delta for fix-apt-provider-and-store-config-scope

## MODIFIED Requirements

### Requirement: State File Schema

Each provider's observed state SHALL be stored in `<store>/.state/<machine-id>/<Provider>.state.yaml`. State files are machine-generated and SHALL NOT be hand-edited.

The state file SHALL have the following top-level structure:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | integer | YES | State schema version. Currently `2`. |
| `provider` | string | YES | Provider name. |
| `machine_id` | string | YES | Machine identifier from config. |
| `last_apply_session` | string | NO | Session identifier of the last apply run. |
| `last_apply_at` | string | NO | ISO timestamp (`YYYYMMDDTHHmmss`) of the last apply. |
| `last_apply_config_hash` | string | NO | SHA-256 content hash of the merged Hamsfile at last successful apply. Used as baseline for delete-set diffing. |
| `resources` | mapping of resource ID to resource state | YES | Per-resource observed state, keyed by resource identity (`app` for package-class, `urn` for script-class). |

Each resource state entry SHALL contain the following fields:

**Common fields (all resource classes):**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `state` | enum | YES | One of: `ok`, `failed`, `pending`, `removed`, `hook-failed`. |
| `first_install_at` | string | NO | ISO timestamp when this resource was first installed by hams on this machine. **Immutable** once set — subsequent re-installs, upgrades, removals, and re-installs-after-remove SHALL NOT modify this field. Written as `yaml:"first_install_at,omitempty"`. |
| `updated_at` | string | NO | ISO timestamp of the last state transition for this resource. SHALL be bumped on every `SetResource` call that actually transitions state (install, re-install, upgrade, remove, re-install-after-remove, failure). |
| `removed_at` | string | NO | ISO timestamp when the resource most recently transitioned to `state: removed`. SHALL be set when and only when `state == removed`. SHALL be cleared (absent from YAML via `omitempty`) whenever the resource transitions back to any non-removed state (e.g., `ok`, `failed`). Written as `yaml:"removed_at,omitempty"`. |
| `checked_at` | string | NO | ISO timestamp of last probe/refresh. |
| `last_error` | string | NO | Last error message if `state == failed`. Cleared on successful transition. |

**Package-class additional fields:**

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Installed version as reported by the provider. |

**KV-config-class additional fields (defaults, git-config, duti):**

| Field | Type | Description |
|-------|------|-------------|
| `value` | any | Current observed value. |

**Check-based-class additional fields (bash, system, ansible):**

| Field | Type | Description |
|-------|------|-------------|
| `check_cmd` | string | Check command re-run during probe. |
| `check_stdout` | string | Fingerprint (or truncated output) of the `check_cmd`'s stdout. |

**Filesystem-class additional fields (git-clone, file, download):**

| Field | Type | Description |
|-------|------|-------------|
| `remote` | string | Remote URL (git clone) or source URL (download). |
| `local_path` | string | Local filesystem path. |
| `default_branch` | string | Default branch name (git clone only). |

**Hook status fields (appended when hooks are present):**

| Field | Type | Description |
|-------|------|-------------|
| `hook_status` | mapping | Keyed by `pre_install` / `post_install`, each containing `state` (ok/failed) and `run_at` timestamp. |

**Field semantics by state transition (MUST hold for every provider):**

| Transition | `first_install_at` | `updated_at` | `removed_at` | `last_error` |
|------------|-------------------|--------------|--------------|--------------|
| New resource → `state: ok` | SET to `now` | SET to `now` | untouched (absent) | cleared |
| Existing `state: ok` → `state: ok` (re-install / upgrade) | UNCHANGED | SET to `now` | untouched | cleared |
| Any state → `state: removed` | UNCHANGED | SET to `now` | SET to `now` | untouched |
| `state: removed` → `state: ok` | UNCHANGED | SET to `now` | CLEARED (removed from YAML) | cleared |
| Any state → `state: failed` | UNCHANGED | SET to `now` | untouched | SET |

```yaml
# <store>/.state/MacbookProM5X/Homebrew.state.yaml
schema_version: 2
provider: Homebrew
machine_id: MacbookProM5X
last_apply_session: "sess_20260412T143022"
last_apply_config_hash: "sha256:a1b2c3d4e5f6..."

resources:
  git:
    state: ok
    version: "2.44.0"
    first_install_at: "20260410T091500"
    updated_at: "20260410T091500"
    checked_at: "20260412T143022"

  htop:
    state: removed
    version: "3.3.0"
    first_install_at: "20260410T091700"
    updated_at: "20260412T120000"
    removed_at: "20260412T120000"
    checked_at: "20260412T143022"
```

#### Scenario: State records successful first installation

- **WHEN** hams successfully installs a new resource (no prior entry)
- **THEN** the state file SHALL contain a resource entry with `state: ok`, `first_install_at` set to the current timestamp, `updated_at` equal to `first_install_at`, and no `removed_at` key.

#### Scenario: Re-install preserves first_install_at

- **WHEN** a resource currently in `state: ok` with `first_install_at: T0` is re-installed or upgraded at time `T1`
- **THEN** the state entry SHALL have `first_install_at: T0` (unchanged), `updated_at: T1`, no `removed_at` key, and no `last_error` key.

#### Scenario: Remove transitions record removed_at

- **WHEN** the user runs `hams apt remove bat` (or any equivalent provider remove) for a resource with `first_install_at: T0`
- **THEN** the state entry SHALL have `state: removed`, `first_install_at: T0` (unchanged), `updated_at: T1`, and `removed_at: T1` where T1 is the current timestamp.
- **AND** the entry SHALL NOT be deleted from the state file — it remains for audit.

#### Scenario: Re-install after remove preserves first_install_at and clears removed_at

- **WHEN** a resource has `state: removed`, `first_install_at: T0`, `removed_at: T1`, and is subsequently re-installed at time `T2`
- **THEN** the state entry SHALL have `state: ok`, `first_install_at: T0` (unchanged), `updated_at: T2`, and no `removed_at` key (cleared via YAML omitempty).

#### Scenario: Failed install records error without changing first_install_at

- **WHEN** a resource currently in `state: ok` with `first_install_at: T0` fails a subsequent operation at time `T1`
- **THEN** the state entry SHALL have `state: failed`, `first_install_at: T0` (unchanged), `updated_at: T1`, and `last_error` set to the error message.

#### Scenario: Hook failure recorded independently

- **WHEN** a `post_install` hook fails but the parent resource installed successfully
- **THEN** the resource SHALL have `state: ok` and `hook_status.post_install.state: failed`. The next apply SHALL retry the hook without re-installing the package.

---

### Requirement: Project-Level Config Schema

Each hams store repository SHALL contain a project-level config at `<store>/hams.config.yaml` with an optional local override at `<store>/hams.config.local.yaml`.

The project-level config SHALL support **only** the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | integer | YES | Schema version. Currently `1`. |
| `provider_priority` | list of strings | NO | Provider execution priority override for this store. |
| `llm_cli` | string | NO | Path to LLM CLI binary (store-level override). |
| `notification` | mapping | NO | Notification channel configuration (store-level override). |

The project-level config SHALL NOT accept the machine-scoped fields `profile_tag` or `machine_id`. These fields are permitted only in the global config files at `${HAMS_CONFIG_HOME}/hams.config.yaml` and `${HAMS_CONFIG_HOME}/hams.config.local.yaml`.

If either `profile_tag` or `machine_id` is present at the top level of `<store>/hams.config.yaml` or `<store>/hams.config.local.yaml` (i.e., the YAML-parsed value is a non-empty string), hams SHALL:

1. Exit with a non-zero exit code.
2. Emit an error message to stderr that names the offending field, the full path to the offending file, and explicitly points the user to the correct global config location. The error message template SHALL be:

   ```
   config: <absolute-path>: field "<profile_tag|machine_id>" is machine-scoped and must not be set at store level.
   Move it to ~/.config/hams/hams.config.yaml (or hams.config.local.yaml for untracked per-machine overrides).
   ```

This validation SHALL occur at config-load time, before any merge of store-level values into the effective configuration. Both `hams.config.yaml` (git-tracked) and `hams.config.local.yaml` (gitignored) SHALL be subject to identical rejection rules.

The effective configuration SHALL be resolved by merging in the following precedence order (highest wins):

1. `<store>/hams.config.local.yaml`
2. `<store>/hams.config.yaml`
3. `${HAMS_CONFIG_HOME}/hams.config.local.yaml`
4. `${HAMS_CONFIG_HOME}/hams.config.yaml`

Scalar fields SHALL use last-writer-wins. The `provider_priority` list SHALL be fully replaced (not merged) by any higher-precedence source that defines it.

```yaml
# <store>/hams.config.yaml (valid)
schema_version: 1
provider_priority:
  - bash
  - Homebrew
  - pnpm
  - npm
```

```yaml
# <store>/hams.config.yaml (INVALID — hams will reject at load)
schema_version: 1
profile_tag: dev          # ERROR: machine-scoped, belongs in ~/.config/hams/hams.config.yaml
machine_id: sandbox        # ERROR: machine-scoped, belongs in ~/.config/hams/hams.config.yaml
```

#### Scenario: Project-level priority overrides global priority

- **WHEN** the global config defines `provider_priority: [bash, Homebrew, pnpm]`
- **AND** the project-level config defines `provider_priority: [bash, apt, npm]`
- **THEN** the effective `provider_priority` SHALL be `[bash, apt, npm]`.

#### Scenario: Project-level config is optional

- **WHEN** a store repository does not contain `hams.config.yaml`
- **THEN** hams SHALL fall back to global config values without error.

#### Scenario: Store-level profile_tag is rejected

- **WHEN** `<store>/hams.config.yaml` contains `profile_tag: dev` at the top level
- **THEN** hams SHALL exit with a non-zero exit code
- **AND** stderr SHALL contain the absolute path of the offending file, the field name `profile_tag`, and a pointer to `~/.config/hams/hams.config.yaml`
- **AND** no config merge SHALL occur.

#### Scenario: Store-level machine_id is rejected

- **WHEN** `<store>/hams.config.yaml` contains `machine_id: sandbox` at the top level
- **THEN** hams SHALL exit with a non-zero exit code
- **AND** stderr SHALL contain the absolute path of the offending file, the field name `machine_id`, and a pointer to `~/.config/hams/hams.config.yaml`.

#### Scenario: Store-local override with machine-scoped field is rejected

- **WHEN** `<store>/hams.config.local.yaml` contains `profile_tag: dev` or `machine_id: foo` at the top level
- **THEN** hams SHALL reject the file with the same error semantics as `hams.config.yaml` — store-local overrides are NOT exempt from the scope rule.

#### Scenario: Global config with profile_tag loads successfully

- **WHEN** `${HAMS_CONFIG_HOME}/hams.config.yaml` contains `profile_tag: macOS` and `machine_id: MacbookProM5X`
- **AND** `<store>/hams.config.yaml` contains only `schema_version: 1` and `provider_priority: [...]`
- **THEN** hams SHALL load the merged configuration without error, using the global values for `profile_tag` and `machine_id`.

---

### Requirement: Schema Version Forward Compatibility

All schema files (config, Hamsfile, state) SHALL include a `schema_version` integer field. The hams binary SHALL:

1. Reject files with a `schema_version` higher than the binary understands, with an error message recommending `hams self-upgrade`.
2. Accept and process files with a `schema_version` equal to or lower than the binary supports.
3. When writing files, always use the highest schema version the binary supports.
4. For **state files only**, auto-migrate legacy `schema_version` values (currently `1`) to the current version (`2`) on load. The loader SHALL:
   - Detect `schema_version: 1` (or absent, which is treated as `1`).
   - Rename any resource's legacy `install_at` field to `first_install_at` (if `first_install_at` is empty).
   - Set the in-memory `schema_version` to `2`.
   - Persist the migration to disk on the next `Save` call. No separate migration command is required.
5. The state-file auto-migration is one-way. After a state file is rewritten at `schema_version: 2`, it SHALL NOT be downgraded by the binary. Rolling back hams to an older version requires the user to re-initialize the state directory (`rm -rf .state/<machine-id>/` and re-run `hams apply`).

Config and Hamsfile schemas SHALL NOT use the auto-migration mechanism in this version — only state files. If future config/Hamsfile schema revisions require migration, they will be specified separately.

#### Scenario: Future schema version triggers upgrade suggestion

- **WHEN** hams (supporting state schema version 2) encounters a state file with `schema_version: 3`
- **THEN** hams SHALL exit with a non-zero code and display: `State file uses schema version 3, but this hams binary only supports up to version 2. Run 'hams self-upgrade' to update.`

#### Scenario: Current schema version is processed normally

- **WHEN** hams (supporting state schema version 2) loads a state file with `schema_version: 2`
- **THEN** hams SHALL process the file normally without warnings.

#### Scenario: Legacy state file is auto-migrated on load

- **WHEN** hams loads a state file with `schema_version: 1` containing a resource `bat: {state: ok, install_at: "20260410T091500", updated_at: "20260410T091500"}`
- **THEN** the in-memory state SHALL have `schema_version: 2`, the resource `bat` SHALL have `first_install_at: "20260410T091500"` and the legacy `install_at` field SHALL be empty in the struct.
- **AND** the next call to `Save` SHALL write the file with `schema_version: 2` and `first_install_at: "20260410T091500"` (no `install_at` key present in the YAML output).

#### Scenario: Legacy state file absent install_at is migrated without error

- **WHEN** hams loads a state file with `schema_version: 1` containing a resource with neither `install_at` nor `first_install_at` set (e.g., a resource in `state: pending`)
- **THEN** the migration SHALL succeed without error, the in-memory `schema_version` SHALL be `2`, and the resource's `first_install_at` SHALL remain empty.
