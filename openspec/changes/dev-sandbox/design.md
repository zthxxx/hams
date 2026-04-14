# Dev Sandbox — Design

## Context

hams mutates real host state: installs packages, writes config, runs provider binaries. Two verification tiers already exist:

- **Unit tests** — DI-isolated, no side effects, <5s to run. Good for parsing logic, state machines, diff computation.
- **E2E via `act`** — Docker containers, real package managers, matches CI byte-for-byte. Takes minutes per iteration.

Neither tier supports the inner loop a developer actually lives in: *"edit Go code → immediately poke at behavior against a real Linux environment → iterate"*. Today that loop requires either unsafe host experimentation or a 2-minute `act` cycle per change.

**Constraint ceiling**: whatever we build cannot touch the host. hams's first principle is *"you cannot trust any change to the host before development is complete"*. The dev loop must be as containerized as E2E — but as fast as unit tests.

**Stakeholders**: solo-developer ergonomics today; contributors later. Most feedback loops will be driven from macOS (Apple Silicon) against Linux containers, so cross-arch correctness is non-optional.

## Goals / Non-Goals

**Goals:**

- Sub-second turnaround from `.go` save to a fresh Linux binary usable inside a container.
- Zero host mutation. No installs, no config writes, no provider invocations outside Docker.
- A runnable, versioned fixture per example — `examples/<name>/` captures *everything* about a scenario (config, store, resulting state) so it doubles as documentation and an E2E seed.
- Thin orchestration: bash for shell glue, Go for anything non-trivial. No tool buried inside the other's paradigm.
- Trivially extensible: adding a new scenario is `cp -r examples/.template examples/<new>/` followed by edits.

**Non-Goals:**

- Running multiple *instances of the same* example concurrently. Per-example singleton is the user's problem.
- Cross-example coordination. `task dev EXAMPLE=a` and `task dev EXAMPLE=b` run in parallel without talking.
- Hot reload of YAML config. Config is picked up on next `hams` invocation via R/W mounts; no watcher needed.
- Auto-driving `hams apply` on binary rebuild. The developer chooses what to run and when.
- Windows host support. Out of scope per project charter.
- Native (non-Docker) sandboxes. Linux containers only.

## Decisions

### D1 — Docker over VM, chroot, or host execution

Alternatives considered:

| Option | Isolation | Startup | macOS host | Verdict |
|--------|-----------|---------|------------|---------|
| Host execution | None | Instant | Native | ✗ Violates "no host mutation" |
| chroot / `unshare` | Weak (Linux-only) | Fast | Unavailable | ✗ No macOS path |
| Lima / full VM | Strong | 10-30s cold | Works | ✗ Overkill, slow cold start |
| Docker Desktop | Strong | 1-3s cold | Works | ✓ Chosen |
| Podman | Strong | Similar | Works | — Acceptable drop-in; users who prefer it can alias |

**Chose Docker** because: (1) already a prerequisite for `act`-based E2E, so zero new dependency; (2) Docker Desktop handles the macOS→Linux bridge transparently; (3) bind-mounts let host-side `go build` feed container-side execution without any container-side build toolchain.

### D2 — Directory bind-mount for `bin/`, not file-level mount

Mounting a single file pins the container to its inode. `go build`'s atomic rename produces a new inode. The container keeps executing the stale binary until restarted.

Alternatives:

- **File mount + container restart per rebuild** — kills the attached shell on every save. Unacceptable UX.
- **Copy the binary into the container on rebuild** — requires a push mechanism (`docker cp`) inside the watcher. Works, but adds round-trip latency and couples the watcher to container lifecycle.
- **Directory mount** — filename lookups resolve fresh on each `hams` invocation. Rebuild just replaces the file; next call sees it. No restart, no copy, no coupling. ✓ Chosen.

Trade-off: the entire `bin/` is visible inside the container. Acceptable — `bin/` contains only hams build outputs and is already `.gitignore`'d.

### D3 — Custom `internal/devtools/watch/main.go`, not reflex/watchexec/air

The watcher is the most contested sub-decision. Alternatives:

| Tool | Incremental via `$GOCACHE`? | Forces run-phase? | Verdict |
|------|------------------------------|-------------------|---------|
| `air` | Yes (shells out to `go build`) | Yes — runs target after build | ✗ Can't run a Linux binary on macOS |
| `CompileDaemon` | Yes | Yes | ✗ Same as air |
| `reflex` | Only if command uses it | No | ~ Works, but… |
| `watchexec` | Only if command uses it | No | ~ Works, but… |
| Custom Go via `fsnotify` | Yes (explicit) | No | ✓ Chosen |

Why custom wins: **incremental compilation is a hard requirement**. Project will grow large; full rebuilds will become unacceptable. The watcher must invoke `go build` with the default `$GOCACHE` intact and never pass `-a` or clear the cache. With reflex/watchexec this is achievable via the wrapped command, but we *also* need:

1. Debounce (500ms) — reflex/watchexec support this via flags.
2. Coalesce concurrent saves (one pending build, never queue more) — neither tool does this cleanly; they either queue or drop.
3. Recursive new-subdirectory discovery — fsnotify doesn't recurse; we walk and re-`Add()` on `Create`.
4. Commit-SHA-annotated output for feedback alignment with `hams --version`.

(3) and (4) are enough to tip the balance. Once we write Go code for recursion anyway, we may as well own the whole loop and keep behavior observable.

**Placement rule**: `scripts/` is bash-only. Any non-trivial Go tooling lives under `internal/devtools/<tool>/` and is invoked as `go run ./internal/devtools/<tool>`. This keeps lint, test, and module hygiene uniform — a `.go` file orphaned under `scripts/` escapes the normal build graph.

### D4 — Per-example Dockerfile with shared `.template/` baseline

Alternatives:

- **One shared image, install packages at runtime** — every session re-runs `apt-get install`. Slow, and ties image content to scenario state.
- **Per-example image, fully independent Dockerfile** — duplication across examples that only differ in one `RUN` line.
- **Shared `.template/Dockerfile` copied on first use, editable per example** — ✓ Chosen.

Rationale: the `.template/Dockerfile` ships the common baseline (Debian slim + CA certs + curl + bash + git + sudo + `dev` user with passwordless sudo). Examples inherit by copy (explicit) rather than by base-image reference (implicit). This means bisecting a scenario's image never requires time-travel through a template repo.

Image tag: `hams-dev-<example>` with no version/hash suffix. Docker's layer cache handles incremental rebuilds; a rebuild on unchanged sources is a ~100ms no-op.

### D5 — Arch resolution via symlink, not per-call shell wrapper

Container arch is fixed at `docker run` time. The host already knows `$GOARCH` from `detect-arch.sh`. Creating `/usr/local/bin/hams` as a symlink at container start is simpler than a shell wrapper that re-resolves `uname -m` on every invocation.

Alternatives considered:

- **Shell wrapper in `/usr/local/bin/hams`** — works, but branches on every call and bakes arch logic into the image.
- **Mount only the correct binary file at `/usr/local/bin/hams`** — reintroduces the inode problem (D2).
- **Symlink created by `docker exec ... ln -sf` after `docker run`** — ✓ Chosen. Dockerfile stays arch-agnostic; the host owns all arch knowledge.

### D6 — `examples/<name>/{config,store,state}` all git-tracked

Classic instinct: gitignore runtime artifacts like `.state/`. Here that instinct is wrong.

An example's *point* is the end-to-end story: "given this config + this store, running `hams apply` produces this state." If state isn't committed, the example is not fully reproducible by reading the repo — it's reproducible only by running the example, which defeats its value as a fixture.

Only build/tool caches are excluded. No such caches exist today; rule is forward-looking (`examples/*/.cache/` if ever introduced).

Trade-off accepted: state leakage across dev sessions is visible in `git status`. Developers either commit meaningful changes or `git checkout examples/<name>/state/` to reset. This is *more* visibility than gitignoring, which is the right tradeoff for a tool that manages host state.

### D7 — Per-example container name `hams-<example>`

Alternatives:

- **Global singleton `hams-dev`** — `task dev EXAMPLE=a` and `EXAMPLE=b` fight silently. Rejected.
- **Random suffix `hams-dev-<rand>`** — reattach requires lookup. Rejected.
- **Stable per-example `hams-<example>`** — ✓ Chosen. Parallel examples coexist; `task dev:shell EXAMPLE=<name>` is deterministic.

User retains the "single instance of the *same* example" invariant. Scripts perform `docker stop hams-<example> || true` at start (to recover from SIGKILL crashes) but do not warn about clobbering. This matches the user's explicit direction.

### D8 — `hams --version` format fix bundled

`cmd/hams/main.go` currently outputs a version string not matching the project's convention. Fixing it is a pre-existing defect, not dev-sandbox scope creep. Two reasons to bundle:

1. The watcher's success output (`[watch] built <sha> in 1.2s`) aligns with the new version format (`<version> (<sha>)`), so fixing both in one change keeps observability coherent.
2. The fix is mechanical (~3-line `fmt.Sprintf`) and carries no spec risk.

Dissenting view logged: a purist would split this into its own change. The user explicitly decided against splitting.

## Risks / Trade-offs

- **[macOS Docker Desktop bind-mount latency]** → Using directory mounts keeps per-call lookup cost low (tens of ms). Mitigation: if measurements ever show this dominating, switch to VirtioFS-optimized bind options or a named volume + `docker cp` push on rebuild.
- **[Large future projects break sub-second rebuilds]** → Mandatory `$GOCACHE` incremental builds absorb most growth. If incremental alone isn't enough, follow-up work adds selective package rebuilds (watch only touched packages).
- **[`fsnotify` misses new subdirectories on some filesystems]** → We explicitly add watchers on `Create` events for directories. On exotic filesystems (e.g., NFS over bind-mount), coverage may regress. Mitigation: fail loudly if a build was expected but no event arrived within a sanity window — deferred until we actually see this.
- **[Example state drift clutters `git status`]** → Accepted trade-off (D6). Compensated by the reproducibility gain.
- **[Cross-example parallel runs compete for `bin/hams-linux-<arch>`]** → The watcher writes a single binary path per arch; two dev sessions on the same host with the same arch will overwrite each other. Trade-off accepted; a future iteration could scope output to `bin/dev/<example>/` if pain emerges.
- **[Developers forget to `docker rm -f hams-<example>` after SIGKILL]** → Self-healing via `docker stop ... || true` at start. Rare enough that automated GC is not worth the complexity.

## Migration Plan

No migration — this is net-new infrastructure. Existing `task test`, `task ci:e2e`, and `act`-based pipelines are untouched.

Rollout:

1. Land the `internal/devtools/watch/` module and unit-test its debounce/coalesce state machine.
2. Land `scripts/commands/dev/*.sh` and `examples/.template/`.
3. Wire `Taskfile.yml` with `dev` and `dev:shell` targets.
4. Ship `examples/basic-debian/` as the first real scenario + smoke test.
5. Update `cmd/hams/main.go` version format; verify release builds still populate ldflags correctly.

Rollback: revert the change. No state to clean up (examples fixtures can remain; tasks stop being referenced).

## Open Questions

- **Should the watcher also rebuild on `.mod`/`.sum` changes?** Current plan: yes (any file under `./cmd ./internal ./pkg` and any `go.mod`/`go.sum` at root). Confirm during tasks.md breakdown.
- **Should `task dev` auto-invoke `task dev:shell` in a second terminal?** Current plan: no — printing the attach hint is enough, and hijacking the user's shell choice is overreach. Revisit if onboarding feedback demands it.
- **Windows via WSL2** — not goal, but likely works accidentally. Do we claim support? Current answer: no, silently works is enough.

## Spec Dependency Graph

Single capability: `dev-sandbox`. No cross-capability dependencies — this change neither modifies nor depends on existing specs. The capability sits orthogonal to runtime behavior (it's pure developer tooling).

```
dev-sandbox (new)
  ├── consumes: existing `bin/hams` build conventions (CGO_ENABLED=0, ldflags)
  ├── consumes: existing hams config layout (HAMS_CONFIG_HOME, store_path, profile_tag)
  └── produces: no runtime behavior; tooling only
```
