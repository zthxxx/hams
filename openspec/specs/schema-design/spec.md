# Schema Design Spec

This spec defines every YAML file schema consumed or produced by hams: global config, project-level config, Hamsfiles, Hamsfile local overrides, state files, lock files, and the URN structure. It also specifies the YAML comment-preservation and atomic-write requirements for the hamsfile SDK.

All schemas target `go-yaml` v3 with `yaml.Node`-based round-trip fidelity.

---

## ADDED Requirements

### Requirement: Global Config Schema

The global configuration file at `${HAMS_CONFIG_HOME}/hams.config.yaml` (default `~/.config/hams/hams.config.yaml`) SHALL define machine-level settings that apply across all stores and profiles.

The schema SHALL include the following top-level fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | integer | YES | Schema version for forward compatibility. Currently `1`. |
| `profile_tag` | string | YES | Active profile directory name (e.g., `macOS`, `openwrt`). |
| `machine_id` | string | YES | Unique identifier for this machine (e.g., `MacbookProM5X`). Used as state directory name. |
| `store_repo` | string | YES | Path or GitHub shorthand (`owner/repo`) to the hams store repository. |
| `llm_cli` | string | NO | Path to LLM CLI binary (e.g., `/usr/local/bin/claude`). Omit if LLM enrichment is not used. |
| `provider_priority` | list of strings | NO | Ordered provider execution priority. Overrides the built-in default order. |
| `notification` | mapping | NO | Notification channel configuration. |

The `notification` mapping SHALL contain:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bark_token` | string | NO | Bark push notification token. MUST NOT appear in this file; reserved for `.local.yaml` or keychain. Presence here SHALL trigger a validation warning. |

```yaml
# ~/.config/hams/hams.config.yaml
schema_version: 1
profile_tag: macOS
machine_id: MacbookProM5X
store_repo: zthxxx/hams-store
llm_cli: /usr/local/bin/claude
provider_priority:
  - bash
  - Homebrew
  - apt
  - pnpm
  - npm
  - uv
  - goinstall
  - cargo
  - code-ext
  - mas
  - git
  - defaults
  - duti
```

#### Scenario: First-time setup writes global config

WHEN the user runs `hams apply --from-repo=zthxxx/hams-store` on a fresh machine
AND is prompted for profile tag `macOS` and machine ID `MacbookProM5X`
THEN hams SHALL create `~/.config/hams/hams.config.yaml` with `schema_version: 1`, the provided `profile_tag`, `machine_id`, and `store_repo` fields.

#### Scenario: Missing required fields produce validation error

WHEN hams loads a global config file that is missing `machine_id`
THEN hams SHALL exit with a non-zero exit code and an error message stating which required field is missing, along with a suggested fix command.

---

### Requirement: Global Config Local Override

A companion file `${HAMS_CONFIG_HOME}/hams.config.local.yaml` SHALL exist for secrets and machine-specific overrides that MUST NOT be committed to version control.

The local file SHALL support the same schema as the global config. Fields present in the local file SHALL override the corresponding fields in the base config. The `provider_priority` list, if present in the local file, SHALL fully replace (not merge with) the base list.

```yaml
# ~/.config/hams/hams.config.local.yaml
notification:
  bark_token: "aBcDeFgHiJkLmNoPqRsT"
llm_cli: /opt/homebrew/bin/codex
```

#### Scenario: Bark token in local config is accepted

WHEN hams loads a global config where `hams.config.local.yaml` contains `notification.bark_token`
THEN hams SHALL merge the token into the effective config and use it for Bark push notifications.

#### Scenario: Bark token in base config triggers warning

WHEN hams loads `hams.config.yaml` (non-local) and it contains `notification.bark_token`
THEN hams SHALL emit a warning to stderr recommending the user move the token to `hams.config.local.yaml` or OS keychain.

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

### Requirement: Hamsfile Schema

Each provider's desired state SHALL be declared in a file named `<Provider>.hams.yaml` within the active profile directory. The `<Provider>` prefix SHALL use the provider's display name with its canonical capitalization (e.g., `Homebrew.hams.yaml`, `pnpm.hams.yaml`, `bash.hams.yaml`).

The Hamsfile SHALL have the following top-level structure:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | integer | YES | Schema version. Currently `1`. |
| `provider` | string | YES | Provider name (must match filename prefix). |
| `groups` | list of group mappings | YES | Tag-based groupings of resources. |

Each group mapping SHALL contain:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tag` | string | YES | Group label (e.g., `dev-tools`, `media`, `system`). |
| `items` | list of item mappings | YES | Resources belonging to this group. |

