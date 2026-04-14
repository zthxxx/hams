# Dev Sandbox — Design Proposal (v4)

## Why

Developers need a fast feedback loop when changing hams code. Unit tests are isolated (no real system); `task test:e2e` via `act` takes minutes per iteration. There's no lightweight way to interactively probe hams behavior against a real Linux environment during development.

## What

Add a `task dev EXAMPLE=<name>` workflow that:

1. Auto-creates `examples/<name>/` from `examples/.template/` if missing (with store_path pre-seeded in the global config).
2. Builds a per-example Docker image (with `apt-get update` pre-warmed; optional package installs per example).
3. Launches a throwaway container `hams-<example>` with directory bind-mounts for binary, config, store, and state.
4. Runs a Go-native file watcher (`fsnotify`-based) on the host that rebuilds `bin/hams-linux-<arch>` on `.go` save, using `$GOCACHE` for mandatory incremental compilation. Container sees the new binary on next invocation via directory mount.
5. Prints `docker exec -it hams-<example> bash` as the attach hint; also exposes `task dev:shell EXAMPLE=<name>`.
6. On `Ctrl+C`, stops the container (auto `--rm` removes it).

Per-example container singleton (`hams-<example>`) is the user's responsibility — the scripts do not check for pre-existing containers of the same example. Different examples run in parallel without conflict.

## Architecture

```
host: task dev EXAMPLE=basic-debian
 │
 ├─ (A) ensure-example.sh
 │       └─ if examples/basic-debian/ missing → cp -r examples/.template examples/basic-debian/
 │           (template includes config/hams.config.yaml with
 │            store_path: /workspace/store pre-seeded)
 │
 ├─ (B) build-image.sh
 │       └─ docker build -t hams-dev-<name> \
 │              -f examples/<name>/Dockerfile examples/<name>/
 │           (Docker layer cache handles incremental; no manual hash tagging)
 │
 ├─ (C) detect-arch
 │       └─ uname -m → GOARCH (amd64|arm64)
 │
 ├─ (D) initial build (GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0):
 │       go build -o bin/hams-linux-$GOARCH ./cmd/hams
 │       LDFLAGS version=dev commit=$(git rev-parse --short HEAD)
 │
 ├─ (E) start-container.sh
 │       docker run -d --name hams-<example> --rm \
 │         --user $(id -u):$(id -g) \
 │         -v $(pwd)/bin:/hams-bin:ro \
 │         -v $(pwd)/examples/<name>/config:/home/dev/.config/hams \
 │         -v $(pwd)/examples/<name>/store:/workspace/store \
 │         -v $(pwd)/examples/<name>/state:/workspace/store/.state \
 │         -e HAMS_CONFIG_HOME=/home/dev/.config/hams \
 │         hams-dev-<name> sleep infinity
 │       # after start, create arch-resolved symlink inside container:
 │       docker exec hams-<example> ln -sf /hams-bin/hams-linux-$GOARCH /usr/local/bin/hams
 │
 ├─ (F) print: "Attach with: docker exec -it hams-<example> bash  (or: task dev:shell EXAMPLE=<name>)"
 │
 └─ (G) go run ./internal/devtools/watch --arch $GOARCH
         fsnotify on ./cmd ./internal ./pkg (recursive, auto-add new subdirs)
         debounce 500ms, coalesce concurrent saves
         invokes: GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -o bin/hams-linux-$GOARCH ./cmd/hams
         relies on $GOCACHE for incremental compilation (mandatory — build time MUST stay sub-second on small edits)
         prints: "[watch] built <commit-sha> in 1.2s"

Ctrl+C: trap → docker stop hams-<example> → --rm cleans up
```

## Decisions

### Arch auto-detection (addresses M-series amd64 emulation perf)

```
HOST              →  CONTAINER
darwin/arm64      →  linux/arm64  (native, fast)
darwin/amd64      →  linux/amd64  (native, fast)
linux/{arch}      →  linux/{arch} (native)
```

`scripts/commands/dev/detect-arch.sh` maps `uname -m` → GOARCH. Taskfile's existing `build:linux` stays at amd64 (used by CI which runs on ubuntu-amd64). `task dev` uses its own arch-aware build path.

### Directory bind-mount (addresses binary staleness)

Bind-mounting a single file at host path pins the container to the original inode; `go build`'s atomic rename produces a new inode the container never sees.

Fix: mount `bin/` as a directory (`-v $(pwd)/bin:/hams-bin:ro`). Filename lookups resolve fresh each call.

### Arch resolution via symlink, not wrapper script

Container arch is fixed at `docker run` time — it cannot change during a dev session. Resolving arch on every invocation via a shell wrapper is wasted branching. Simpler: the host (which already knows `$GOARCH` from `detect-arch.sh`) creates a symlink at container start:

```sh
docker exec hams-<example> ln -sf /hams-bin/hams-linux-$GOARCH /usr/local/bin/hams
```

No `RUN` line in the Dockerfile, no `/bin/sh` wrapper, no per-call `uname` branching. The symlink target resolves through the bind-mounted directory, so filename lookups still see freshly rebuilt binaries.

### File watcher: custom Go module at `internal/devtools/watch/` (mandatory incremental builds)

**Hard requirement**: incremental compilation is non-negotiable — the project will grow large and full rebuilds will become unacceptable. The watcher MUST drive `go build` in a way that leverages `$GOCACHE`.

reflex/watchexec cannot be used because they only trigger a command on file change — they have no knowledge of Go's build cache semantics and no way to guarantee `GOCACHE` is persisted/shared with the invoked build. air/CompileDaemon also force a run phase after build, which breaks our Linux-binary-on-macOS-host case.

**Placement**: the watcher is Go code, so it lives where Go code belongs in this repo — `internal/devtools/watch/main.go`. It is invoked as `go run ./internal/devtools/watch` from shell scripts. Under `scripts/` we keep only bash (dynamic shell orchestration); any non-trivial tooling implemented in Go becomes a proper module under `internal/devtools/`.

Dependency: `github.com/fsnotify/fsnotify`. Behaviour:

1. **Recursive watching**: fsnotify does not recurse by default. On start, `filepath.WalkDir` over `./cmd ./internal ./pkg` and `Add()` each directory. On `Create` events for directories, add a watcher for the new directory too.
2. **Debounce**: 500ms quiet window. Every save resets the timer; build fires after 500ms of no further changes.
3. **Coalesce concurrent saves**: if a save arrives while a build is in flight, set a `pending` flag; when current build finishes and flag is set, run another build immediately. Never queue more than one pending build.
4. **Build env**: `GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0` — without these, macOS hosts produce a darwin binary that the container cannot execute. `go build` uses the default `$GOCACHE` for incremental compilation; the watcher MUST NOT pass `-a` or clear the cache.
5. **Output**: on success, prints `[watch] built <short-commit-sha> in 1.2s`. On failure, prints stderr and keeps watching — next save retries.

### Per-example Dockerfile with pre-warmed apt

Each example ships its own `Dockerfile`. `examples/.template/Dockerfile` is the default baseline:

```dockerfile
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash git sudo \
 && rm -rf /var/lib/apt/lists/*
RUN apt-get update  # keep lists primed for in-container apt install
RUN useradd -m -u 1000 -s /bin/bash dev && \
    echo "dev ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/dev
WORKDIR /workspace
# /usr/local/bin/hams is created at `docker run` time by start-container.sh via:
#   docker exec <container> ln -sf /hams-bin/hams-linux-<arch> /usr/local/bin/hams
# so no arch branching lives in the image.
```

Image tag strategy: `hams-dev-<example>` (no version/hash suffix). Docker's layer cache handles incremental rebuilds automatically; a plain `docker build` on unchanged sources is a ~100ms no-op. This keeps `build-image.sh` simple (no hash math, no orphan-tag cleanup).

### `hams --version` format

The current format is incorrect relative to the intended convention, so it is fixed as part of this change (not scope creep — pre-existing defect that also happens to improve watch-loop feedback). `cmd/hams/main.go` (where `app.Version` is set) formats as:

```
dev builds:     "dev (a6f4218)"
release builds: "v1.2.4 (a6f4218)"
```

Implementation: `fmt.Sprintf("%s (%s)", version.Version(), version.Commit())`. No change to ldflags logic; `VERSION` var stays `"dev"` for unpinned builds, becomes `v1.2.4` via release ldflags.

### Examples layout + auto-create

hams has two layers of config: the **global** config at `$HAMS_CONFIG_HOME/hams.config.yaml` (with `store_path`) and the **store** config at `<store>/hams.config.yaml` (with `profile_tag`, `machine_id`). The template seeds both so the user can run `hams apply` inside the container with no flags.

```
examples/
  .template/                    # default skeleton, copied on missing EXAMPLE
    Dockerfile
    config/
      hams.config.yaml          # global: store_path: /workspace/store
                                # → container ~/.config/hams/
    store/
      hams.config.yaml          # store:  profile_tag: dev
                                #         machine_id:  sandbox
      dev/                      # profile_tag dir, hamsfiles live here
        .gitkeep
    state/                      # empty; hams writes .state artifacts here
      .gitkeep                  # → container /workspace/store/.state
  basic-debian/                 # example scenario
    Dockerfile                  # optional override
    config/hams.config.yaml
    store/
      hams.config.yaml
      dev/
        apt.hams.yaml
        bash.hams.yaml
    state/                      # tracked; hams writes here (artifact of the scenario)
```

