# Provider System Spec

**Change**: hams-v1-design
**Capability**: provider-system
**Status**: draft

---

## ADDED Requirements

### Requirement: Provider Interface Lifecycle

The provider interface SHALL define a unified lifecycle with the following phases, each represented as a method on the `Provider` interface:

1. **Register** -- declares provider metadata (name, display-name, platform constraints, depend-on list, hams-flags, auto-inject flags, verb routing table).
2. **Bootstrap** -- ensures the provider's own runtime is available (e.g., Homebrew must exist before `hams brew install` can work). Executes the provider's `depend-on` chain recursively.
3. **Probe** -- queries the host environment for current state of resources managed by this provider. Read-only, safe to run in parallel with other providers.
4. **Plan** -- diffs desired state (Hamsfile) against observed state (refreshed state file) to produce an ordered action list (install, update, remove, skip, retry).
5. **Apply** -- executes the planned actions sequentially within the provider, respecting hook ordering (pre/post/defer).
6. **Remove** -- uninstalls a single resource, updates state to `removed`, and deletes the entry from the Hamsfile.
7. **List** -- returns the provider's view of managed resources (Hamsfile entries diffed against state), not a passthrough to the wrapped CLI's native list.
8. **Enrich** -- asynchronously invokes LLM (via configured CLI subprocess) to generate or update tags and intro descriptions for resources.

Every provider -- builtin or external -- MUST implement Register, Probe, Plan, Apply, and Remove. List and Enrich are REQUIRED for builtin providers and OPTIONAL for external providers.

Bootstrap MAY be a no-op if the provider has no runtime dependency (e.g., `bash` provider on a system where bash is always present).

#### Scenario: Builtin provider implements all phases

WHEN a builtin provider (e.g., Homebrew) is loaded
THEN it SHALL expose Register, Bootstrap, Probe, Plan, Apply, Remove, List, and Enrich methods.

#### Scenario: External plugin provider omits optional phases

WHEN an external plugin provider does not implement Enrich
THEN the provider system SHALL treat Enrich as a no-op for that provider and log a debug-level message.

#### Scenario: Lifecycle phase ordering

WHEN `hams apply` is invoked for a set of providers
THEN the system SHALL execute phases in strict order: Register (all providers) -> Bootstrap (topological order) -> Probe (parallel) -> Plan (all providers) -> Apply (sequential, priority order) -> Enrich (async, non-blocking).

---

### Requirement: Resource Identity Model

The system SHALL support two resource identity schemes, determined by the provider's resource class:

- **Natural package name** (Class 1 -- Package providers): The identity is the canonical package name as known to the wrapped package manager (e.g., `git`, `ripgrep`, `typescript`). No URN prefix. The provider MUST ensure uniqueness within its own namespace.
- **URN identity** (Class 2, 3, 4 -- KV Config, Check-based, Filesystem providers): The identity is `urn:hams:<provider>:<resource-id>` where `<resource-id>` is a user-supplied or auto-generated stable identifier. The `urn` module SHALL validate URN format on creation.

A resource identity MUST be immutable once recorded in state. Renaming a resource identity SHALL be treated as remove-old + add-new.

#### Scenario: Package provider uses natural name

WHEN a user runs `hams brew install ripgrep`
THEN the state file SHALL record the resource with identity `ripgrep` (no URN prefix) under the Homebrew provider section.

#### Scenario: Script-type provider uses URN

WHEN a user defines a bash step with `step: install-nvm` in the Hamsfile
THEN the state file SHALL record the resource with identity `urn:hams:bash:install-nvm`.

#### Scenario: URN validation rejects malformed identifiers

WHEN a URN is provided that does not match the pattern `urn:hams:<provider>:<id>` (e.g., missing provider segment, empty id, containing whitespace)
THEN the system SHALL reject it with an error message specifying the expected format.

#### Scenario: Resource identity immutability

WHEN a resource exists in state with identity `urn:hams:bash:setup-env`
AND the user changes the `step:` field in the Hamsfile to `setup-environment`
THEN the plan phase SHALL treat this as a removal of `urn:hams:bash:setup-env` and addition of `urn:hams:bash:setup-environment`.

---

### Requirement: Probe Contract

The Probe phase SHALL implement four distinct probe strategies based on the resource class. Each strategy defines how the provider determines whether a resource is present, its current state, and what fields are recorded.

#### Class 1: Package Providers

Providers: Homebrew, pnpm, npm, uv, goinstall, cargo, mas, apt, code-ext.

The provider SHALL invoke the wrapped package manager's native list command (e.g., `brew list`, `pnpm list -g --json`, `code --list-extensions`) and parse its output to determine installed packages and versions.

State fields: `app`, `version`, `install-at`, `updated-at`, `checked-at`.

#### Class 2: KV Config Providers

Providers: defaults, git-config, duti.

The provider SHALL invoke the appropriate read-back command (e.g., `defaults read <domain> <key>`, `git config --get <key>`, `duti -x <ext>`) and compare the returned value against the desired value in the Hamsfile.