A single resource MAY appear in multiple groups (multiple tags). The canonical identity of a resource is its `app` name (package-type) or `urn` (script-type). Duplicate identities across groups SHALL produce a validation error.

#### Package-Type Item Schema

For package-type providers (Homebrew, pnpm, npm, uv, goinstall, cargo, mas, apt, code-ext):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `app` | string | YES | Package identifier (e.g., `git`, `ripgrep`, `visual-studio-code`). |
| `intro` | string | NO | Human-readable description of what this package does. |
| `tags` | list of strings | NO | Additional tags beyond the enclosing group's tag. |
| `hooks` | mapping | NO | Lifecycle hooks (see Hooks section). |

```yaml
# <store>/macOS/Homebrew.hams.yaml
schema_version: 1
provider: Homebrew

groups:
  - tag: dev-tools
    items:
      - app: git
        intro: Distributed version control system.
      - app: ripgrep
        intro: Fast recursive grep alternative.
        tags:
          - search
      - app: visual-studio-code
        intro: Code editor by Microsoft.
        tags:
          - editor
        hooks:
          post_install:
            - run: hams code-ext apply
              defer: true

  - tag: media
    items:
      - app: ffmpeg
        intro: Multimedia framework for audio/video processing.
      - app: imagemagick
```

#### Script-Type Item Schema

For script-type providers (bash, defaults, duti, git-config, system, file, ansible):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `urn` | string | YES | Unique resource identifier in `urn:hams:<provider>:<resource-id>` format. |
| `step` | string | YES | Human-readable name of the operation. |
| `description` | string | NO | Longer explanation of what this step does. |
| `tags` | list of strings | NO | Additional tags beyond the enclosing group's tag. |
| `run` | string | YES (bash) | Shell command to execute. |
| `check` | string | Conditional | Idempotency check command. Required unless the command itself is idempotent. |
| `hooks` | mapping | NO | Lifecycle hooks. |

Provider-specific fields (e.g., `domain`, `key`, `value` for defaults; `scope`, `key`, `value` for git-config) SHALL be defined per-provider in the Builtin Providers spec. The Hamsfile schema accommodates them as extension fields within each item.

```yaml
# <store>/macOS/bash.hams.yaml
schema_version: 1
provider: bash

groups:
  - tag: system
    items:
      - urn: "urn:hams:bash:install-homebrew"
        step: Install Homebrew
        description: Install Homebrew package manager if not present.
        run: |
          /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        check: command -v brew

      - urn: "urn:hams:bash:set-zsh-default"
        step: Set zsh as default shell
        run: chsh -s /bin/zsh
        check: "[ \"$SHELL\" = \"/bin/zsh\" ]"

  - tag: network
    items:
      - urn: "urn:hams:bash:setup-proxy"
        step: Configure proxy settings
        run: ./scripts/setup-proxy.sh
        check: curl -s --connect-timeout 2 https://example.com > /dev/null
```

```yaml
# <store>/macOS/defaults.hams.yaml
schema_version: 1
provider: defaults

groups:
  - tag: finder
    items:
      - urn: "urn:hams:defaults:finder-show-extensions"
        step: Show all file extensions in Finder
        domain: NSGlobalDomain
        key: AppleShowAllExtensions
        value: true

      - urn: "urn:hams:defaults:finder-show-hidden"
        step: Show hidden files in Finder
        domain: com.apple.finder
        key: AppleShowAllFiles
        value: true
```

#### Scenario: Valid Hamsfile is parsed without error

WHEN hams loads a Hamsfile with `schema_version: 1`, a valid `provider` field, and at least one group with one item
THEN hams SHALL parse the file successfully and make all items available to the provider.

#### Scenario: Duplicate app identity across groups is rejected

WHEN a Hamsfile contains the same `app: git` in two different groups
THEN hams SHALL exit with a validation error identifying the duplicate entry and the groups it appears in.

#### Scenario: Script-type entry missing URN is rejected

WHEN a bash Hamsfile contains an item without a `urn` field
THEN hams SHALL exit with a validation error stating that script-type resources require a URN.

---

### Requirement: Hamsfile Hooks Schema — Deferred to v1.1