**Everything under `examples/` is git-tracked** — `config/`, `store/`, and `state/`. Examples are first-class fixtures that document a complete hams scenario end-to-end, including the state produced by running `hams apply`. The only thing excluded is build/tool cache (e.g., `examples/*/.cache/` if any ever appears); no such caches exist today.

State leakage across dev sessions is accepted — developers know what they're doing. No auto-wipe, no `dev:clean` task. To reset: `git checkout examples/<name>/state/` (since it's tracked) or edit manually. Committing state changes after a meaningful run is encouraged when the resulting state is itself the point of the example.

### Script organization

Shell scripts live under `scripts/commands/dev/` (always run from project root). Go helpers live under `internal/devtools/` and are invoked as `go run` targets:

```
scripts/commands/dev/
  main.sh                 # entry point, arg parsing, orchestration, trap
  ensure-example.sh       # copy .template → examples/<name> if missing
  detect-arch.sh          # uname -m → echo GOARCH
  build-image.sh          # docker build -t hams-dev-<example>
  start-container.sh      # docker run with bind-mounts + post-start symlink

internal/devtools/
  watch/
    main.go               # fsnotify (recursive) + debounce + coalesce + incremental go build loop
```

Rule: `scripts/` is for bash only (dynamic shell orchestration). Any non-trivial tooling that must be written in Go belongs in `internal/devtools/<tool>/` and is invoked from bash via `go run ./internal/devtools/<tool>`.

Taskfile.yml stays thin:

```yaml
dev:
  desc: 'Start dev sandbox with hot reload (usage: task dev EXAMPLE=basic-debian)'
  cmds:
    - bash scripts/commands/dev/main.sh --example {{.EXAMPLE}}
  requires:
    vars: [EXAMPLE]

dev:shell:
  desc: 'Attach interactive bash to running container (usage: task dev:shell EXAMPLE=basic-debian)'
  cmds:
    - docker exec -it hams-{{.EXAMPLE}} bash
  requires:
    vars: [EXAMPLE]
```

### File ownership

`docker run --user $(id -u):$(id -g)` makes container writes carry the host user's uid/gid. `.state/` files created during `hams apply` are cleanable without `sudo` on Linux hosts.

## Components

| Component | Responsibility |
|-----------|----------------|
| `scripts/commands/dev/main.sh` | Orchestrator: ensure → build image → initial build → start container → print hint → watch → trap cleanup |
| `scripts/commands/dev/ensure-example.sh` | Auto-create example from `.template/` on first use |
| `scripts/commands/dev/detect-arch.sh` | Map host arch → linux GOARCH |
| `scripts/commands/dev/build-image.sh` | `docker build -t hams-dev-<example>` (relies on Docker layer cache) |
| `scripts/commands/dev/start-container.sh` | `docker run` with correct mounts and user mapping; post-start `docker exec ... ln -sf` to create arch-resolved `/usr/local/bin/hams` |
| `internal/devtools/watch/main.go` | Recursive fsnotify watch, 500ms debounce, coalesce concurrent saves, `GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build` with mandatory `$GOCACHE` incremental compilation |
| `examples/.template/` | Default Dockerfile + pre-seeded config/hams.config.yaml (store_path) + store/hams.config.yaml (profile_tag) + empty state/ |
| `examples/<name>/` | Per-scenario config, Dockerfile overrides, hamsfiles |
| `Taskfile.yml` | Thin wrappers: `dev`, `dev:shell` |
| `cmd/hams/main.go` | `--version` format change: `<version> (<commit>)` |

## Error Handling

- **`EXAMPLE` not provided**: `task dev` exits with usage hint and list of existing examples.
- **Docker daemon not running**: surface docker error, exit non-zero.
- **Initial `go build` fails**: exit before starting container, no cleanup needed.
- **`hams-<example>` container already exists** (prior run left it via SIGKILL): `docker stop hams-<example> || true` first, then start fresh. Per-example singleton is the user's responsibility; the scripts perform no cross-example coordination.
- **Example's Dockerfile build fails**: surface docker error, exit before continuing.
- **watcher Go build fails mid-session**: watch.go logs error, keeps watching — next save retries.

## Out of Scope

- Cross-example singleton enforcement. `hams-<example>` containers are per-example; running two instances of the *same* example is user-managed (the scripts stop-then-restart to recover from crashes but do not warn about clobbering).
- SIGKILL cleanup of orphaned containers (user runs `docker rm -f hams-<example>` manually).
- Non-Linux sandboxes (Linux containers only; macOS/Windows host → linux guest via Docker Desktop).
- Automatic `hams apply` on binary update (developer drives the container manually).
- Port forwarding (no current use case).
- Watching `.yaml` config changes (hot reload is binary-only; config edits are picked up on next `hams` invocation via R/W mount anyway).
- MOTD banner in the attached shell (not worth the Dockerfile noise; if developers want it, they can add to their own example's Dockerfile).