State fields: `urn`, `value`, `config-at`, `updated-at`, `checked-at`.

#### Class 3: Check-based Providers

Providers: bash, ansible (the `system (chsh/scutil)` provider mentioned in earlier drafts is not shipped in v1).

The provider SHALL execute the user-supplied `check:` command. The exit code determines presence (0 = present, non-zero = absent). The stdout of the check command SHALL be captured as a fingerprint for drift detection.

State fields: `urn`, `value`, `config-at`, `updated-at`, `checked-at`, `check-stdout`.

If no `check:` field is provided, the resource SHALL fall back to always-run semantics (re-applied on every `hams apply`).

#### Class 4: Filesystem Providers

Providers: git-clone, file, download.

The provider SHALL check for the existence of the target path on the filesystem. For git-clone, only remote URL, local path, and default branch are recorded; no commit hash or branch tracking.

State fields: `urn`, `remote` (if applicable), `local-path`, `default-branch` (git-clone only).

#### Scenario: Homebrew probe lists installed packages

WHEN the Homebrew provider executes its Probe phase
THEN it SHALL run the Homebrew native list command, parse the output, and update state entries with current version and `checked-at` timestamp for each installed package.

#### Scenario: Check-based resource with check command

WHEN a bash resource defines `check: "command -v nvm"`
AND the Probe phase executes that command
AND the exit code is 0 with stdout `/Users/user/.nvm/nvm.sh`
THEN the state SHALL record `check-stdout: "/Users/user/.nvm/nvm.sh"` and mark the resource as present.

#### Scenario: Check-based resource without check field

WHEN a bash resource has no `check:` field
THEN the Probe phase SHALL mark the resource as `unknown` and the Plan phase SHALL schedule it for re-apply on every `hams apply`.

#### Scenario: Filesystem probe checks path existence

WHEN a git-clone resource declares `local-path: ~/Projects/dotfiles`
AND the directory `~/Projects/dotfiles` exists on disk
THEN the Probe SHALL mark the resource as present without inspecting git commit history or branch state.

#### Scenario: KV Config probe detects drift

WHEN a defaults resource declares `value: true` for `com.apple.dock autohide`
AND `defaults read com.apple.dock autohide` returns `0` (false)
THEN the Probe SHALL record the observed value `0` in state and the Plan phase SHALL schedule an apply action to correct the drift.

---

### Requirement: Depend-on DAG

Each provider SHALL declare a `depend-on` list in its manifest specifying which other providers (or specific resources within those providers) must be bootstrapped before this provider can operate. The depend-on declarations MAY be platform-conditional (analogous to GitHub Actions `if:` expressions, e.g., `os == "darwin"`).

The provider system SHALL build a directed acyclic graph (DAG) from all provider depend-on declarations and resolve it using topological sort to determine Bootstrap execution order.

#### Scenario: VS Code Extensions provider depends on Homebrew

WHEN the code-ext provider declares `depend-on: [{provider: homebrew, resource: visual-studio-code}]`
AND the system is on macOS
THEN the Bootstrap phase SHALL ensure Homebrew is bootstrapped and `visual-studio-code` is installed before the code-ext provider begins its Probe or Apply phases.

#### Scenario: Platform-conditional dependency

WHEN the Homebrew provider declares `depend-on: [{provider: bash, script: install-homebrew, if: "os == 'darwin' || os == 'linux'"}]`
AND the system is on OpenWrt (which does not match the condition)
THEN the depend-on entry SHALL be skipped and Homebrew SHALL NOT attempt to bootstrap on that platform.

#### Scenario: Cycle detection fails fast

WHEN provider A declares `depend-on: [B]` and provider B declares `depend-on: [A]`
THEN the DAG resolution SHALL detect the cycle immediately and terminate with a fatal error listing the cycle path (e.g., `A -> B -> A`).

#### Scenario: Topological execution order

WHEN the DAG contains: bash (no deps), homebrew (depends on bash), code-ext (depends on homebrew)
THEN Bootstrap SHALL execute in order: bash -> homebrew -> code-ext.

#### Scenario: Multiple providers at same DAG level

WHEN homebrew and apt are both at DAG level 1 (both depend only on bash)
THEN they SHALL be bootstrapped according to the provider execution priority list, not in arbitrary order.

---

### Requirement: Provider Runtime Auto-Bootstrap

Providers that depend on external CLI tools (brew, pnpm, npm, cargo, uv, etc.) SHALL declare their runtime dependency via the `depend-on` mechanism in the manifest. The Bootstrap phase SHALL execute the depend-on chain to auto-install missing tools — providers MUST NOT assume their runtime is pre-installed on the host.

When Bootstrap fails for a provider that has a corresponding hamsfile in the active profile directory, the apply command SHALL treat this as a fatal error and abort with a non-zero exit code. Providers without hamsfiles MAY fail silently during bootstrap (debug-level log).