> **v1 status (as of 2026-04-16):** The `hooks:` schema below is documented and the `internal/provider/hooks.go` execution engine is fully built and tested, but the v1 hamsfile loader does NOT yet parse `hooks:` keys and no provider's `Plan()` method populates `Action.Hooks`. In v1, a `hooks:` block in a hamsfile is **silently ignored**. The scenarios in this section describe v1.1 behavior; v1 behavior is documented in `cli-architecture/spec.md` (hooks-defer delta, commit TBD).

Items in a Hamsfile MAY declare lifecycle hooks via a `hooks` mapping. Hooks SHALL only fire on the `NotPresent -> Install` transition.

The `hooks` mapping SHALL support:

| Field | Type | Description |
|-------|------|-------------|
| `pre_install` | list of hook entries | Commands to run before installing this resource. |
| `post_install` | list of hook entries | Commands to run after installing this resource. |

Each hook entry SHALL contain:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `run` | string | YES | Shell command or nested hams provider call (e.g., `hams brew install foo`). |
| `defer` | boolean | NO | If `true`, execution is deferred until after the current provider finishes all its installs. Default: `false`. |

```yaml
- app: visual-studio-code
  intro: Code editor by Microsoft.
  hooks:
    pre_install:
      - run: echo "Installing VS Code..."
    post_install:
      - run: hams code-ext apply
        defer: true
      - run: defaults write com.microsoft.VSCode ApplePressAndHoldEnabled -bool false
```

#### Scenario: Pre-hook failure blocks installation

WHEN a resource has a `pre_install` hook that exits with a non-zero code
THEN the parent resource SHALL NOT be installed, and both the hook and the resource SHALL be marked `failed` in state.

#### Scenario: Deferred post-hook executes after provider completes

WHEN a resource has a `post_install` hook with `defer: true`
AND the provider finishes all its regular installs
THEN the deferred hook SHALL execute after the provider's last regular install, but before the next provider begins.

---

### Requirement: Hamsfile Local Override Schema

Each Hamsfile MAY have a local override at `<Provider>.hams.local.yaml`. Local files SHALL NOT be committed to version control (enforced by `.gitignore` patterns `*.local.*`).

The local file SHALL use the same schema as the base Hamsfile. Merge semantics SHALL be provider-specific, registered by each provider with the hamsfile SDK. The general merge rules are:

1. **Package-type providers** (Homebrew, pnpm, npm, etc.): Items in the local file SHALL be appended to the merged item list. If a local item has the same `app` as a base item, the local item's sub-fields (e.g., `hooks`, `tags`) SHALL merge into the base entry (local fields override base fields at the leaf level).
2. **Script-type providers** (bash, defaults, duti, etc.): Items with the same `urn` in the local file SHALL fully override the matching base entry. Items with new URNs SHALL be appended.
3. **Group merging**: If the local file contains a group with a `tag` that exists in the base file, items are merged into that group. If the tag is new, the entire group is appended.

```yaml
# <store>/macOS/Homebrew.hams.local.yaml
schema_version: 1
provider: Homebrew

groups:
  - tag: work
    items:
      - app: slack
        intro: Team communication platform.
      - app: zoom
        intro: Video conferencing.

  - tag: dev-tools
    items:
      # Overrides hooks for visual-studio-code from base Hamsfile
      - app: visual-studio-code
        hooks:
          post_install:
            - run: ./scripts/work-vscode-setup.sh
```

#### Scenario: Local items append to base list

WHEN `Homebrew.hams.yaml` contains `[git, ripgrep]` under tag `dev-tools`
AND `Homebrew.hams.local.yaml` contains `[slack]` under a new tag `work`
THEN the effective Hamsfile SHALL contain both groups with all three items.

#### Scenario: Same-URN local entry overrides base entry

WHEN `bash.hams.yaml` contains an item with `urn: urn:hams:bash:setup-proxy`
AND `bash.hams.local.yaml` contains an item with the same URN but a different `run` command
THEN the effective item SHALL use the local file's `run` command.

#### Scenario: Same-app local entry merges sub-fields

WHEN `Homebrew.hams.yaml` contains `app: visual-studio-code` with `intro` and `hooks.post_install`
AND `Homebrew.hams.local.yaml` contains `app: visual-studio-code` with only `hooks.post_install`
THEN the effective entry SHALL retain the base `intro` and use the local `hooks.post_install`.

---

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

- **WHEN** the user runs `hams apt remove htop` (or any equivalent provider remove) for a resource with `first_install_at: T0`
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

### Requirement: URN Structure

Script-type resources SHALL be identified by a URN (Uniform Resource Name) with the following format:

```
urn:hams:<provider>:<resource-id>
```

