# sudo-isolation

## Purpose

Defines the dependency-injection boundary that isolates `hams` unit tests from the host's real `sudo(8)` binary. The `internal/sudo` package exposes two orthogonal interfaces (`Acquirer` for credential lifecycle, `CmdBuilder` for command construction) plus ready-to-use test doubles (`NoopAcquirer`, `SpyAcquirer`, `DirectBuilder`, `RecordingBuilder`) so that every caller — `runApply`, `apt.Provider`, and any future privileged provider — can be exercised without ever triggering a password prompt. Real sudo behavior (root skip, NOPASSWD success, no-sudoers failure) is verified exclusively through a `//go:build sudo`-tagged suite that runs inside an `e2e/sudo` Docker target, invoked locally by `task test:sudo` and in CI by the `sudo:` job.

## Requirements

### Requirement: Split Sudo Interfaces

The `internal/sudo` package SHALL expose two orthogonal interfaces — `Acquirer` and `CmdBuilder` — so that consumers depend only on the responsibility they use.

- `Acquirer` SHALL declare exactly two methods: `Acquire(ctx context.Context) error` and `Stop()`.
- `CmdBuilder` SHALL declare exactly one method: `Command(ctx context.Context, name string, args ...string) *exec.Cmd`.
- The production `Manager` type SHALL implement `Acquirer`; the production `Builder` type SHALL implement `CmdBuilder`.
- No package SHALL depend on a combined sudo interface. Consumers that only construct commands SHALL NOT depend on `Acquirer`; consumers that only manage lifecycle SHALL NOT depend on `CmdBuilder`.

#### Scenario: Apt provider depends on CmdBuilder only

- **WHEN** `apt.Provider` is constructed
- **THEN** it SHALL receive a `sudo.CmdBuilder` via its constructor parameter
- **AND** its struct SHALL NOT hold a reference to any `sudo.Acquirer`
- **AND** it SHALL use the injected `CmdBuilder.Command` for every privileged subprocess (`apt-get install`, `apt-get remove`, `HandleCommand` install/remove paths).

#### Scenario: Apply flow depends on Acquirer only

- **WHEN** `runApply` begins its execution
- **THEN** it SHALL receive a `sudo.Acquirer` via a function parameter
- **AND** it SHALL call `Acquirer.Acquire(ctx)` at most once before dispatching to providers
- **AND** it SHALL call `Acquirer.Stop` via `defer` before returning
- **AND** it SHALL NOT reference any `sudo.CmdBuilder`, `sudo.Manager`, or `sudo.Builder` directly.

---

### Requirement: In-Package Test Doubles

The `internal/sudo` package SHALL ship production-quality test doubles that consumers can use without defining their own mocks.

- `NoopAcquirer` SHALL implement `Acquirer` with `Acquire` always returning `nil` and `Stop` as a no-op.
- `SpyAcquirer` SHALL implement `Acquirer` while recording call counts for `Acquire` and `Stop` under a mutex.
- `DirectBuilder` SHALL implement `CmdBuilder` by returning `exec.CommandContext(ctx, name, args...)` with no sudo wrapping.
- `RecordingBuilder` SHALL implement `CmdBuilder` by recording `{Name, Args}` under a mutex and returning a harmless command that exits zero without touching the host (e.g., `/bin/true`).
- All four doubles SHALL live in `internal/sudo/noop.go` alongside production types, not in a separate test-only subpackage.

#### Scenario: Injecting NoopAcquirer in CLI tests

- **WHEN** a test in `internal/cli` calls `runApply` or constructs `NewApp`
- **THEN** it SHALL pass `sudo.NoopAcquirer{}` (or `&sudo.SpyAcquirer{}` when call assertions are needed)
- **AND** the test SHALL NOT observe any `Password:` prompt
- **AND** no `sudo(8)` process SHALL be spawned.

#### Scenario: Injecting RecordingBuilder in apt tests

- **WHEN** a test in `internal/provider/builtin/apt` exercises `Apply`, `Remove`, or `HandleCommand`
- **THEN** it SHALL construct the provider as `apt.New(&sudo.RecordingBuilder{})`
- **AND** it MAY assert the captured `RecordingCall` entries to verify command name and args
- **AND** no real `apt-get` or `sudo` process SHALL be spawned.

---

### Requirement: isRoot Branch Coverage Without Real Root

The `internal/sudo` package SHALL expose an overridable `isRoot` sentinel so unit tests can exercise both the root-skips-sudo and non-root-wraps-sudo branches of `Builder.Command` without requiring the test process to actually run as uid 0.