This ensures that `hams apply` on a fresh machine auto-installs all required toolchains in dependency order (e.g., bash -> homebrew -> pnpm) before applying the user's configuration.

#### Scenario: Bootstrap failure is fatal when hamsfile exists

WHEN the pnpm provider fails to bootstrap (npm not found)
AND the active profile contains `pnpm.hams.yaml`
THEN the apply command SHALL exit with a non-zero code and report the failure, NOT silently skip the provider.

#### Scenario: Bootstrap failure is silent when no hamsfile

WHEN the cargo provider fails to bootstrap (cargo not found)
AND the active profile does NOT contain a `cargo.hams.yaml`
THEN the apply command SHALL skip the provider at debug log level and continue normally.

#### Scenario: Auto-install chain on fresh machine

WHEN `hams apply` runs on a fresh machine with no Homebrew installed
AND the Homebrew provider declares `depend-on: [{provider: bash, script: "install-homebrew"}]`
THEN Bootstrap SHALL invoke the bash provider to run the install script before proceeding with Homebrew operations.

---

### Requirement: Hook Model

Providers SHALL support pre-install, post-install, pre-update, and post-update hooks attached to individual resource entries in the Hamsfile. Hooks are side effects of lifecycle actions and SHALL NOT be recorded as independent resources in the state file.

#### Hook Trigger Condition

Install hooks SHALL only fire on the `NotPresent -> Install` transition. If a resource is already `InstalledOk` in state, install hooks SHALL NOT execute on subsequent `hams apply` invocations.

Update hooks (`pre-update`, `post-update`) SHALL fire when a package is upgraded or reinstalled (e.g., `hams brew upgrade <app>` or `hams brew reinstall <app>`). Update hooks SHALL NOT fire during the initial install — only install hooks fire then.

#### Hook Failure Semantics

- **Pre-hook failure**: The parent resource's install action SHALL NOT execute. Both the pre-hook and the parent resource SHALL be marked `failed` in state.
- **Post-hook failure**: The parent resource SHALL remain marked `ok` in state. The hook SHALL be marked `hook-failed` in state. On the next `hams apply`, the system SHALL retry the post-hook without re-installing the already-present parent resource.

#### Defer Hooks

A hook with `defer: true` SHALL NOT execute immediately after its parent resource's install. Instead, it SHALL be queued and executed after the current provider finishes all its install actions. Deferred hooks execute in the order they were queued (Hamsfile order).

`defer: true` scopes to the current provider only, NOT after all providers complete.

#### Nested Declarations in Hooks

Hooks MAY contain nested provider or app declarations (e.g., a post-install hook for a brew package that runs `hams pnpm add <package>`). These nested declarations SHALL be dispatched to the appropriate provider for execution. The nested provider MUST already be bootstrapped.

#### Hook Lifecycle

Hooks are manually-edited only. Uninstalling a package SHALL delete its hooks from the Hamsfile. Reinstalling the same package does NOT restore previously deleted hooks.

#### Scenario: Update hooks fire on upgrade

WHEN a brew package `neovim` has a post-update hook and the user runs `hams brew upgrade neovim`
THEN the post-update hook SHALL execute after the upgrade completes
AND install hooks SHALL NOT fire (only update hooks).

#### Scenario: Update hooks fire on reinstall

WHEN a brew package `neovim` has pre-update and post-update hooks and the user runs `hams brew reinstall neovim`
THEN pre-update SHALL execute before the reinstall and post-update SHALL execute after
AND install hooks SHALL NOT fire.

#### Scenario: Pre-hook executes before install

WHEN a brew package `neovim` has a pre-install hook `pre: "echo preparing nvim config"`
AND `neovim` is not present in state
THEN the hook SHALL execute before `brew install neovim`, and only if the hook succeeds SHALL the install proceed.

#### Scenario: Pre-hook failure blocks install

WHEN a brew package `neovim` has a pre-install hook that exits with code 1
THEN `brew install neovim` SHALL NOT execute
AND the state SHALL record both the hook and `neovim` as `failed`.

#### Scenario: Post-hook failure keeps parent ok

WHEN a brew package `neovim` installs successfully
AND its post-install hook `post: "ln -s ~/.config/nvim ..."` fails with exit code 1
THEN `neovim` SHALL remain `ok` in state
AND the hook status SHALL be `hook-failed`
AND the next `hams apply` SHALL retry the post-hook without re-running `brew install neovim`.

#### Scenario: Deferred hook execution order

WHEN the Homebrew provider is applying packages [git, ripgrep, neovim] in Hamsfile order
AND `git` has a post-hook with `defer: true`
AND `neovim` has a post-hook with `defer: true`
THEN both deferred hooks SHALL execute after all three packages are installed, in order: git's hook first, then neovim's hook.

#### Scenario: Hooks do not fire for already-installed resources

WHEN `git` is already `InstalledOk` in state
AND `git` has a post-install hook
AND `hams apply` is run
THEN the hook SHALL NOT execute.