| Segment | Constraints |
|---------|-------------|
| `urn` | Literal string `urn`. |
| `hams` | Literal string `hams`. Namespace identifier. |
| `<provider>` | Provider name in lowercase (e.g., `bash`, `defaults`, `duti`, `git-config`). |
| `<resource-id>` | Lowercase alphanumeric string with hyphens. MUST be unique within the provider. No colons allowed. |

The hamsfile SDK SHALL validate URN format on load and reject malformed URNs with a descriptive error.

Package-type resources SHALL NOT use URNs. Their identity is the natural package name in the `app` field.

```
urn:hams:bash:install-homebrew
urn:hams:defaults:finder-show-extensions
urn:hams:duti:set-vscode-markdown
urn:hams:git-config:user-email
urn:hams:system:set-hostname
urn:hams:file:zshrc-symlink
```

#### Scenario: Valid URN is accepted

WHEN a Hamsfile contains `urn: "urn:hams:bash:install-homebrew"`
THEN the hamsfile SDK SHALL parse it successfully and extract provider `bash` and resource ID `install-homebrew`.

#### Scenario: Malformed URN is rejected

WHEN a Hamsfile contains `urn: "hams:bash:install-homebrew"` (missing `urn:` prefix)
THEN the hamsfile SDK SHALL reject the entry with an error message describing the expected format.

#### Scenario: URN with colon in resource ID is rejected

WHEN a Hamsfile contains `urn: "urn:hams:bash:install:homebrew"` (colon in resource-id)
THEN the hamsfile SDK SHALL reject the entry because colons are not allowed in the resource-id segment.

---

### Requirement: YAML Comment Preservation

The hamsfile SDK SHALL preserve all YAML comments (line comments, block comments, and head/foot comments) during read-modify-write cycles. Users hand-edit Hamsfiles and rely on comments for documentation.

Implementation requirements:

1. The SDK SHALL use `go-yaml` v3 with `yaml.Node` tree manipulation for all Hamsfile operations.
2. The SDK SHALL NOT use `yaml.Marshal`/`yaml.Unmarshal` cycles that discard comments. Instead, it SHALL operate on the `yaml.Node` AST directly for modifications.
3. Round-trip fidelity: loading a Hamsfile and saving it without modifications SHALL produce byte-identical output (excluding trailing newline normalization).

```yaml
# This is my Homebrew setup
# Last reviewed: 2026-03-15

schema_version: 1
provider: Homebrew

groups:
  - tag: dev-tools  # Essential development tools
    items:
      - app: git        # Everyone needs git
        intro: Distributed version control system.
      # TODO: consider adding mercurial
      - app: ripgrep
        intro: Fast recursive grep alternative.
```

After adding a new entry programmatically, ALL existing comments SHALL be preserved in their original positions.

#### Scenario: Comments survive round-trip

WHEN hams reads a Hamsfile containing inline comments and block comments
AND writes it back without modifications
THEN the output file SHALL be byte-identical to the input file (modulo trailing newline).

#### Scenario: Comments survive entry addition

WHEN hams adds `app: fd` to the `dev-tools` group of a Hamsfile that contains inline and block comments
THEN all pre-existing comments SHALL remain in their original positions relative to their associated nodes.

---

### Requirement: Atomic File Writes

All Hamsfile and state file writes SHALL be atomic to prevent corruption from crashes or interruptions.

The write procedure SHALL be:

1. Write the complete content to a temporary file in the same directory as the target (e.g., `<target>.tmp.<pid>`).
2. Call `fsync` on the temporary file descriptor.
3. Rename the temporary file to the target path (atomic on POSIX filesystems).
4. If the rename fails, the temporary file SHALL be cleaned up.

#### Scenario: Crash during write does not corrupt file

WHEN hams is writing a state file and the process is killed after step 1 but before step 3
THEN the original state file SHALL remain intact and the orphaned temporary file SHALL be cleaned up on the next hams invocation.

#### Scenario: Concurrent read during write sees consistent state

WHEN a state file is being written atomically
AND another process reads the same file path during the write
THEN the reader SHALL see either the old complete file or the new complete file, never a partial write.

---

### Requirement: Lock File Format

The single-writer lock SHALL be enforced via a lock file at `<store>/.state/<machine-id>/.lock`.

The lock file SHALL contain the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `pid` | integer | Process ID of the lock holder. |
| `command` | string | The hams command being executed (e.g., `apply`, `refresh`). |
| `started_at` | string | ISO timestamp (`YYYYMMDDTHHmmss`) when the lock was acquired. |

