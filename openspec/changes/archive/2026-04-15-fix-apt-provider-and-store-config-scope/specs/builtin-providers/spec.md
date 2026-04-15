# Builtin Providers — Delta for fix-apt-provider-and-store-config-scope

## MODIFIED Requirements

### Requirement: apt Provider

The apt provider SHALL wrap `apt-get install`, `apt-get remove`, and `dpkg -s` for Debian/Ubuntu-based Linux systems. It is Linux-only and requires sudo for all mutating operations.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `apt` |
| Display name | `apt` |
| File | `apt.hams.yaml` |
| Resource class | Package |
| Platform | Linux only |
| depend-on | none |
| Priority | 3 |

**Hamsfile schema**: Follows CP-1 (Package Provider Common Pattern).

**Auto-inject flags:**

- `-y` SHALL be auto-injected on all `apt-get install` and `apt-get remove` commands to avoid interactive confirmation.
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
| `hams apt install <pkg>` | 1. Invoke `runner.Install(pkg)`. 2. On success, load the effective `apt.hams.yaml` (+ `.local.yaml` override), append `{app: <pkg>}` to the default group if absent, and write the file atomically via the hamsfile SDK. 3. On failure, return the error without modifying the hamsfile. |
| `hams apt remove <pkg>` | 1. Invoke `runner.Remove(pkg)`. 2. On success, load the effective `apt.hams.yaml`, remove the `{app: <pkg>}` entry via the hamsfile SDK's `RemoveItem` method, and write atomically. Missing entry SHALL be a silent no-op on the hamsfile side. 3. On failure, return the error without modifying the hamsfile. |
| `hams apt list` | Diff view (`FormatDiff` of desired vs observed). |
| `hams apt search <query>` | Passthrough to `apt search`. |
| `hams apt show <pkg>` | Passthrough to `apt show`. |
| Any other verb | Passthrough to `apt-get <verb> <args>`. |

**Stdout/stderr policy:**

All commands invoked through the `CmdRunner` interface's real implementation (both Apply/Remove executor paths AND the `hams apt install`/`remove` CLI paths) SHALL stream stdout and stderr to the user's terminal in real time. Stdout/stderr SHALL NOT be silenced, redirected to `io.Discard`, or buffered. Dry-run mode (`--dry-run` flag) SHALL print the equivalent command to stdout without executing it and SHALL NOT invoke the `CmdRunner`.

**State ownership:**

The apt provider's CLI handlers (`hams apt install`, `hams apt remove`) SHALL NOT write directly to `<store>/.state/<machine-id>/apt.state.yaml`. State file mutation remains the exclusive responsibility of the executor (`provider.Executor`). CLI handlers declare intent by updating the hamsfile; `hams apply` reconciles state.

**LLM enrichment:**

- Source: `apt show <package>` provides `Description`, `Homepage`.

#### Scenario: Install an apt package updates hamsfile

- **WHEN** the user runs `hams apt install bat` on a Debian system where `bat` is not yet in `apt.hams.yaml`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, "bat")` which executes `sudo apt-get install -y bat` with stdout/stderr streaming to the user's terminal
- **AND** on success, SHALL append `{app: bat}` to `apt.hams.yaml` via the hamsfile SDK
- **AND** SHALL NOT directly modify `apt.state.yaml`
- **AND** on the next `hams apply`, the executor SHALL probe, observe bat installed, and transition the state entry to `state: ok` with `first_install_at` set.

#### Scenario: Install command failure leaves hamsfile untouched

- **WHEN** the user runs `hams apt install nonexistent-pkg-xyz`
- **AND** `runner.Install` returns an error (exit code non-zero from `apt-get`)
- **THEN** the apt provider SHALL return the error to the user
- **AND** `apt.hams.yaml` SHALL NOT contain a new entry for `nonexistent-pkg-xyz`.

#### Scenario: Install is idempotent on the hamsfile

- **WHEN** the user runs `hams apt install bat` and `bat` is already present in `apt.hams.yaml`
- **THEN** the apt provider SHALL still invoke `runner.Install(ctx, "bat")` (apt-get itself is idempotent)
- **AND** SHALL NOT create a duplicate entry for `bat` in `apt.hams.yaml` — the hamsfile SHALL contain exactly one `{app: bat}` entry after the command completes.

#### Scenario: Remove an apt package updates hamsfile

- **WHEN** the user runs `hams apt remove bat` on a Debian system where `bat` is present in `apt.hams.yaml`
- **THEN** the apt provider SHALL invoke `runner.Remove(ctx, "bat")` which executes `sudo apt-get remove -y bat` with stdout/stderr streaming
- **AND** on success, SHALL remove the `{app: bat}` entry from `apt.hams.yaml` via the hamsfile SDK
- **AND** SHALL NOT directly modify `apt.state.yaml`
- **AND** on the next `hams apply`, the executor SHALL transition the state entry to `state: removed` with `removed_at` set and `first_install_at` preserved.

#### Scenario: Remove command failure leaves hamsfile untouched

- **WHEN** the user runs `hams apt remove bat`
- **AND** `runner.Remove` returns an error (exit code non-zero from `apt-get`)
- **THEN** the apt provider SHALL return the error to the user
- **AND** `apt.hams.yaml` SHALL still contain the `{app: bat}` entry.

#### Scenario: Remove of absent hamsfile entry is a no-op on the file

- **WHEN** the user runs `hams apt remove bat` and `bat` is NOT present in `apt.hams.yaml`
- **AND** `runner.Remove` succeeds (apt-get is idempotent: removing an already-removed package returns 0)
- **THEN** the apt provider SHALL complete successfully without modifying `apt.hams.yaml` and without error.

#### Scenario: Stdout and stderr are not silenced

- **WHEN** the user runs `hams apt install bat`
- **THEN** all output from `sudo apt-get install -y bat` (progress lines, "Setting up bat (...)" messages, errors) SHALL appear on the user's terminal in real time
- **AND** SHALL NOT be buffered, discarded, or captured by hams.

#### Scenario: Dry-run does not invoke the command runner

- **WHEN** the user runs `hams apt install bat --dry-run` (or an equivalent global `--dry-run` flag)
- **THEN** the apt provider SHALL print the equivalent command (`[dry-run] Would install: sudo apt-get install -y bat`) to stdout
- **AND** SHALL NOT call `runner.Install`
- **AND** SHALL NOT modify `apt.hams.yaml`.

#### Scenario: Probe apt packages via CmdRunner

- **WHEN** the apt provider runs probe for a state file containing resources `bat` and `jq`, both in `state: ok`
- **THEN** the provider SHALL call `runner.IsInstalled(ctx, "bat")` and `runner.IsInstalled(ctx, "jq")`
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

#### Scenario: Unit test with fake CmdRunner detects missing hamsfile update

- **WHEN** a unit test injects a fake `CmdRunner` that records every call and invokes `HandleCommand(ctx, ["install", "bat"])`
- **THEN** the test SHALL assert that `runner.Install` was called exactly once with `pkg == "bat"`
- **AND** SHALL assert that `apt.hams.yaml` on the test tempdir now contains `{app: bat}`
- **AND** SHALL fail if either assertion fails — the fake does not shell out to real `apt-get`, making this assertion runnable on any developer's machine regardless of OS or privilege level.