- `isRoot` SHALL be a package-level `var` of type `func() bool`, defaulting to `func() bool { return os.Getuid() == 0 }`.
- Tests that flip `isRoot` SHALL restore the original value in `t.Cleanup`.
- The `isRoot` seam SHALL exist only inside `internal/sudo`. No package outside `internal/sudo` SHALL reference it.

#### Scenario: Testing the non-root branch as a non-root developer

- **WHEN** a unit test wants to verify that `Builder.Command("apt-get", "install", "-y", "htop")` prepends `sudo`
- **THEN** it SHALL set `isRoot = func() bool { return false }` inside a `t.Cleanup`-guarded override
- **AND** it SHALL invoke `Builder.Command` and assert the returned `cmd.Args` begins with `"sudo"`
- **AND** the test SHALL pass regardless of whether the developer is running as root or non-root.

#### Scenario: Testing the root branch as a non-root developer

- **WHEN** a unit test wants to verify that `Builder.Command` skips the `sudo` prefix when running as root
- **THEN** it SHALL set `isRoot = func() bool { return true }` inside a `t.Cleanup`-guarded override
- **AND** it SHALL assert the returned `cmd.Args[0]` equals the requested program name (e.g., `"apt-get"`)
- **AND** no `sudo` process SHALL be spawned.

---

### Requirement: Hermetic `go test ./...`

Running `go test ./...` (and therefore `task test:unit`) on any host — root or non-root, with or without sudoers entries — SHALL NOT prompt for a password, spawn a `sudo(8)` process, or touch any privileged resource.

- The deprecated package-level helper `sudo.RunWithSudo` SHALL NOT exist in the codebase. All callers SHALL have migrated to `CmdBuilder.Command`.
- `internal/cli` tests SHALL inject `NoopAcquirer`/`SpyAcquirer` into `runApply` and `NewApp`.
- `internal/provider/builtin/apt` tests SHALL inject `DirectBuilder` or `RecordingBuilder` into `apt.New`.
- Any future provider that needs privileged execution SHALL take a `sudo.CmdBuilder` via its constructor, following the apt precedent.

#### Scenario: Running the unit test suite on a non-root laptop

- **WHEN** a developer runs `task test:unit` on a macOS or Linux laptop as a non-root user
- **THEN** every package under `./...` SHALL complete without blocking on interactive input
- **AND** the terminal SHALL NOT display `Password:`
- **AND** `ps` SHALL show no `sudo` processes spawned by the test binaries.

#### Scenario: No sudo.RunWithSudo callers remain

- **WHEN** the repository is searched with `rg 'sudo\.RunWithSudo' --type go`
- **THEN** the search SHALL return zero matches
- **AND** `grep -r 'RunWithSudo' internal/ cmd/` SHALL return zero matches.

#### Scenario: CI and Taskfile enforce the no-RunWithSudo invariant

- **WHEN** the `lint:no-run-with-sudo` Taskfile task runs (locally or via the `lint-no-run-with-sudo` CI job)
- **THEN** it SHALL grep `RunWithSudo` across `internal`, `cmd`, and `pkg`
- **AND** it SHALL exit zero when there are no matches
- **AND** it SHALL exit non-zero with a descriptive error when any match reappears
- **AND** the `lint` aggregate Taskfile task SHALL include `lint:no-run-with-sudo` so `task lint` fails the same way.

---

### Requirement: Build-Tag Gated Real-Sudo Tests

Tests that exercise real `sudo(8)` behavior SHALL live in source files guarded by the `//go:build sudo` constraint, so they are literally invisible to `go test ./...`.

- Real-sudo tests SHALL reside in `internal/sudo/sudo_sudo_test.go`.
- The file SHALL begin with `//go:build sudo` followed by a blank line before `package sudo`.
- Tests SHALL skip themselves via `t.Skip` when the running uid does not match the scenario they verify (e.g., `TestAcquire_AsRoot_*` skips unless `os.Getuid() == 0`).
- The real-sudo suite SHALL cover at minimum:
  - `Manager.Acquire` succeeding under root (no-op path).
  - `Manager.Acquire` succeeding under a non-root user with `NOPASSWD` sudoers.
  - `Manager.Acquire` failing under a non-root user with no sudoers entry (no TTY, no password).
  - `Builder.Command` skipping the sudo prefix under root and successfully executing the resulting command.
  - `Builder.Command` prepending `sudo` under a non-root user and successfully executing via `NOPASSWD`.

#### Scenario: Default go test ignores real-sudo tests