#### Scenario: Nested provider call in hook

WHEN a brew package `visual-studio-code` has a post-hook containing `hams code-ext install ms-python.python`
THEN the hook execution engine SHALL dispatch the code-ext install to the code-ext provider
AND the code-ext provider MUST already be bootstrapped (else the hook fails).

---

### Requirement: Provider Manifest Format

Each provider SHALL declare a manifest containing the following metadata fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | YES | Machine-readable provider name (lowercase, alphanumeric + hyphens). Used in URNs, CLI commands, file names. |
| `display-name` | string | YES | Human-readable name with proper capitalization. Used in file names (e.g., `Homebrew.hams.yaml`), TUI output, documentation. |
| `platform` | list\<string\> | NO | Supported platforms (`darwin`, `linux`, `openwrt`). If omitted, provider is available on all platforms. |
| `depend-on` | list\<DependOnEntry\> | NO | Provider bootstrap dependencies. See Depend-on DAG requirement. |
| `hams-flags` | list\<FlagDef\> | NO | Provider-specific flags using `--hams-` prefix (e.g., `--hams-tag`, `--hams-lucky`). |
| `auto-inject` | map\<string, list\<string\>\> | NO | Flags automatically injected into wrapped CLI commands per verb (e.g., `install: ["-g", "-y"]` for npm). |
| `verb-routing` | map\<string, VerbDef\> | YES | Maps user-facing verbs (install, remove, list, etc.) to provider actions. Defines which verbs are passthrough, which are hams-interpreted. |
| `resource-class` | int (1-4) | YES | Determines probe strategy and identity scheme. |
| `file-prefix` | string | YES | The display-name-cased prefix for Hamsfile and state file names (e.g., `Homebrew` -> `Homebrew.hams.yaml`). |

The manifest SHALL be defined as a Go struct with YAML tags for builtin providers and as a YAML/JSON file for external plugins.

#### Scenario: Homebrew manifest declaration

WHEN the Homebrew builtin provider registers
THEN its manifest SHALL include: `name: "homebrew"`, `display-name: "Homebrew"`, `platform: ["darwin", "linux"]`, `resource-class: 1`, `file-prefix: "Homebrew"`, and verb-routing for install, remove, list, and search.

#### Scenario: Manifest validation on registration

WHEN a provider registers with a manifest missing the required `name` field
THEN the registration SHALL fail with an error message identifying the missing field.

#### Scenario: Auto-inject flags in manifest

WHEN the npm provider manifest declares `auto-inject: {install: ["-g"]}` (global install)
AND a user runs `hams npm install typescript`
THEN the provider SHALL automatically append `-g` to the npm install command (i.e., `npm install -g typescript`).

---

### Requirement: go-plugin Extension Model

External providers SHALL be implemented as separate binary processes communicating with the hams host via `hashicorp/go-plugin` using local gRPC. The system SHALL NOT support stdio-based plugin protocols.

#### Plugin Discovery

The system SHALL discover external plugins by scanning the following locations in order:

1. `${HAMS_CONFIG_HOME}/plugins/<plugin-name>` -- user-installed plugins.
2. `${HAMS_DATA_HOME}/plugins/<plugin-name>` -- runtime-downloaded plugins.

Each plugin directory MUST contain a plugin binary and a `manifest.yaml` file.

#### Plugin Subprocess Model

- The hams host process SHALL launch each external plugin as a subprocess.
- Communication SHALL use local gRPC (Unix domain socket or localhost TCP, as determined by `go-plugin`).
- The host SHALL manage plugin subprocess lifecycle: start on demand, health-check, restart on crash (with backoff), terminate on hams exit.
- Plugin processes SHALL inherit the host's environment variables but SHALL NOT have direct access to the host's in-process state.

#### Plugin Interface

The gRPC service definition SHALL mirror the Provider interface (Register, Bootstrap, Probe, Plan, Apply, Remove, List, Enrich) with protobuf message types for request/response.

The `pkg/sdk` package SHALL provide a Go SDK that implements the plugin server boilerplate so that external provider authors only need to implement the provider logic.

#### Scenario: External plugin loaded from config directory

WHEN a plugin binary exists at `${HAMS_CONFIG_HOME}/plugins/my-provider/my-provider`
AND a `manifest.yaml` exists at `${HAMS_CONFIG_HOME}/plugins/my-provider/manifest.yaml`
THEN hams SHALL discover and load the plugin on startup, register it using the manifest metadata, and communicate via gRPC.

#### Scenario: Plugin crash and restart

WHEN an external plugin process crashes during the Apply phase
THEN the host SHALL log the crash, mark the current resource as `failed` in state, and attempt to restart the plugin (with exponential backoff, max 3 retries) before marking the provider as unavailable for the remainder of the apply.

#### Scenario: No stdio protocol

WHEN an external plugin attempts to communicate via stdin/stdout text protocol
THEN the host SHALL reject the connection and log an error indicating that only gRPC protocol is supported.

