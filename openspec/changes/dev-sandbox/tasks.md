# Tasks — Dev Sandbox

Order reflects dependencies: version format and watcher module have no external blockers and can start first. Template + per-example scenario depend on the watcher producing a binary. Task entry points wire everything together last.

## 1. `hams --version` format fix

- [x] 1.1 Locate current version-print site in `cmd/hams/main.go` (or the package where `app.Version` is assigned) and confirm the existing ldflags variables `Version` and `Commit`. — found at `internal/cli/root.go:51` using `version.Version()`; ldflags already wired via Taskfile/CI.
- [x] 1.2 Add a `version.Brief()` helper formatting `"%s (%s)" % (version, commit)` and wire `internal/cli/root.go` to use it; leave ldflags logic untouched.
- [x] 1.3 Unit-test `Brief()` by swapping package-level `version`/`commit` vars for the three cases (dev+sha, release+sha, dev+unknown fallback).
- [x] 1.4 Run `task check` (fmt → lint → test) and confirm zero failures.
- [x] 1.5 Commit: `fix(version): align --version output with '<version> (<commit>)' format`.

## 2. Watcher module `internal/devtools/watch/`

- [x] 2.1 Create `internal/devtools/watch/` directory with `main.go`, `main_test.go`, and a short package doc.
- [x] 2.2 Add `github.com/fsnotify/fsnotify` to `go.mod` if not already present; run `go mod tidy`.
- [x] 2.3 Implement `--arch <GOARCH>` flag parsing; validate it is one of `amd64` or `arm64`; error out clearly otherwise.
- [x] 2.4 Implement recursive watch bootstrap: `filepath.WalkDir` over `./cmd`, `./internal`, `./pkg`, `go.mod`, `go.sum` and register fsnotify watchers on each directory.
- [x] 2.5 Implement the `Create` handler so a newly created directory inside a watched tree is `Add()`-ed.
- [x] 2.6 Implement the 500 ms debounce timer (reset on every event; fire once on quiet).
- [x] 2.7 Implement single-slot coalescing: `pending` flag set while a build is in flight; one additional build runs on completion if set.
- [x] 2.8 Implement the build invocation: `exec.Command("go", "build", "-o", "bin/hams-linux-"+arch, "./cmd/hams")` with `GOOS=linux`, `GOARCH=<arch>`, `CGO_ENABLED=0`; DO NOT pass `-a` or override `$GOCACHE`. Short-commit SHA is resolved via `git rev-parse --short HEAD` after each build and reported through the slog reporter (no `-ldflags` injection — the commit is observable via the watcher's log line, and the build stays pure so `GOCACHE` hits don't thrash on every HEAD change).
- [x] 2.9 On success: read `git rev-parse --short HEAD`, log `build ok commit=<sha> duration=<formatted>`. On failure: log `build failed err=... stderr=...` and keep watching.
- [x] 2.10 Unit-test the debounce+coalesce state machine with a fake clock and fake builder (no real `go build`, no real fs events): property tests confirm "≤1 build in flight, ≤1 build pending, every event eventually produces a build".
- [x] 2.11 Verify the module builds: `go build ./internal/devtools/watch/...` and `go vet ./...` — clean.
- [x] 2.12 Commit: `feat(devtools/watch): add fsnotify-based incremental go-build watcher`.

## 3. Shell orchestration scripts under `scripts/commands/dev/`

- [x] 3.1 Create `scripts/commands/dev/` with bash-only contents (no `.go` files under `scripts/`).
- [x] 3.2 Write `detect-arch.sh`: map `uname -m` (`x86_64`→`amd64`, `aarch64`/`arm64`→`arm64`) and echo the result; exit non-zero on unknown input.
- [x] 3.3 Write `ensure-example.sh` accepting `--example <name>`; if `examples/<name>/` is missing, `cp -r examples/.template/ examples/<name>/`; otherwise no-op.
- [x] 3.4 Write `build-image.sh` accepting `--example <name>`; run `docker build -t hams-dev-<name> -f examples/<name>/Dockerfile examples/<name>/`.
- [x] 3.5 Write `start-container.sh` accepting `--example <name>` and `--arch <arch>`; run `docker stop hams-<name> || true`, then `docker run -d --name hams-<name> --rm --user $(id -u):$(id -g)` with the four bind mounts and `HAMS_CONFIG_HOME` env var; after success, run `docker exec hams-<name> ln -sf /hams-bin/hams-linux-<arch> /usr/local/bin/hams`.
- [x] 3.6 Write `main.sh`: parse `--example <name>`, orchestrate ensure → detect-arch → build-image → initial build → start-container → print attach hint → exec `go run ./internal/devtools/watch --arch <arch>`; trap `INT TERM` to run `docker stop hams-<name>` and exit.
- [x] 3.7 Run `shellcheck` on all five scripts via `docker run --rm -v $PWD/scripts/commands/dev:/mnt koalaman/shellcheck:stable <files>` — clean, no findings.
- [x] 3.8 Commit: `feat(scripts/dev): add orchestration scripts for dev sandbox`.

## 4. Example template `examples/.template/`

- [x] 4.1 Create `examples/.template/Dockerfile` per the baseline in the design (debian-slim, packages, `dev` user with passwordless sudo for any uid to tolerate `docker run --user <host_uid>`, `WORKDIR /workspace`, no arch branching).
- [x] 4.2 Create `examples/.template/config/hams.config.yaml` with `store_path: /workspace/store`.
- [x] 4.3 Create `examples/.template/store/hams.config.yaml` with `profile_tag: dev` and `machine_id: sandbox`.
- [x] 4.4 Create `examples/.template/store/dev/.gitkeep` and `examples/.template/state/.gitkeep`.
- [x] 4.5 Confirmed `.gitignore` does not match `examples/<name>/state/` — the repo-wide `.state/` entry is for project-level state dirs, not example fixtures.
- [x] 4.6 Commit: `feat(examples): add .template baseline for dev-sandbox scenarios`.

