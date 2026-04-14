# Dev Sandbox — Design Proposal

## Why

Developers need a fast feedback loop when changing hams code. Today, testing requires either running unit tests (isolated, no real system) or running the full e2e suite via `act` (minutes per iteration). There's no lightweight way to interactively probe hams behavior against a real Linux environment during development.

## What

Add a `task dev` workflow that:

1. Launches a throwaway `debian:bookworm-slim` Docker container with the local Linux binary bind-mounted.
2. Runs `air` on the host to watch Go sources and recompile the Linux binary on every save — container sees the new binary instantly via the bind mount.
3. Prints a `docker exec` command so the developer can attach an interactive shell to the container and run `hams` commands manually.
4. On `Ctrl+C`, stops the container and `--rm` deletes it.

## Architecture

```
host: task dev EXAMPLE=basic-debian
 │
 ├─ (A) Build bin/hams-linux-amd64 with VERSION=dev-<short-sha>
 │
 ├─ (B) docker run -d --name hams-dev --rm \
 │        -v $(pwd)/bin/hams-linux-amd64:/usr/local/bin/hams:ro \
 │        -v $(pwd)/examples/<EXAMPLE>:/workspace/store \
 │        debian:bookworm-slim sleep infinity
 │
 ├─ (C) print: "Attach with: docker exec -it hams-dev bash"
 │
 └─ (D) air .  (watches ./cmd, ./internal, ./pkg; rebuilds bin/hams-linux-amd64)
         │
         └─ container sees new binary on next hams invocation (bind mount)

Ctrl+C: trap → docker stop hams-dev → --rm cleans up
```

### Version hash in dev builds

Current `Taskfile.yml` computes `VERSION` via `git describe --tags --always --dirty`. For release builds this produces semver (e.g., `v0.3.1`). For dev builds (no tag, dirty tree), it already produces `<commit-sha>-dirty` which is suitable as a dev hash.

**Decision**: no change to VERSION logic — the existing `git describe` output already serves both cases. `hams --version` will output e.g., `v0.3.1` on release or `a6f4218-dirty` during dev. Developers can verify the binary updated by re-running `hams --version` inside the attached shell.

### air configuration

`.air.toml` invokes `task build:linux` (or directly `go build` with linux ldflags) to keep the output at `bin/hams-linux-amd64`. Watch directories: `cmd/`, `internal/`, `pkg/`. Extensions: `.go`. Debounce: 500ms.

### examples/ directory layout

```
examples/
  basic-debian/
    hams.config.yaml          # profile_tag: dev, machine_id: sandbox
    dev/
      apt.hams.yaml           # packages: jq
      bash.hams.yaml          # run: echo "hello from dev sandbox"
      git-config.hams.yaml    # user.email: dev@example.com
```

The container mounts this at `/workspace/store` (read-write — hams writes `.state/` into the store). Since examples are gitignored at the state level (`.state/` is in the root `.gitignore`), state artifacts from dev runs don't pollute the repo. Developer runs `hams apply --store /workspace/store` inside the attached shell.

## Components & Responsibilities

| Component | Responsibility |
|-----------|----------------|
| `Taskfile.yml` `dev` task | Orchestrates first build → docker start → print attach hint → exec air → trap Ctrl+C |
| `.air.toml` | Tells air to rebuild `bin/hams-linux-amd64` on `.go` changes |
| `examples/basic-debian/` | Minimum viable scenario: apt + bash + git-config |
| Docker container `hams-dev` | Provides the sandbox environment; binary bind-mounted |

## Error Handling

- **EXAMPLE missing**: `task dev` without `EXAMPLE` → print list of available examples (dirs under `examples/`) and exit 1.
- **air not installed**: `task setup` adds `go install github.com/air-verse/air@latest`. `task dev` checks for `air` binary and errors with install hint if absent.
- **Container already running**: if `hams-dev` container already exists (prior `task dev` didn't clean up), the task stops it first (`docker stop hams-dev || true`) then starts fresh.
- **Initial build fails**: task exits before starting docker, no cleanup needed.
- **air exits unexpectedly**: `trap` handler still cleans up the container.

## Out of Scope

- Multiple concurrent dev containers (always one `hams-dev` at a time).
- Non-Linux sandboxes (dev targets linux/amd64 only; macOS/arm64 devs use Docker Desktop's amd64 emulation).
- Automatic `hams apply` on binary update — developer drives the container manually via attached shell.
- Port forwarding / network setup — none needed for core hams workflow.