---

### Requirement: Builtin vs External Provider Classification

The system SHALL classify all known providers into builtin (compiled into the hams binary) and external (loaded via go-plugin). The following table defines the v1 classification:

| Provider | Type | Resource Class | Platform | Depend-on |
|---|---|---|---|---|
| bash | Builtin | 3 (Check-based) | all | none |
| homebrew | Builtin | 1 (Package) | darwin, linux | bash (install script, darwin/linux only) |
| apt | Builtin | 1 (Package) | linux (debian-based) | none |
| pnpm | Builtin | 1 (Package) | all | bash (install script) or homebrew |
| npm | Builtin | 1 (Package) | all | homebrew (node) or pnpm (node) |
| uv | Builtin | 1 (Package) | all | bash (install script) or homebrew |
| goinstall | Builtin | 1 (Package) | all | homebrew (go) |
| cargo | Builtin | 1 (Package) | all | bash (rustup) or homebrew (rust) |
| code-ext | Builtin | 1 (Package) | darwin, linux | homebrew (visual-studio-code) |
| mas | Builtin | 1 (Package) | darwin | homebrew (mas) |
| git-config | Builtin | 2 (KV Config) | all | none (uses bundled go-git or system git) |
| git-clone | Builtin | 4 (Filesystem) | all | none (uses bundled go-git or system git) |
| defaults | Builtin | 2 (KV Config) | darwin | none |
| duti | Builtin | 2 (KV Config) | darwin | homebrew (duti) |
| ansible | Builtin | 3 (Check-based) | all | bash (pipx install --include-deps ansible) |

In v1, 15 builtin providers are shipped (bash counts once; `git` gives two providers — `git-config` + `git-clone`). External providers (loaded via `hashicorp/go-plugin` local gRPC) are **interface-defined but not shipped in v1** — see `Provider Plugin Interface` section below for the contract. Planned-but-not-shipped providers (`system` OS config, generic `file` writer, `download` URL fetch) are **not present** in v1.

#### Scenario: Builtin provider available without plugin setup

WHEN a user installs hams and runs `hams brew install git`
THEN the Homebrew provider SHALL be available without any plugin directory or external binary, because it is compiled into the hams binary.

#### Scenario: External provider deferred in v1

WHEN a user attempts to use an ansible provider in v1
THEN the system SHALL report that the ansible provider is not available in this release and suggest checking future versions.

---

### Requirement: Provider Execution Priority

When multiple providers are ready to execute (i.e., all their DAG dependencies are satisfied and they are at the same topological level), the system SHALL determine their execution order using a configurable priority list.

The default priority list SHALL be:

```
brew, apt, pnpm, npm, uv, goinstall, cargo, code-ext, mas, git-config, git-clone, defaults, duti, bash, ansible
```

This list is overridable via the `provider-priority` field in `hams.config.yaml`.

Providers not present in the priority list SHALL be appended after all listed providers, sorted alphabetically by provider name.

#### Scenario: Default priority ordering

WHEN `hams apply` has providers [defaults, homebrew, pnpm] all at the same DAG level
THEN they SHALL execute in order: homebrew, pnpm, defaults (following the default priority list).

#### Scenario: Custom priority override

WHEN `hams.config.yaml` contains `provider-priority: [pnpm, homebrew, defaults]`
AND `hams apply` has providers [defaults, homebrew, pnpm] all at the same DAG level
THEN they SHALL execute in order: pnpm, homebrew, defaults.

#### Scenario: Unlisted provider sorts to end

WHEN a custom external provider `my-provider` is loaded
AND it is not in the priority list
AND the priority list contains [bash, homebrew]
THEN `my-provider` SHALL execute after all providers in the priority list.

#### Scenario: DAG takes precedence over priority

WHEN homebrew depends on bash
AND the priority list orders homebrew before bash
THEN bash SHALL still execute before homebrew because DAG ordering takes precedence over priority ordering.

---

### Requirement: Write-Serial Execution Model

The provider system SHALL enforce a "read parallel, write serial" execution model.

#### Parallel Reads (Probe Phase)

All provider Probe phases MAY execute concurrently. Probe operations are read-only and SHALL NOT mutate state files, Hamsfiles, or the host environment.

#### Serial Writes (Apply Phase)

All Apply phase operations SHALL execute sequentially. Within the Apply phase:

1. Providers execute one at a time, in the order determined by DAG + priority.
2. Within a single provider, resources execute sequentially in Hamsfile declaration order.
3. All Hamsfile writes and state file writes SHALL be serialized through the `hamsfile` module's global mutex.

#### Future Extensibility

The architecture SHALL use interfaces and channels (not direct function calls or shared mutable state) for Apply phase coordination, so that future versions can introduce parallel provider execution without breaking the existing contract.

#### Scenario: Parallel probe execution

WHEN `hams apply` is invoked with providers [homebrew, pnpm, defaults]
THEN all three providers' Probe phases MAY run concurrently
AND no provider's Probe SHALL write to any state file during this phase.