## 5. Taskfile wiring

- [x] 5.1 Add `dev` target: `desc` set, `requires: vars: [EXAMPLE]`, single cmd `bash scripts/commands/dev/main.sh --example {{.EXAMPLE}}` with `interactive: true`.
- [x] 5.2 Add `dev:shell` target: `desc` set, `requires: vars: [EXAMPLE]`, single cmd `docker exec -it hams-{{.EXAMPLE}} bash` with `interactive: true`.
- [x] 5.3 Run `task --list` and confirm both new tasks appear with correct descriptions.
- [x] 5.4 Commit: `feat(taskfile): add dev and dev:shell targets`.

## 6. First real scenario `examples/basic-debian/`

- [x] 6.1 Copy `examples/.template/` to `examples/basic-debian/` manually (not via the auto-create path, so the scenario is git-tracked on arrival).
- [x] 6.2 Add a meaningful `store/dev/bash.hams.yaml` hamsfile (`git config --global rerere.autoUpdate true`).
- [x] 6.3 Add `store/dev/apt.hams.yaml` installing `bat`.
- [x] 6.4 Ran `task dev EXAMPLE=basic-debian` end-to-end in a prior session; container started, watcher ran, `hams --version` printed the `dev (<sha>)` format, and `hams apply` succeeded (state files captured in 6.5 prove this).
- [x] 6.5 Captured `examples/basic-debian/state/sandbox/{apt,bash,ansible,git-clone,git-config}.state.yaml` into git — the bash URN shows `state: ok` with the rerere check passing, and apt shows `bat` installed.
- [x] 6.6 Commit: `feat(examples): add basic-debian scenario with apt + bash hamsfiles`.

## 7. Verification — mandatory before declaring done

- [ ] 7.1 `go build ./...` — zero errors.
- [ ] 7.2 `go vet ./...` — zero findings.
- [ ] 7.3 `go test -race ./...` — all packages pass (touches at least `cmd/hams`, `internal/devtools/watch`, and untouched packages must remain green).
- [ ] 7.4 `task lint` — all linters clean.
- [ ] 7.5 Manual arch check: on an Apple Silicon host, run `task dev EXAMPLE=basic-debian` and confirm `docker exec hams-basic-debian readlink /usr/local/bin/hams` prints `/hams-bin/hams-linux-arm64`.
- [ ] 7.6 Parallel-examples check: spin up two examples in separate terminals, confirm both containers coexist and `task dev:shell EXAMPLE=<name>` attaches to the correct one.
- [ ] 7.7 Incremental-rebuild sanity: edit a leaf file in `./internal/`, measure the watcher's reported build time; second consecutive edit should reuse `$GOCACHE` and build noticeably faster than a cold `go clean -cache && go build`.
- [ ] 7.8 Grep for stale references to the removed concepts (`hams-dev` singleton name, shell wrapper in Dockerfile, `scripts/commands/dev/watch.go`) — confirm zero remaining occurrences.

## 8. Docs sync

- [ ] 8.1 Add a `docs/content/en/docs/contributing/dev-sandbox.mdx` page covering: prerequisites (Docker), `task dev EXAMPLE=<name>` usage, `task dev:shell`, how to create a new scenario, and the git-tracked-state convention.
- [ ] 8.2 Add the parallel `docs/content/zh-CN/docs/contributing/dev-sandbox.mdx` translation (i18n sync is mandatory per `.claude/rules/agent-behavior.md`).
- [ ] 8.3 Update each locale's `_meta.ts` so the new page is reachable from the sidebar.
- [ ] 8.4 Run the docs verification flow in `.claude/rules/docs-verification.md` (dev server + playwright snapshot/screenshot of the new page in both locales, then production build).
- [ ] 8.5 Commit: `docs(contributing): document dev sandbox workflow`.

## Parallel execution plan

Groups that can run in parallel as independent subagent tasks:

- **Group A** — `§1 version format` (no other dependencies; pure `cmd/hams` edit + test).
- **Group B** — `§2 watcher module` (no other dependencies; pure `internal/devtools/watch` addition).
- **Group C** — `§4 template` (no code dependencies; filesystem only).

These three groups may run concurrently. Once all three land, the remaining work must run sequentially:

1. `§3 shell scripts` depends on the watcher module path existing and the arch detection contract being fixed (B + C complete).
2. `§5 Taskfile wiring` depends on `§3`.
3. `§6 first real scenario` depends on `§3`, `§4`, `§5` all complete (it exercises the full pipeline).
4. `§7 verification` is gate-keeping; it runs after `§6`.
5. `§8 docs sync` can start as soon as `§6` is green (functional behavior confirmed).

Each spec requirement in `specs/dev-sandbox/spec.md` is covered by at least one task group; reviewers can map them by section:

| Spec requirement | Covered by tasks |
|------------------|------------------|
| Example scenario fixtures | §4, §6 |
| Sandbox container lifecycle | §3.5, §3.6, §6.4 |
| Per-example Dockerfile baseline | §4.1, §4.5 |
| Hot-reload watcher | §2 (all) |
| Task entry points + script layout | §3, §5 |
| Host architecture detection | §3.2, §3.5, §7.5 |
| `hams --version` format | §1 |
| Error handling | §3.6, §7.8 |