```yaml
# <store>/.state/MacbookProM5X/.lock
pid: 42567
command: apply
started_at: "20260412T143022"
```

Lock acquisition procedure:

1. Attempt to create the lock file with `O_CREAT | O_EXCL` (fail if exists).
2. If the file exists, read the PID and check if the process is still running.
3. If the process is dead (stale lock), remove the lock file and retry.
4. If the process is alive, exit with an error message stating which command holds the lock, its PID, and when it started.

#### Scenario: Concurrent apply is blocked

WHEN `hams apply` is running with PID 42567
AND the user runs `hams apply` in another terminal
THEN the second invocation SHALL exit with a non-zero code and a message like: `Lock held by PID 42567 (command: apply, started at 20260412T143022). Another hams session is running.`

#### Scenario: Stale lock is cleaned up

WHEN a lock file exists with PID 99999
AND no process with PID 99999 is running
THEN hams SHALL remove the stale lock file, log a warning about the stale lock, and proceed with lock acquisition.

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

---

### Requirement: Hamsfile SDK Read/Write API

The `internal/hamsfile` package SHALL expose a typed Go API for providers to read and write Hamsfiles without direct YAML manipulation. All providers MUST use this SDK; direct file I/O on Hamsfiles is prohibited.

The SDK SHALL provide at minimum:

1. **Load**: Parse a Hamsfile and its `.local.yaml` companion, apply merge semantics, return a typed structure.
2. **AddItem**: Add a new item to a specified group (creating the group if needed). Preserve existing comments.
3. **RemoveItem**: Remove an item by `app` or `urn`. Remove the group if it becomes empty.
4. **UpdateItem**: Update fields of an existing item in-place. Preserve comments on unchanged fields.
5. **ListItems**: Return all items in the effective (merged) Hamsfile.
6. **Save**: Write the modified Hamsfile atomically with comment preservation.

The SDK SHALL hold a global write mutex. Concurrent reads are permitted; writes are serialized.

Each provider SHALL register its merge strategy with the SDK at registration time. The merge strategy SHALL define how `.local.yaml` entries combine with base entries for that provider.

#### Scenario: Provider adds a package via SDK

WHEN the Homebrew provider calls `AddItem("dev-tools", Item{App: "fd", Intro: "Fast find alternative."})` on a Hamsfile that already has items in the `dev-tools` group
THEN the SDK SHALL append the new item to the group's items list, preserve all existing comments, and write the file atomically.

#### Scenario: Provider removes a package via SDK

WHEN the Homebrew provider calls `RemoveItem("htop")` on a Hamsfile
THEN the SDK SHALL remove the `htop` entry from its group, remove the group if empty, and write the file atomically.

#### Scenario: Concurrent writes are serialized

WHEN two goroutines attempt to call `Save` simultaneously on the same Hamsfile
THEN the SDK SHALL serialize the writes via its global mutex, ensuring each write sees the result of the previous one.

---

### Requirement: Provider-Specific Extension Fields

The Hamsfile item schema SHALL be extensible with provider-specific fields. The hamsfile SDK SHALL pass through unknown fields without validation at the schema level; per-provider validation is the responsibility of the provider.

Known extension field patterns:

| Provider | Extension Fields |
|----------|-----------------|
| `defaults` | `domain`, `key`, `value`, `type` |
| `duti` | `uti`, `extension`, `role` |
| `git-config` | `scope` (`global`/`system`/`local`), `key`, `value` |
| `system` | `hostname`, `shell` |
| `file` | `source`, `destination`, `mode`, `template` (boolean) |
| `git-clone` | `remote`, `local_path`, `default_branch` |

The detailed field definitions for each provider are specified in the Builtin Providers spec. This schema spec only establishes that extension fields are permitted and pass through the SDK.

#### Scenario: Extension fields survive round-trip

WHEN a defaults Hamsfile contains `domain: NSGlobalDomain` and `key: AppleShowAllExtensions` on an item
AND the hamsfile SDK loads and saves the file without modifications
THEN the extension fields SHALL be preserved in the output.

#### Scenario: Unknown fields do not cause errors

WHEN a Hamsfile contains a field `custom_metadata: foo` on an item that the SDK does not recognize
THEN the SDK SHALL preserve the field without error, passing it through for provider-level validation.

---

### Requirement: Timestamp Format

All timestamps in hams YAML files (config, state, lock) SHALL use the ISO-based compact format `YYYYMMDDTHHmmss` in the local timezone of the machine.

