# dev-sandbox Specification

## Purpose
TBD - created by archiving change dev-sandbox. Update Purpose after archive.
## Requirements
### Requirement: Example scenario fixtures under `examples/`

The system SHALL store developer sandbox scenarios as self-contained, git-tracked fixtures under `examples/<name>/`, where each fixture captures the complete input and output of a hams run (config, store, state).

The template skeleton at `examples/.template/` SHALL be the canonical baseline and SHALL be copied verbatim (minus itself) into `examples/<name>/` on first use of a new example name.

Every directory under `examples/<name>/` — including `config/`, `store/`, and `state/` — SHALL be git-tracked. Only future build or tool caches (e.g., `examples/*/.cache/`) MAY be excluded via `.gitignore`.

The template SHALL pre-seed the following so the container can run `hams apply` with no flags:

- `config/hams.config.yaml` — global config with `store_path: /workspace/store`.
- `store/hams.config.yaml` — store config with `profile_tag: dev` and `machine_id: sandbox`.
- `store/dev/` — profile directory where hamsfiles live.
- `state/` — empty directory where hams writes `.state` artifacts.

#### Scenario: Example auto-created from template on first use

- **WHEN** a developer runs `task dev EXAMPLE=basic-debian` and `examples/basic-debian/` does not exist
- **THEN** the orchestrator copies `examples/.template/` to `examples/basic-debian/`
- **AND** the copy includes `Dockerfile`, `config/hams.config.yaml`, `store/hams.config.yaml`, `store/dev/.gitkeep`, `state/.gitkeep`
- **AND** the developer can edit the new directory before re-running

#### Scenario: Existing example is preserved

- **WHEN** a developer runs `task dev EXAMPLE=basic-debian` and `examples/basic-debian/` already exists
- **THEN** the orchestrator does not overwrite any file under `examples/basic-debian/`
- **AND** proceeds directly to image build and container start

#### Scenario: Example state is git-trackable

- **WHEN** a developer runs `hams apply` inside the sandbox and hams writes `.state` artifacts under `examples/basic-debian/state/`
- **THEN** `git status` shows those files as tracked modifications (not ignored)
- **AND** the developer can either commit the state (to freeze the scenario) or `git checkout` to reset

### Requirement: Sandbox container lifecycle

The system SHALL launch one Docker container per example, named `hams-<example>`. Container lifecycle SHALL be: start on `task dev`, stop and auto-remove on `Ctrl+C`.

The container SHALL run with `--rm` and `--user $(id -u):$(id -g)` so files written inside the container carry the host user's ownership.

The container SHALL bind-mount the following paths (read-write unless noted):

- Host `./bin/` → container `/hams-bin/` (read-only) — for arch-suffixed binaries.
- Host `examples/<name>/config/` → container `$HOME/.config/hams/` — global config.
- Host `examples/<name>/store/` → container `/workspace/store/` — store root.
- Host `examples/<name>/state/` → container `/workspace/store/.state/` — state output.

The container SHALL have `HAMS_CONFIG_HOME` set to `$HOME/.config/hams` via `-e`.

After `docker run`, the orchestrator SHALL create a symlink `/usr/local/bin/hams → /hams-bin/hams-linux-<arch>` via `docker exec ... ln -sf`, where `<arch>` is the host-derived `$GOARCH`. The Dockerfile SHALL NOT contain any arch-branching logic.

The orchestrator SHALL run `docker stop hams-<example> || true` before starting a new container with the same name, to recover from SIGKILL-orphaned containers.

The orchestrator SHALL NOT check for or coordinate with containers belonging to other examples. Cross-example parallel runs SHALL succeed.

#### Scenario: Container starts with correct mounts and user

- **WHEN** the orchestrator starts the sandbox for `basic-debian`
- **THEN** a container named `hams-basic-debian` is running
- **AND** `docker inspect hams-basic-debian` shows all four bind mounts resolved to host paths under the project
- **AND** files created inside the container are owned by the host user's uid/gid

#### Scenario: `/usr/local/bin/hams` resolves to the host-derived arch

- **WHEN** the orchestrator has started the container on an `arm64` host
- **THEN** `docker exec hams-<example> readlink /usr/local/bin/hams` prints `/hams-bin/hams-linux-arm64`
- **AND** `docker exec hams-<example> hams --version` prints a valid version string

