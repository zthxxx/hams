# Builtin Providers — Spec Delta

## MODIFIED Requirements

### Requirement: apt Provider

The `apt` Provider has the same shape as `homebrew`, with Debian/Ubuntu-specific commands:

- **Detect:** `command -v apt-get` and `[ -f /etc/debian_version ]`.
- **Capabilities:** `install`, `remove`, `update`, `upgrade`, `list`, `search`, `show`, `apply`.
- **Flags:**
  - `--no-install-recommends` SHALL be passable through `flags:` per package.
  - `--purge` SHALL be passable for the remove path.
- `sudo` SHALL be auto-injected for all mutating apt commands (apt requires root).

**Command boundary (DI requirement):**

All outbound calls to `apt-get` and `dpkg` SHALL be routed through a dedicated Go interface owned by the apt provider package. The interface SHALL expose at minimum the following methods:

| Method | Real implementation |
|--------|---------------------|
| `Install(ctx, pkg string) error` | `sudo apt-get install -y <pkg>`, streaming stdout/stderr to the user's terminal |
| `Remove(ctx, pkg string) error` | `sudo apt-get remove -y <pkg>`, streaming stdout/stderr |
| `IsInstalled(ctx, pkg string) (installed bool, version string, err error)` | `dpkg -s <pkg>`, parse `Status: install ok installed` line + `Version:` line |

The real implementation SHALL compose with the existing `sudo.CmdBuilder` to acquire root. The interface SHALL be injected via the provider's constructor so that unit tests can substitute an in-memory fake that records call history and maintains a virtual "installed packages" set. Unit tests SHALL NOT shell out to the real `apt-get` or `dpkg` under any circumstance.

**Probe implementation:**

- Iterate resources present in the state file.
- For each resource whose `state != removed`, call `IsInstalled(pkg)` via the command interface.
- Populate `provider.ProbeResult` with `State: ok` + observed `Version` for installed packages, or `State: failed` for any package that is absent or whose `dpkg -s` invocation errors.

**Apply flow (executor path):**

1. `sudo apt-get update` (once per apply session, not per package).
2. For each missing package, call `runner.Install(pkg)`.

**Remove flow (executor path):**

1. For each removal action, call `runner.Remove(pkg)`.
2. Provider SHALL NOT auto-run `apt autoremove` (user decision).

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams apt install <pkg>` | 1. Invoke `runner.Install(pkg)`. 2. On success, load (or create) `apt.state.yaml`, call `state.SetResource(pkg, StateOK, WithVersion(version))` using version captured from `runner.IsInstalled(pkg)`, and persist via atomic write. 3. Load the effective `apt.hams.yaml` (+ `.local.yaml` override), append `{app: <pkg>}` to the default group if absent, and write the file atomically via the hamsfile SDK. 4. On failure of any `runner.Install` call, return the error without modifying the hamsfile or state file (atomic semantics preserved). |
| `hams apt remove <pkg>` | 1. Invoke `runner.Remove(pkg)`. 2. On success, load (or create) `apt.state.yaml`, call `state.SetResource(pkg, StateRemoved)`, and persist via atomic write. 3. Load the effective `apt.hams.yaml`, remove the `{app: <pkg>}` entry via the hamsfile SDK's `RemoveItem` method, and write atomically. Missing entry SHALL be a silent no-op on the hamsfile side. 4. On failure of any `runner.Remove` call, return the error without modifying the hamsfile or state file. |
| `hams apt list` | Diff view (`FormatDiff` of desired vs observed). |
| `hams apt search <query>` | Passthrough to `apt search`. |
| `hams apt show <pkg>` | Passthrough to `apt show`. |
| Any other verb | Passthrough to `apt-get <verb> <args>`. |

**Stdout/stderr policy:**

All commands invoked through the `CmdRunner` interface's real implementation (both Apply/Remove executor paths AND the `hams apt install`/`remove` CLI paths) SHALL stream stdout and stderr to the user's terminal in real time. Stdout/stderr SHALL NOT be silenced, redirected to `io.Discard`, or buffered. Dry-run mode (`--dry-run` flag) SHALL print the equivalent command to stdout without executing it and SHALL NOT invoke the `CmdRunner`, the hamsfile, or the state file.

**State ownership:**

The apt provider's CLI handlers (`hams apt install`, `hams apt remove`) SHALL load and atomically persist `<store>/.state/<machine-id>/apt.state.yaml` after each successful `runner.Install` / `runner.Remove` invocation. State writes from the CLI handlers SHALL go through `state.SetResource(...)` (with `state.WithVersion(...)` for installs) so timestamps (`first_install_at`, `updated_at`, `removed_at`) follow the same lifecycle rules the executor uses. The executor (`provider.Executor`) retains state-write authority for the declarative `hams apply` path. Both writers reuse the same atomic-write helper to avoid partial-state on crash.

**LLM enrichment:**

- Source: `apt show <package>` provides `Description`, `Homepage`.

#### Scenario: Install an apt package updates hamsfile and state

- **WHEN** the user runs `hams apt install htop` on a Debian system where `htop` is not yet in `apt.hams.yaml` and not in `apt.state.yaml`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, "htop")` which executes `sudo apt-get install -y htop` with stdout/stderr streaming to the user's terminal
- **AND** on success, SHALL append `{app: htop}` to `apt.hams.yaml` via the hamsfile SDK
- **AND** on success, SHALL write `apt.state.yaml` with `resources.htop.state = ok`, `first_install_at` set to the current timestamp, `updated_at` equal to `first_install_at`, no `removed_at` key, and `version` populated from `runner.IsInstalled(ctx, "htop")`.