#### Scenario: Sequential apply execution

WHEN `hams apply` has providers [homebrew, pnpm] ready to apply
THEN homebrew's entire Apply phase (all resources + hooks) SHALL complete before pnpm's Apply phase begins.

#### Scenario: Resource ordering within provider

WHEN the Homebrew Hamsfile lists resources in order: [git, ripgrep, neovim]
THEN the Apply phase SHALL install them in that exact order: git first, ripgrep second, neovim third.

#### Scenario: State file write serialization

WHEN the homebrew provider completes installing `git` and needs to write state
AND the pnpm provider's Probe phase is running concurrently (in a future parallel-Apply version)
THEN the state write SHALL acquire the global mutex, blocking any concurrent write until complete.

---

### Requirement: Provider CLI Wrapping

Each provider SHALL wrap an existing CLI tool and provide the following CLI integration capabilities.

#### Verb Routing

The provider's `verb-routing` manifest field SHALL define how user-facing verbs map to provider actions:

- **hams-interpreted verbs**: Verbs that hams intercepts and handles with its own logic (e.g., `list` shows Hamsfile-vs-state diff, not raw `brew list` output).
- **passthrough verbs**: Verbs that are forwarded directly to the wrapped CLI with hams recording the result (e.g., `install`, `remove`).

The provider SHALL validate that the user-supplied verb is in the verb-routing table. Unknown verbs SHALL produce an error with a list of available verbs.

#### Subcommand Parsing

The provider SHALL parse its own subcommands independently of hams global flag parsing. The parsing boundary is:

```
hams <global-flags> <provider-name> <provider-subcommand> <args> --hams-flags -- <passthrough-args>
```

Everything after `<provider-name>` is owned by the provider's parser until `--` is encountered.

#### Auto-Inject Flags

The provider manifest MAY declare flags that are automatically injected into the wrapped CLI command per verb. For example:

- npm `install` auto-injects `-g` (global).
- brew `install` auto-injects nothing (Homebrew defaults are sufficient).
- pnpm `add` auto-injects `-g` (global).
- Most install verbs auto-inject `-y` or equivalent non-interactive flag where available.
- Package name arguments MAY have `@latest` appended for providers that support version suffixes, unless the user specifies a version.

Auto-injected flags SHALL appear in `--help` output with a note indicating they are auto-injected by hams.

#### `--hams-` Prefix Flags

Provider-specific flags that are NOT forwarded to the wrapped CLI MUST use the `--hams-` prefix (e.g., `--hams-tag=dev,cli`, `--hams-lucky`). This prefix prevents collision with the wrapped CLI's own flags.

Exception: `--help` needs no prefix and is intercepted by the provider to display hams-augmented help (provider verbs + hams flags + wrapped CLI help).

#### Force-Forward Separator

