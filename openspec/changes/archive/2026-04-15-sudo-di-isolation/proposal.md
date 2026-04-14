## Why

Running `task test:unit` triggered an interactive `Password:` prompt because `internal/cli/apply.go` called `sudo.NewManager().Acquire(ctx)` unconditionally inside `runApply`, and `internal/provider/builtin/apt` invoked a package-level `sudo.RunWithSudo` helper. Any test that drove the apply code path (or an apt provider method under a non-root user) escalated straight to the host's real `sudo(8)` — a clear DI/boundary-isolation violation.

The breakage had three distinct symptoms:

1. Unit tests hit real `sudo` on the host; they should run inside Docker-as-root or with a mock.
2. The prompt surfaced as a bare `Password:` with no contextual message explaining why sudo was needed.
3. The `isRoot` branch (sudo vs. no-sudo prefix) had no isolated coverage for both user modes.

## What Changes

- Extract two interfaces in `internal/sudo`: `Acquirer` (lifecycle: `Acquire` / `Stop`) and `CmdBuilder` (`Command(ctx, name, args...) *exec.Cmd`).
- Keep `Manager` + new `SudoBuilder` as the production implementations; add `NoopAcquirer` + `DirectBuilder` for unit-test injection.
- Inject `sudo.CmdBuilder` into `apt.Provider` via its constructor; inject `sudo.Acquirer` into `runApply` / `applyCmd` / `NewApp`.
- Replace every `sudo.RunWithSudo` caller with `p.sudo.Command(...)`; **BREAKING** remove the deprecated `sudo.RunWithSudo` package function.
- Update all unit tests (`internal/cli`, `internal/provider/builtin/apt`, `internal/sudo`) to inject `NoopAcquirer` / `DirectBuilder` so `go test ./...` never prompts for a password.
- Add a `//go:build sudo` test file (`internal/sudo/sudo_sudo_test.go`) that exercises real `Manager.Acquire` and `SudoBuilder.Command` under both root and non-root-with-NOPASSWD.
- Add a Docker target (`e2e/sudo/Dockerfile` + `run-tests.sh`) that runs the build-tagged tests once as root and once as a `NOPASSWD` user; wire it through a new `sudo:` job in `.github/workflows/ci.yml` and a `test:sudo` Taskfile entry.

## Capabilities

### New Capabilities

- `sudo-isolation`: DI-isolation contract for sudo credential acquisition and command wrapping — defines the `Acquirer` / `CmdBuilder` interfaces, mandates test-time noop injection for all callers, and requires a Docker-based target for real-sudo verification.

### Modified Capabilities

_None._ All new contracts (interfaces, test doubles, DI wiring, Docker verification target) live in the new `sudo-isolation` capability. Internal wiring changes to `apt.Provider` and `runApply` are implementation details of that capability and do not alter user-facing behavior in `builtin-providers` or `cli-architecture`.

## Impact

- **Code**: `internal/sudo/{sudo,noop,sudo_sudo_test,sudo_test}.go`, `internal/provider/builtin/apt/{apt,apt_test}.go`, `internal/cli/{apply,apply_test,register,root,root_test}.go`.
- **Infra**: `e2e/sudo/Dockerfile`, `e2e/sudo/run-tests.sh`, `.github/workflows/ci.yml` (new `sudo:` job), `Taskfile.yml` (new `test:sudo` task).
- **Behavior**: `go test ./...` runs hermetically on any host regardless of root/sudoers configuration. Real-sudo regressions are caught by the new Docker job, not by developer laptops.
- **Breaking**: `sudo.RunWithSudo` is removed — any out-of-tree caller must migrate to `sudo.CmdBuilder.Command`.