#### Scenario: Install command failure leaves hamsfile and state untouched

- **WHEN** the user runs `hams apt install nonexistent-pkg-xyz`
- **AND** `runner.Install` returns an error (exit code non-zero from `apt-get`)
- **THEN** the apt provider SHALL return the error to the user
- **AND** `apt.hams.yaml` SHALL NOT contain a new entry for `nonexistent-pkg-xyz`
- **AND** `apt.state.yaml` SHALL NOT contain a new entry for `nonexistent-pkg-xyz`.

#### Scenario: Re-install bumps updated_at and preserves first_install_at

- **WHEN** the user runs `hams apt install htop` and `htop` is already present in `apt.state.yaml` with `state: ok`, `first_install_at: T0`
- **THEN** the apt provider SHALL still invoke `runner.Install(ctx, "htop")` (apt-get itself is idempotent)
- **AND** SHALL NOT create a duplicate entry for `htop` in `apt.hams.yaml` — the hamsfile SHALL contain exactly one `{app: htop}` entry after the command completes
- **AND** SHALL update `apt.state.yaml` so `resources.htop.first_install_at = T0` (immutable), `updated_at` equals the new timestamp, and `state = ok`.

#### Scenario: Remove an apt package updates hamsfile and state

- **WHEN** the user runs `hams apt remove htop` on a Debian system where `htop` is present in `apt.hams.yaml` with `first_install_at: T0`
- **THEN** the apt provider SHALL invoke `runner.Remove(ctx, "htop")` which executes `sudo apt-get remove -y htop` with stdout/stderr streaming
- **AND** on success, SHALL remove the `{app: htop}` entry from `apt.hams.yaml` via the hamsfile SDK
- **AND** on success, SHALL update `apt.state.yaml` so `resources.htop.state = removed`, `first_install_at = T0` (preserved), `removed_at` set to the current timestamp, and `updated_at` equal to `removed_at`.

#### Scenario: Remove command failure leaves hamsfile and state untouched

- **WHEN** the user runs `hams apt remove htop`
- **AND** `runner.Remove` returns an error (exit code non-zero from `apt-get`)
- **THEN** the apt provider SHALL return the error to the user
- **AND** `apt.hams.yaml` SHALL still contain the `{app: htop}` entry
- **AND** `apt.state.yaml` SHALL retain the previous resource entry for `htop` unchanged.

#### Scenario: Remove of absent hamsfile entry is a no-op on the file

- **WHEN** the user runs `hams apt remove htop` and `htop` is NOT present in `apt.hams.yaml`
- **AND** `runner.Remove` succeeds (apt-get is idempotent: removing an already-removed package returns 0)
- **THEN** the apt provider SHALL complete successfully without modifying `apt.hams.yaml` and without error
- **AND** SHALL still record `state: removed` for `htop` in `apt.state.yaml` so the audit trail is complete.

#### Scenario: Re-install after remove clears removed_at

- **WHEN** the user runs `hams apt install htop` and `htop` is currently in `apt.state.yaml` with `state: removed`, `first_install_at: T0`, `removed_at: T1`
- **THEN** the apt provider SHALL transition `apt.state.yaml` to `state: ok`, `first_install_at: T0` (preserved), `updated_at: T2` (current time), no `removed_at` key (cleared via YAML omitempty).

#### Scenario: Stdout and stderr are not silenced

- **WHEN** the user runs `hams apt install htop`
- **THEN** all output from `sudo apt-get install -y htop` (progress lines, "Setting up htop (...)" messages, errors) SHALL appear on the user's terminal in real time
- **AND** SHALL NOT be buffered, discarded, or captured by hams.