Examples:
- `20260412T143022` (April 12, 2026 at 14:30:22 local time)
- `20260101T000000` (January 1, 2026 at midnight local time)

Timestamps SHALL be stored as quoted YAML strings to prevent YAML parsers from interpreting them as integers or dates.

#### Scenario: Timestamps are quoted in YAML output

WHEN hams writes a state file with `checked_at: "20260412T143022"`
THEN the value SHALL be quoted in the YAML output to prevent parser reinterpretation.

#### Scenario: Timestamp format is consistent across files

WHEN hams writes timestamps to state files, lock files, and log references
THEN all timestamps SHALL use the `YYYYMMDDTHHmmss` format without timezone suffix, colons, or hyphens.

---

### Requirement: Directory Layout for State

State files SHALL be organized under `<store>/.state/<machine-id>/`:

```
<store>/
  .state/                          # .gitignore'd
    MacbookProM5X/                 # machine-id directory
      .lock                        # single-writer lock file
      Homebrew.state.yaml
      pnpm.state.yaml
      bash.state.yaml
      defaults.state.yaml
    OpenwrtRouter/
      apt.state.yaml
      bash.state.yaml
```

The `.state/` directory SHALL be listed in the store's `.gitignore` by default. The `hams apply --from-repo=` bootstrap flow SHALL create the `.state/<machine-id>/` directory if it does not exist.

#### Scenario: State directory is created on first apply

WHEN `hams apply` runs for the first time on a machine with ID `MacbookProM5X`
AND the `.state/MacbookProM5X/` directory does not exist
THEN hams SHALL create the directory before writing any state files.

#### Scenario: State directory is gitignored

WHEN `hams apply --from-repo=` initializes a new store
THEN the `.gitignore` file in the store root SHALL contain `.state/` (or a pattern that excludes the state directory from git tracking).

### Requirement: Optional git backend for remote state storage

The state system SHALL support an optional git backend mode where state files are stored in an independent private git repository on a branch named `state/<machine-id>`. This mode is NOT implemented in v1 but the state interface MUST be designed to allow swapping the local filesystem backend for a git-backed backend without changing consumer code.

#### Scenario: State backend abstraction

WHEN the state package is initialized
THEN it SHALL use a `StateBackend` interface that abstracts read/write/list operations
AND the default implementation SHALL be `LocalFilesystemBackend`
AND the interface SHALL be sufficient to later implement a `GitBackend` that pushes state to a `state/<machine-id>` branch in a configurable remote repository.

---
<!-- Merged from change: fix-v1-planning-gaps -->

# Schema Design — Spec Delta (fix-v1-planning-gaps)

## ADDED

### `preview-cmd` Field for KV Config Resources

Hamsfile entries for KV config providers (defaults, duti, git-config) SHALL support an optional `preview-cmd` field:

- `preview-cmd` stores the original human-readable command string (e.g., `defaults write com.apple.dock autohide -bool true`).
- The field is for review/audit purposes only; execution SHALL use the structured `args` field.
- `preview-cmd` SHALL be preserved during YAML round-trips (comment preservation applies).
- When a KV config resource is recorded via CLI (e.g., `hams defaults write ...`), the provider SHALL auto-populate `preview-cmd` from the original command arguments.

#### Scenario: defaults provider records preview-cmd

Given the user runs `hams defaults write com.apple.dock autohide -bool true`
When the provider records this to `defaults.hams.yaml`
Then the entry SHALL contain:
```yaml
- urn: urn:hams:defaults:com.apple.dock.autohide
  args:
    domain: com.apple.dock
    key: autohide
    type: bool
    value: "true"
  preview-cmd: "defaults write com.apple.dock autohide -bool true"
```

## MODIFIED

### Sensitive Config Key Detection

`hams config set` SHALL detect sensitive keys using both exact match and substring pattern matching:

- Exact matches: `llm_cli` (existing).
- Substring patterns: keys containing `token`, `key`, `secret`, `password`, `credential`.
- When a sensitive key is detected, the value SHALL be written to `hams.config.local.yaml` instead of `hams.config.yaml`.
- A log message SHALL inform the user that the value was routed to `.local.yaml` for security.

#### Scenario: token key auto-routes to local config

Given the user runs `hams config set notification.bark_token abc123`
When the config module detects `bark_token` contains the substring `token`
Then the value SHALL be written to `hams.config.local.yaml`
And a message SHALL be logged: "Sensitive key 'notification.bark_token' written to .local.yaml".