The `--` separator and everything after it in the command line SHALL be forwarded verbatim to the wrapped CLI command, bypassing all hams and provider flag parsing. The `--` itself MUST be preserved in the forwarded arguments so that the wrapped CLI can use it to distinguish its own flags from positional arguments (e.g., `cargo run -- -v` requires the `--` to separate Cargo flags from the compiled binary's flags).

#### Scenario: Verb routing for hams-interpreted list

WHEN a user runs `hams brew list`
THEN the Homebrew provider SHALL NOT execute `brew list` as a passthrough
BUT SHALL instead display a diff between the Homebrew Hamsfile entries and the current state.

#### Scenario: Auto-inject -g for npm install

WHEN a user runs `hams npm install typescript`
THEN the npm provider SHALL execute `npm install -g typescript` (with `-g` auto-injected)
AND the `--help` output for `hams npm install` SHALL document that `-g` is auto-injected.

#### Scenario: --hams-tag flag not forwarded

WHEN a user runs `hams brew install git --hams-tag=dev,vcs`
THEN `--hams-tag=dev,vcs` SHALL be consumed by the provider for Hamsfile tagging
AND SHALL NOT be forwarded to `brew install`.

#### Scenario: Force-forward with -- separator

WHEN a user runs `hams brew install ffmpeg -- --with-libvpx --with-sdl2`
THEN the `--` separator and subsequent arguments `--with-libvpx --with-sdl2` SHALL be forwarded verbatim to `brew install ffmpeg`, preserving the `--` in the forwarded argument list.

#### Scenario: Force-forward preserves -- for wrapped CLI argument parsing

WHEN a user runs `hams cargo run -- -v --flag`
THEN the provider SHALL forward `["run", "--", "-v", "--flag"]` to `cargo`
AND `-v` and `--flag` SHALL be treated as arguments to the compiled binary (not as Cargo flags) because the `--` is preserved.

#### Scenario: Unknown verb error

WHEN a user runs `hams brew upgrade git`
AND `upgrade` is not in the Homebrew provider's verb-routing table
THEN the system SHALL print an error listing available verbs (e.g., install, remove, list, search, enrich).

#### Scenario: --help intercepted by provider

WHEN a user runs `hams brew install --help`
THEN the provider SHALL display combined help: hams-specific options (--hams-tag, --hams-lucky), auto-injected flags, and the wrapped `brew install --help` output.

---

### Requirement: Provider Registration and Discovery

The system SHALL support two registration mechanisms:

1. **Builtin registration**: Builtin providers register via Go `init()` functions or Fx module provides, adding themselves to a global provider registry during application startup.
2. **Plugin discovery registration**: External plugins are discovered from the plugin directories (see go-plugin Extension Model), their `manifest.yaml` is parsed, and they are registered into the same provider registry.

The registry SHALL reject duplicate provider names. If two providers attempt to register with the same `name`, the system SHALL fail with an error identifying both sources.

All registered providers SHALL be available for inspection via `hams provider list` (or equivalent diagnostic command).

#### Scenario: Builtin providers auto-registered on startup

WHEN hams starts up
THEN all compiled-in builtin providers SHALL be registered in the provider registry without any user configuration.

#### Scenario: Duplicate provider name rejected

WHEN a builtin provider `homebrew` is registered
AND an external plugin also declares `name: "homebrew"` in its manifest
THEN the system SHALL fail with an error: `duplicate provider name "homebrew": builtin conflicts with plugin at <path>`.

#### Scenario: Provider listing

WHEN a user runs the provider diagnostic command
THEN the output SHALL list all registered providers with their name, type (builtin/external), resource class, and platform constraints.

---

### Requirement: Platform Filtering

Providers that declare a `platform` list in their manifest SHALL only be loaded and registered on matching platforms. Platform detection SHALL use the Go `runtime.GOOS` value mapped to the hams platform identifiers (`darwin`, `linux`, `openwrt`).

A provider with no `platform` declaration SHALL be available on all platforms.

#### Scenario: macOS-only provider on Linux

WHEN hams runs on Linux
AND the `mas` provider declares `platform: ["darwin"]`
THEN the mas provider SHALL NOT be registered and SHALL NOT appear in provider listings.

#### Scenario: Cross-platform provider

WHEN the bash provider declares no `platform` field
THEN it SHALL be registered and available on darwin, linux, and openwrt.

---

### Requirement: Bootstrap Self-Install

When a provider's underlying CLI tool is not present on the system, the provider's Bootstrap phase SHALL attempt to install it using its declared `depend-on` chain.

The Bootstrap phase SHALL:

1. Check if the provider's CLI tool is available (e.g., `which brew`, `which pnpm`).
2. If not available, resolve the depend-on chain to find the installation path.
3. Execute the installation (e.g., run the Homebrew install script via the bash provider).
4. Re-check that the CLI tool is now available.
5. Record the bootstrap status in state (`bootstrapped: true` with timestamp).

If bootstrap fails, the provider SHALL be marked as unavailable for the current run, its resources SHALL be marked `failed` in state with a message referencing the bootstrap failure, and execution SHALL continue with the next provider.

#### Scenario: Homebrew bootstraps via bash install script

WHEN hams runs on a fresh macOS machine without Homebrew
AND the Homebrew provider declares `depend-on: [{provider: bash, script: install-homebrew}]`
THEN the Bootstrap phase SHALL execute the Homebrew install script via the bash provider
AND verify that `brew` is available afterward.

#### Scenario: Bootstrap failure marks provider unavailable

WHEN the Homebrew install script fails (exit code non-zero)
THEN the Homebrew provider SHALL be marked unavailable
AND all Homebrew resources SHALL be marked `failed` in state with reason `provider bootstrap failed`
AND execution SHALL proceed to the next provider.

#### Scenario: Bootstrap skipped when tool present

WHEN `brew` is already available on PATH
THEN the Homebrew provider's Bootstrap phase SHALL be a no-op and SHALL proceed directly to Probe.

---

### Requirement: Provider Error Handling

Provider errors SHALL be categorized and handled according to the following rules:

1. **Bootstrap error**: Provider is marked unavailable for the remainder of the run. All its resources are marked `failed`. Other providers continue.
2. **Probe error**: Individual resource probe failures SHALL be logged and the resource marked `probe-failed` in state. The provider SHALL continue probing remaining resources. A provider-level probe failure (e.g., the CLI tool crashed) SHALL mark all resources as `probe-failed`.
3. **Apply error**: Individual resource apply failures SHALL mark that resource `failed` in state. The provider SHALL continue applying remaining resources (fail-forward within provider). Hook failures follow the hook failure semantics defined above.
4. **Remove error**: Failed removal SHALL mark the resource `remove-failed` in state. The Hamsfile entry SHALL NOT be deleted on removal failure.

All errors SHALL include: provider name, resource identity, phase name, wrapped CLI exit code (if applicable), stderr output (truncated to 1KB), and a suggested fix command when possible.

#### Scenario: Apply continues after single resource failure

WHEN the Homebrew provider is applying [git, ripgrep, neovim]
AND `ripgrep` installation fails
THEN `ripgrep` SHALL be marked `failed` in state
AND the provider SHALL continue to install `neovim`
AND the final summary SHALL report 2 succeeded, 1 failed.

#### Scenario: Error includes actionable information

WHEN `brew install ripgrep` fails with exit code 1 and stderr "Error: ripgrep: no bottle available"
THEN the error report SHALL include: provider `homebrew`, resource `ripgrep`, phase `apply`, exit code `1`, stderr excerpt, and a suggested command (e.g., `brew install --build-from-source ripgrep`).

#### Scenario: Remove failure preserves Hamsfile entry

WHEN `brew uninstall htop` fails
THEN `htop` SHALL remain in `Homebrew.hams.yaml`
AND state SHALL record `htop` as `remove-failed`.

### Requirement: Write-then-install atomicity sequence

The provider system SHALL follow a strict write-then-install sequence for all install operations:

1. Write the resource entry to `<Provider>.hams.yaml` via the hamsfile SDK.
2. Set the resource state to `pending` in `<Provider>.state.yaml`.
3. Execute the wrapped install command.
4. On success: update state to `ok` with observed values.
5. On failure: state remains `failed`, YAML entry is NOT rolled back (desired state is truth).

If step 3 succeeds but step 4 fails (e.g., state file write error), the resource SHALL be marked `installed-but-not-recorded` in a recovery log. The next `hams apply` or `hams refresh` SHALL detect this condition and auto-repair by re-probing the resource and updating state.

#### Scenario: Normal install sequence

WHEN `hams brew install htop` is executed
THEN the system SHALL first write `htop` to `Homebrew.hams.yaml` with state `pending`
AND then execute `brew install htop`
AND on success update state to `ok` with the installed version.

#### Scenario: Install failure preserves desired state

WHEN `brew install htop` fails with a network error
THEN `htop` SHALL remain in `Homebrew.hams.yaml` (not rolled back)
AND state SHALL record `htop` as `failed` with the error message
AND the next `hams apply` SHALL retry installing `htop`.

#### Scenario: State write failure after successful install

WHEN `brew install htop` succeeds but the state file write fails (e.g., disk full)
THEN hams SHALL log `htop` as `installed-but-not-recorded` in the session log
AND the next `hams refresh` SHALL detect `htop` is installed via `brew list` and repair the state entry to `ok`.

### Requirement: Future Bun/TS SDK for external providers

The provider system design SHALL acknowledge that a Bun/TypeScript SDK for external provider authors is planned for a future version. The Go SDK (`pkg/sdk/`) SHALL be the only supported SDK in v1, but the `hashicorp/go-plugin` gRPC interface SHALL use language-neutral protobuf definitions so that a TypeScript client can be generated from the same `.proto` files.

#### Scenario: Protobuf definitions are language-neutral

WHEN the go-plugin gRPC service is defined
THEN the service definition SHALL use `.proto` files (not Go-specific code generation)
AND the `.proto` files SHALL be stored in a location accessible for future SDK generation (e.g., `pkg/sdk/proto/`).

---
<!-- Merged from change: fix-v1-planning-gaps -->

# Provider System — Spec Delta (fix-v1-planning-gaps)

## MODIFIED

### Update Hooks Invocation

The provider executor SHALL invoke pre-update and post-update hooks during resource upgrade operations, following the same contract as install hooks:

- `RunPreUpdateHooks()` SHALL execute before the update action.
- `RunPostUpdateHooks()` SHALL execute after a successful update action.
- Pre-update hook failure SHALL prevent the update and mark the resource as `failed`.
- Post-update hook failure SHALL mark the resource as `hook-failed` (the update itself succeeded).
- Deferred update hooks (`defer: true`) SHALL be collected and executed after all resources in the current provider are processed.

#### Scenario: Package upgrade triggers update hooks

Given a Hamsfile entry for `htop` with a `post-update` hook that runs `htop --version > /tmp/htop-version`
When `hams apply` detects that `htop` needs upgrading (state version differs from available)
Then the provider executor SHALL run the post-update hook after the upgrade completes
And the state SHALL record hook success or failure independently from the upgrade result.

### Provider List Diff

The `List()` method on every provider SHALL compare desired resources (from Hamsfile) against observed resources (from state) and present a diff:

- Resources in Hamsfile but not in state SHALL be marked as additions (`+`).
- Resources in state but not in Hamsfile SHALL be marked as removals (`-`).
- Resources in both but with divergent status SHALL be marked as mismatches (`~`).
- Output SHALL support both human-readable (colored) and `--json` machine-readable formats.

#### Scenario: hams brew list shows diff

Given a Hamsfile containing `git` and `curl`, and state containing `git` and `wget`
When the user runs `hams brew list`
Then the output SHALL show `curl` as `+` (desired but not installed), `wget` as `-` (installed but not desired), and `git` as matched.