#### Scenario: Dry-run does not invoke the command runner or touch state

- **WHEN** the user runs `hams apt install htop --dry-run` (or an equivalent global `--dry-run` flag)
- **THEN** the apt provider SHALL print the equivalent command (`[dry-run] Would install: sudo apt-get install -y htop`) to stdout
- **AND** SHALL NOT call `runner.Install`
- **AND** SHALL NOT modify `apt.hams.yaml`
- **AND** SHALL NOT load or modify `apt.state.yaml`.

#### Scenario: Probe apt packages via CmdRunner

- **WHEN** the apt provider runs probe for a state file containing resources `htop` and `jq`, both in `state: ok`
- **THEN** the provider SHALL call `runner.IsInstalled(ctx, "htop")` and `runner.IsInstalled(ctx, "jq")`
- **AND** for each installed result, SHALL emit `ProbeResult{ID, State: ok, Version}`; for each uninstalled result, SHALL emit `ProbeResult{ID, State: failed}`.

#### Scenario: Probe skips removed resources

- **WHEN** the apt provider runs probe for a state file where `htop` is in `state: removed`
- **THEN** the provider SHALL NOT call `runner.IsInstalled(ctx, "htop")` — removed resources are excluded from probe iteration.

#### Scenario: apt on macOS

- **WHEN** the apt provider is loaded on macOS
- **THEN** the provider SHALL report itself as `unsupported` for the current platform and SHALL NOT register any commands. The provider SHALL be silently skipped during apply.

#### Scenario: apt update runs once per session

- **WHEN** multiple apt packages need installation during a single `hams apply`
- **THEN** the apt provider SHALL run `sudo apt-get update` exactly once at the beginning of its apply phase, not before each individual package install.

#### Scenario: Unit test with fake CmdRunner detects missing hamsfile + state update

- **WHEN** a unit test injects a fake `CmdRunner` that records every call and invokes `HandleCommand(ctx, ["install", "htop"])`
- **THEN** the test SHALL assert that `runner.Install` was called exactly once with `pkg == "htop"`
- **AND** SHALL assert that `apt.hams.yaml` on the test tempdir now contains `{app: htop}`
- **AND** SHALL assert that `apt.state.yaml` on the test tempdir now contains `resources.htop.state = ok` with `first_install_at` and `updated_at` populated
- **AND** SHALL fail if any assertion fails — the fake does not shell out to real `apt-get`, making this assertion runnable on any developer's machine regardless of OS or privilege level.

## ADDED Requirements

### Requirement: Per-provider Docker integration tests

Every linux-containerizable builtin provider SHALL own its integration
test under `internal/provider/builtin/<provider>/integration/`, with
exactly two files:

- `Dockerfile` — `FROM hams-itest-base:<base-hash>` plus whatever
  minimal delta the provider needs to run. The Dockerfile SHALL NOT
  pre-install the provider's runtime (e.g., python, node, go, rust,
  brew) — runtime installation is the provider's own responsibility
  at integration-test time, because that is what hams must do for real
  users.
- `integration.sh` — bash script sourcing the shared helpers at
  `/e2e/base/lib/{assertions,yaml_assert,provider_flow}.sh`. At
  minimum, the script SHALL call
  `standard_cli_flow <provider> <install_verb> <existing_pkg> <new_pkg>`
  (or its `post_install_check` variant for providers without a PATH
  binary) to exercise the canonical install / re-install / refresh /
  remove lifecycle.

**In scope** (11 providers): apt, ansible, bash, cargo, git (config +
clone), goinstall, homebrew, npm, pnpm, uv, vscodeext.

**Out of scope** (macOS-only): defaults, duti, mas. No docker path
exists; a macOS CI runner would be required.

**Base image**:

- `e2e/base/Dockerfile` SHALL produce a `hams-itest-base:<sha(Dockerfile)>`
  image containing only `debian:bookworm-slim` + `ca-certificates`,
  `curl`, `bash`, `git`, `sudo`, `yq` (pinned version from GitHub
  releases). No language toolchains, no package managers beyond apt-get.
- The base image SHALL be built once per change to `e2e/base/Dockerfile`
  and cached by content hash (`docker image inspect` gate before
  rebuild).

**Per-provider image**:

- SHALL be named `hams-itest-<provider>:<sha(integration/Dockerfile)>`.
- SHALL `FROM hams-itest-base:<frozen-base-hash>` to reuse base layers.
- SHALL be rebuilt only when its own Dockerfile hash changes.
- Stale tags (same repo, different hash) SHALL be pruned opportunistically.