#### Scenario: Parallel examples coexist

- **WHEN** `task dev EXAMPLE=a` and `task dev EXAMPLE=b` run in two shells
- **THEN** both containers `hams-a` and `hams-b` are running simultaneously
- **AND** neither stops the other

#### Scenario: Orphaned container is recovered

- **WHEN** a previous run left container `hams-<example>` in exited state (e.g., after SIGKILL)
- **AND** the developer runs `task dev EXAMPLE=<example>` again
- **THEN** the orchestrator runs `docker stop hams-<example>` (no-op or cleanup) then starts fresh
- **AND** does not fail with "container name already in use"

#### Scenario: `Ctrl+C` stops and removes the container

- **WHEN** the developer presses `Ctrl+C` in the `task dev` terminal
- **THEN** the orchestrator traps the signal, runs `docker stop hams-<example>`
- **AND** because `--rm` was set, the container is removed
- **AND** `docker ps -a --filter name=hams-<example>` returns no rows

### Requirement: Runtime host-uid registration inside sandbox

After `docker run`, the orchestrator SHALL ensure the host's uid and gid are registered in the container's `/etc/passwd`, `/etc/group`, and (when present) `/etc/shadow` files. This registration is required so `sudo` — invoked by providers such as `apt` — succeeds when the container is running as the host uid via `docker run --user`.

The registration SHALL be idempotent: uids that already have a passwd entry (e.g., the baked `dev` user at uid 1000) SHALL NOT be re-registered, and the script SHALL NOT fail when the check short-circuits.

The sudoers policy baked into the template Dockerfile SHALL grant passwordless sudo to any resolvable user (`ALL ALL=(ALL) NOPASSWD: ALL`). This is scoped to dev-sandbox images only; it is not a general hams security posture.

#### Scenario: Host uid unknown to container gets passwd and shadow entries

- **WHEN** the orchestrator starts a container on a host where `id -u` returns a value not present in the image's `/etc/passwd`
- **THEN** `docker exec hams-<example> getent passwd $(id -u)` returns a non-empty line naming user `hostuser`
- **AND** `docker exec hams-<example> getent group $(id -g)` returns a non-empty line naming group `hostgroup` (or the baked group if gid collides)
- **AND** `/etc/shadow` contains `hostuser:*:1::::::` so PAM's sudo account check passes
- **AND** `docker exec hams-<example> sudo -n true` exits 0

#### Scenario: Host uid equal to 1000 is not re-registered

- **WHEN** the host uid is `1000` (same as the baked `dev` user)
- **THEN** `start-container.sh` detects the existing passwd entry via `getent`
- **AND** does not append a duplicate `hostuser` line
- **AND** the container starts without error

#### Scenario: Hams apply succeeds end-to-end via sudo

- **WHEN** an example's hamsfile declares an `apt` package (e.g., `htop`)
- **AND** the developer runs `docker exec hams-<example> hams apply`
- **THEN** hams invokes `sudo apt-get install -y htop` inside the container
- **AND** the command succeeds
- **AND** resulting `.state/<machine_id>/apt.state.yaml` is written with state `ok`
- **AND** the state file on the host is owned by the host uid (not root, not uid 1000)

### Requirement: Per-example Dockerfile with shared baseline

The system SHALL provide a baseline `Dockerfile` at `examples/.template/Dockerfile` that includes:

- Base image `debian:bookworm-slim`.
- Installation of `ca-certificates`, `curl`, `bash`, `git`, `sudo` via `apt-get`.
- A second `apt-get update` to keep package lists primed for in-container `apt install` during dev sessions.
- A `dev` user with uid 1000 and passwordless sudo.
- `WORKDIR /workspace`.

The image tag SHALL be `hams-dev-<example>` with no version or hash suffix. Incremental rebuilds SHALL rely on Docker's native layer cache; the orchestrator SHALL NOT compute or manage image hashes.

Each example MAY override the Dockerfile by editing its own `examples/<name>/Dockerfile`.

#### Scenario: Default template image builds cleanly

- **WHEN** the orchestrator runs `docker build -t hams-dev-basic-debian -f examples/basic-debian/Dockerfile examples/basic-debian/` on an unmodified template copy
- **THEN** the build succeeds
- **AND** the resulting image contains a `dev` user with passwordless sudo and no arch-specific wrapper scripts