- **WHEN** a developer runs `go test ./internal/sudo/...` without `-tags=sudo`
- **THEN** the Go toolchain SHALL NOT compile `sudo_sudo_test.go`
- **AND** none of the `TestAcquire_As*` or `TestBuilder_As*_With*` tests SHALL appear in the test output.

#### Scenario: Real-sudo tests enabled under the sudo tag

- **WHEN** a developer runs `go test -tags=sudo ./internal/sudo/...` inside the `e2e/sudo` Docker image as root
- **THEN** the Go toolchain SHALL compile `sudo_sudo_test.go`
- **AND** `TestAcquire_AsRoot_Succeeds` and `TestBuilder_AsRoot_SkipsSudo` SHALL execute and pass
- **AND** the `*_AsNonRoot_*` tests SHALL self-skip via `t.Skip`.

---

### Requirement: Docker-Based Real-Sudo Verification Target

The repository SHALL provide a Docker-based target that exercises the real-sudo test suite across all three privilege scenarios (root, non-root with NOPASSWD, non-root without sudo).

- `e2e/sudo/Dockerfile` SHALL pin a reproducible base image (e.g., `golang:1.25-bookworm`), install `sudo`, and provision three identities:
  - `root` (default uid 0).
  - `testuser`, a non-root user with `NOPASSWD: ALL` sudoers entry.
  - `nosudouser`, a non-root user with no sudoers entry at all.
- `e2e/sudo/run-tests.sh` SHALL dispatch the test suite across all three identities using `su <user> -c`, selecting the correct `-run` pattern for each scenario.
- The target SHALL be invokable locally via `task test:sudo`, which SHALL run the CI job through `act` so local and CI execution follow the same path.
- The CI workflow (`.github/workflows/ci.yml`) SHALL include a `sudo:` job that builds (or reuses) the image and runs the script.
- The CI job SHALL key its image cache on a content hash derived from `e2e/sudo/Dockerfile`, `go.mod`, and `go.sum`, and SHALL prune stale tags after a successful build.

#### Scenario: `task test:sudo` runs the full real-sudo suite locally

- **WHEN** a developer runs `task test:sudo` on a host with Docker available
- **THEN** the task SHALL execute the `sudo:` CI job via `act`
- **AND** the run SHALL complete with all `TestAcquire_*` and `TestBuilder_*` cases passing or self-skipping appropriately
- **AND** the run SHALL NOT require any sudo password or host-level privilege escalation outside Docker.

#### Scenario: CI sudo job verifies the failure path

- **WHEN** `e2e/sudo/run-tests.sh` dispatches as `nosudouser`
- **THEN** `TestAcquire_AsNonRoot_WithoutSudo_Fails` SHALL execute
- **AND** `Manager.Acquire` SHALL return a non-nil error (no TTY + no cached sudo + no sudoers = acquisition fails)
- **AND** the test SHALL assert that error and pass.

#### Scenario: CI sudo job verifies the NOPASSWD path

- **WHEN** `e2e/sudo/run-tests.sh` dispatches as `testuser`
- **THEN** `TestAcquire_AsNonRoot_WithNOPASSWD_Succeeds` SHALL execute
- **AND** `TestBuilder_AsNonRoot_PrependsSudo` SHALL execute and verify that running `sudo id -u` returns `0`
- **AND** both tests SHALL pass without any password prompt.

---

### Requirement: Informative Sudo Acquisition Message

The production `Manager.Acquire` path SHALL print an explanatory line before invoking interactive `sudo -v`, so users who see the `Password:` prompt understand why it appeared.

- When `isRoot()` returns false and `checkSudo()` reports no cached credential, `Manager.Acquire` SHALL write a one-line explanation to `os.Stderr` before spawning `sudo -v`.
- The line SHALL name the binary (`hams`) and the reason (operations require sudo).
- When `isRoot()` returns true, or when `checkSudo()` confirms a cached credential, no message SHALL be printed.

#### Scenario: Non-root user with no cached sudo sees context before prompt

- **WHEN** a non-root user runs a hams command that triggers `Manager.Acquire` with no cached sudo credential
- **THEN** stderr SHALL first display a line identifying hams as the requester and sudo as the reason (e.g., "hams needs sudo for some operations.")
- **AND** only after that line SHALL the `Password:` prompt from `sudo -v` appear.

#### Scenario: Root user sees no message

- **WHEN** a root user runs a hams command that triggers `Manager.Acquire`
- **THEN** `Manager.Acquire` SHALL return `nil` immediately
- **AND** no stderr message SHALL be printed
- **AND** no `sudo` process SHALL be spawned.