**Test runtime contract**:

- The `hams` binary SHALL be bind-mounted read-only at `/usr/local/bin/hams`.
- Shared helpers SHALL be bind-mounted read-only at `/e2e/base/lib/`.
- Each provider's integration dir SHALL be bind-mounted read-only at
  `/integration/`, with `integration.sh` executable.
- Tests SHALL run as root inside the container (sudo is a no-op).
- Every docker run SHALL start a fresh container; no state crosses
  between provider tests.

#### Scenario: apt integration test runs in isolation with no other provider bootstrapped

- **WHEN** `task ci:itest:run PROVIDER=apt` executes
- **THEN** the `hams-itest-apt` container starts with only the base image + any apt-specific runtime (none needed; apt-get is pre-installed by debian)
- **AND** the container SHALL NOT contain `brew`, `cargo`, `node`, `python3` beyond what debian's base image provides
- **AND** `integration.sh` SHALL call `standard_cli_flow apt install jq btop` and SHALL assert the install/re-install/refresh/remove lifecycle against `.state/<machine>/apt.state.yaml`
- **AND** at NO point SHALL any other provider (Homebrew, pnpm, cargo, etc.) have `Bootstrap` called during the test, because no hamsfile or state file exists for them.

#### Scenario: Per-provider Dockerfile cache reuses across runs

- **WHEN** the developer runs `task ci:itest:run PROVIDER=apt` twice in a row without changing `e2e/base/Dockerfile` or `internal/provider/builtin/apt/integration/Dockerfile`
- **THEN** the second run SHALL NOT rebuild either image — `docker image inspect hams-itest-base:<hash>` and `docker image inspect hams-itest-apt:<hash>` both succeed, skipping the build step.

### Requirement: `standard_cli_flow` shared helper

`e2e/base/lib/provider_flow.sh` SHALL expose a function
`standard_cli_flow` that implements the canonical CLI-only integration
flow. Every in-scope provider's `integration.sh` SHALL call this helper.

**Signature**:

```
standard_cli_flow <provider> <install_verb> <existing_pkg> <new_pkg> [<post_install_check>]
```

- `<provider>` — hams provider name (e.g., `apt`, `brew`, `pnpm`).
- `<install_verb>` — the provider's install subcommand (`install`, `add`).
- `<existing_pkg>` — a package name that the helper installs first to
  seed state, then re-installs to verify timestamp semantics.
- `<new_pkg>` — a second, distinct package name used for the
  install-new / refresh / remove portion.
- `<post_install_check>` (optional, for providers without a PATH
  binary) — a bash function name that verifies installation succeeded.
  Defaults to `command -v <pkg>` if unset.

**Steps performed** (all assertions fail fast with exit 1):

1. `hams <provider> <install_verb> <existing_pkg>` — seed state.
2. Capture `existing_pkg`'s `first_install_at` from the state file.
3. `sleep 1`.
4. `hams <provider> <install_verb> <existing_pkg>` — assert
   `updated_at` bumped, `first_install_at` unchanged.
5. Pre-check: `command -v <new_pkg>` (or the supplied
   `post_install_check`) SHALL fail.
6. `hams <provider> <install_verb> <new_pkg>` — assert the post-install
   check now succeeds, and assert state has `new_pkg.state = ok`,
   `first_install_at` set, `removed_at` absent.
7. `sleep 1; hams refresh --only=<provider>` — assert `new_pkg`'s
   `updated_at` bumped.
8. `hams <provider> remove <new_pkg>` — assert the post-install check
   fails again, state has `new_pkg.state = removed`, `removed_at` set.

#### Scenario: standard_cli_flow used by apt provider

- **WHEN** `internal/provider/builtin/apt/integration/integration.sh` calls `standard_cli_flow apt install jq btop`
- **THEN** the helper SHALL execute the 8-step lifecycle against the `hams` binary in the container and the state file at `<store>/.state/<machine-id>/apt.state.yaml`
- **AND** any failed assertion SHALL exit non-zero with a descriptive message naming the failed step, path, and expected vs actual values.

#### Scenario: standard_cli_flow used with a post-install check hook

- **WHEN** a provider has no PATH binary (e.g., `git-config` sets a git configuration key rather than installing an executable), and its `integration.sh` supplies a custom `post_install_check` function
- **THEN** `standard_cli_flow` SHALL call the supplied function in place of `command -v <pkg>` at the pre-check and post-remove assertions
- **AND** the rest of the lifecycle (state-file timestamp assertions) SHALL proceed identically.