#### Scenario: Rebuild on unchanged sources is a near-no-op

- **WHEN** the orchestrator rebuilds the image and no files under `examples/<name>/` have changed
- **THEN** all layers hit the Docker build cache
- **AND** the build completes in well under 1 second on a warm daemon

### Requirement: Hot-reload watcher with mandatory incremental builds

The system SHALL provide a Go watcher module at `internal/devtools/watch/main.go`, invoked from shell via `go run ./internal/devtools/watch --arch <GOARCH>`.

The watcher SHALL use `github.com/fsnotify/fsnotify` and SHALL recursively watch `./cmd`, `./internal`, and `./pkg`. When a new subdirectory is created inside a watched tree, the watcher SHALL register a watcher on it.

The watcher SHALL debounce file-change events with a 500 ms quiet window: every save resets the timer, and a build fires only after 500 ms of inactivity.

The watcher SHALL coalesce concurrent saves: if a save arrives while a build is in progress, a single `pending` flag is set; when the current build finishes and the flag is set, one additional build runs. No more than one pending build SHALL ever be queued.

Every build invocation SHALL run as: `GOOS=linux GOARCH=<arch> CGO_ENABLED=0 go build -o bin/hams-linux-<arch> ./cmd/hams`.

The watcher SHALL rely on the default `$GOCACHE` for incremental compilation and SHALL NOT pass `-a` or otherwise invalidate the cache.

On build success, the watcher SHALL emit a structured `log/slog` record with message `build ok` and fields `commit=<short-commit-sha>` and `duration=<formatted>` (matching the project's `log/slog`-first logging convention in `.claude/rules/code-conventions.md`). On build failure, it SHALL emit a `build failed` record with the compiler stderr and keep watching.

#### Scenario: `.go` save triggers a debounced, incremental rebuild

- **WHEN** the developer saves a `.go` file under `./internal/`
- **THEN** within ~500 ms the watcher invokes `go build` with `GOOS=linux`, `GOARCH=<host-arch>`, `CGO_ENABLED=0`
- **AND** output goes to `bin/hams-linux-<arch>`
- **AND** Go's build cache is used (second rebuild of the same file is dramatically faster than a cold build)

#### Scenario: Concurrent saves coalesce into at most one extra build

- **WHEN** ten `.go` files are saved within a 100 ms window while a build is in progress
- **THEN** the in-progress build completes as-is
- **AND** exactly one additional build runs after completion
- **AND** no further builds are queued

#### Scenario: New subdirectory is watched automatically

- **WHEN** the developer creates `./internal/newpkg/` and then saves `./internal/newpkg/foo.go`
- **THEN** the watcher picks up the save event (without being restarted)
- **AND** triggers a rebuild within the debounce window

#### Scenario: Build failure keeps the watcher alive

- **WHEN** a `.go` save introduces a syntax error and `go build` exits non-zero
- **THEN** the watcher prints the compiler stderr
- **AND** remains running
- **AND** the next successful save triggers another build attempt

#### Scenario: Rebuilt binary is visible inside the running container

- **WHEN** the watcher writes a new `bin/hams-linux-<arch>` while `hams-<example>` is running
- **AND** the developer then runs `docker exec -it hams-<example> hams --version`
- **THEN** the version output reflects the commit SHA of the new binary, not the previous one

### Requirement: Task entry points and orchestration script layout

The system SHALL expose two Taskfile targets:

- `task dev EXAMPLE=<name>` — orchestrates the full dev session.
- `task dev:shell EXAMPLE=<name>` — runs `docker exec -it hams-<name> bash`.

Both targets SHALL fail with a clear usage message if `EXAMPLE` is not provided.

Shell orchestration SHALL live under `scripts/commands/dev/` with these files:

- `main.sh` — entry point, arg parsing, orchestration, signal trap.
- `ensure-example.sh` — copies template if the example directory is missing.
- `detect-arch.sh` — maps `uname -m` to a linux `GOARCH` (`amd64` / `arm64`) and echoes it.
- `build-image.sh` — runs `docker build -t hams-dev-<example>`.
- `start-container.sh` — runs `docker run` with the correct mounts and post-start symlink.

`scripts/` SHALL contain bash only. Any non-trivial Go tooling consumed by scripts SHALL live under `internal/devtools/<tool>/` and be invoked via `go run`.

#### Scenario: `task dev` without `EXAMPLE` prints usage

- **WHEN** the developer runs `task dev` with no `EXAMPLE` variable
- **THEN** the command exits non-zero
- **AND** prints a usage hint that names the `EXAMPLE=<name>` argument

#### Scenario: `task dev:shell` attaches to the correct per-example container

- **WHEN** `hams-basic-debian` is running and the developer runs `task dev:shell EXAMPLE=basic-debian`
- **THEN** the shell attaches to container `hams-basic-debian`
- **AND** not to any other `hams-*` container

#### Scenario: Go tooling is not placed under `scripts/`

- **WHEN** the repository is inspected for `.go` files under `scripts/`
- **THEN** none are found
- **AND** all Go helpers used by scripts live under `internal/devtools/<tool>/`

### Requirement: Host architecture detection

The system SHALL detect the host architecture and produce a corresponding linux `GOARCH`:

- `uname -m` = `x86_64` → `amd64`.
- `uname -m` = `aarch64` or `arm64` → `arm64`.

The watcher and the container MUST use the same `GOARCH` within a single `task dev` session. Existing CI build paths (e.g., `build:linux`) MUST NOT be modified; `task dev` uses its own arch-aware build.

#### Scenario: Apple Silicon host produces an arm64 container

- **WHEN** the developer runs `task dev EXAMPLE=<name>` on `darwin/arm64`
- **THEN** `detect-arch.sh` echoes `arm64`
- **AND** the built binary is `bin/hams-linux-arm64`
- **AND** the container `/usr/local/bin/hams` symlink points to `/hams-bin/hams-linux-arm64`
- **AND** `hams --version` executes natively without emulation

#### Scenario: CI build path is untouched

- **WHEN** the `task build:linux` target runs (e.g., in GitHub Actions)
- **THEN** it builds for `linux/amd64` as before
- **AND** its output path and flags are unchanged by this feature

### Requirement: `hams --version` format

The `hams` CLI SHALL print its version as `<version> (<commit>)` for both development and release builds:

- Development builds where `VERSION` ldflag is unset SHALL print `dev (<short-commit>)`.
- Release builds where `VERSION` ldflag is set to a tag SHALL print `<tag> (<short-commit>)`, e.g., `v1.2.4 (a6f4218)`.

Implementation SHALL use `fmt.Sprintf("%s (%s)", version.Version(), version.Commit())` in `cmd/hams/main.go` (or the equivalent package where `app.Version` is assigned).

Existing ldflags logic SHALL NOT be changed; only the format string SHALL be updated.

#### Scenario: Dev build version format

- **WHEN** `go build` is invoked without release ldflags
- **AND** the developer runs `hams --version`
- **THEN** the output matches the pattern `dev (<7-char-hex>)`

#### Scenario: Release build version format

- **WHEN** a release build passes `-ldflags "-X ...Version=v1.2.4 -X ...Commit=a6f4218"`
- **AND** the developer runs `hams --version`
- **THEN** the output is exactly `v1.2.4 (a6f4218)`

### Requirement: Error handling for dev sandbox orchestration

The orchestrator SHALL fail fast with informative output in these cases:

- `EXAMPLE` variable missing → print usage and list of existing example directories, exit non-zero.
- Docker daemon unreachable → surface the `docker` CLI error verbatim, exit non-zero.
- Initial `go build` failure → exit before starting the container; print compiler stderr.
- Example-specific `Dockerfile` build failure → surface the `docker build` error, exit before `docker run`.

The watcher SHALL NOT exit on transient build failures during a session; it SHALL log the error and continue watching.

#### Scenario: Missing Docker surfaces a clear error

- **WHEN** Docker is not running and the developer runs `task dev EXAMPLE=basic-debian`
- **THEN** the orchestrator exits non-zero
- **AND** the output includes the upstream `docker` CLI error message
- **AND** does not proceed to start the watcher

#### Scenario: Initial build failure stops the session

- **WHEN** the current working tree has a compile error in `./cmd/hams`
- **AND** the developer runs `task dev EXAMPLE=basic-debian`
- **THEN** the orchestrator prints the compile error
- **AND** exits before running `docker run`
- **AND** the container is not created

