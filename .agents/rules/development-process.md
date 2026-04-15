---
description: Core development process principles for hams project
globs: ["**/*"]
---

# Development Process Principles

- **CLAUDE.md is a map, not an encyclopedia**: keep it under 200 lines, pointing to `docs/`, `openspec/`, `.claude/rules/` for depth. Each layer exposes only its own information plus navigation to the next level.

- **The repo is the single system of record**: knowledge not in the repo does not exist for agents. Discussions, mental decisions, external documents — if they affect development, they MUST land as a versioned artifact inside the repo.

- **Plans are first-class artifacts**: execution plans with progress logs are versioned and centralized in `openspec/changes/<id>/tasks.md`. Complex task breakdowns go to `openspec/changes/<id>/tasks/<capability>.task.md`. Tasks can be refined and decomposed during execution — they are not fixed upfront.

- **Encode taste as rules**: prefer linters, structural tests, and CI checks over natural-language instructions. Mechanically verifiable > prose guidelines.

- **Continuous garbage collection**: pay down tech debt in small, continuous increments — never accumulate for a large cleanup. Track gaps in openspec tasks.

- **When stuck, fix the environment, not the effort**: when an agent hits difficulty, ask "what context, tool, or constraint is missing?" and record the answer in `.claude/rules/`.

- **Default language is en-US**: all files use English unless the filename contains a locale suffix (e.g., `*.zh-CN.*` like `README.zh-CN.md`).

- **Universal secret decoupling**: all token/key/credential values MUST be stored in OS keychain (via keyring, `kind: application password`) or in `*.local.*` config files (which are gitignored). Secret values SHALL never appear in git-tracked config files. This applies to notification tokens (Bark, Discord), LLM API keys, and any future integration credentials.

- **Build outputs go to `./bin/` only**: all `go build` commands MUST output to the `bin/` directory (e.g., `-o bin/hams`). Never output binaries to the project root or any other location. This applies to Taskfile tasks, CI workflows, release scripts, and any ad-hoc build commands. The `bin/` directory is `.gitignore`'d.

- **Frequent atomic commits**: during implementation, commit after each independent task/feature is complete. Every commit should be a coherent, revertable unit. Never batch unrelated changes into one commit.

- **TDD with real-environment safety**: always write unit tests and E2E tests alongside implementation, not after.
  - Providers that interact with real package managers (brew, pnpm, apt, etc.) MUST be tested inside Docker containers to avoid corrupting the host machine.
  - When Docker infrastructure is not yet ready, use these **safe local test packages** for manual verification:
    - `brew`: `htop`
    - `pnpm`: `serve`
    - `bash`: `git config --global rerere.autoUpdate true`
  - Prioritize implementing the smallest verifiable slice first.

- **Verification is the most important process** in hams development. It has three tiers — these are the only test dimensions hams recognizes; pick the one that matches scope and DI surface:
  1. **Code standards** (lint): `golangci-lint`, `eslint`, `markdownlint`, `cspell` — enforced by lefthook pre-commit and CI.
  2. **Unit tests** (`go test`):
     - **Scope:** the hams CLI dispatcher itself + each Provider in isolation.
     - **Definition:** runs directly in Go on the host. MUST NOT mutate any real file outside `t.TempDir()` and MUST NOT exec any real package-manager command. Every file-IO and exec boundary MUST be reachable via a constructor-injected interface so the test can substitute a fake.
     - **Consequence:** the hams CLI and every provider MUST be domain-layered with DI. A provider that wraps `apt-get`/`brew`/`pnpm`/`git`/etc. exposes its outbound calls as a Go interface; the unit test wires a fake that records calls and writes virtual files. The fake is swapped only in the test — it does not affect production execution and does not affect E2E tests. That is the entire point of dependency injection here.
  3. **Integration tests** (`task ci:itest` / per-provider Docker scenarios):
     - **Scope:** the hams CLI + a single Provider, against real filesystem/network/package-manager state.
     - **Definition:** each test case runs inside a dedicated Docker container per provider, isolated from every other provider. Each provider owns its integration test under `internal/provider/builtin/<provider>/integration/` with its own `Dockerfile` and `integration.sh`. The shared base image `hams-itest-base` (at `e2e/base/Dockerfile`) carries only debian-slim + ca-certs, curl, bash, git, sudo, yq — NO language toolchains. Each provider overlay installs its own runtime (node, python, go, rust, brew, …) because that is precisely what hams itself must do for real users. The real `hams` binary is bind-mounted read-only; integration.sh exercises real config read/write, real package install/remove, and real state-file writes. Both the base image and each provider overlay are content-addressed by SHA of their Dockerfile so rebuilds only happen when the underlying file changes. Tests assert observable outcomes (binary on PATH, YAML field values, exit codes) — never DI seams.
  4. **E2E tests** (`task ci:e2e` / `task ci:e2e:one TARGET=<distro>`):
     - **Scope:** the hams CLI + every Provider relevant to the target OS, end-to-end across init → install → uninstall → restore-from-store flows.
     - **Definition:** runs the CI workflow (`.github/workflows/ci.yml`) via [`act`](https://github.com/nektos/act) against a matrix of OS images (Debian/Alpine/OpenWrt) × CPU architectures (amd64/arm64). Tests live in `e2e/<target>/run-tests.sh` and consume the shared assertion helpers in `e2e/base/lib/`.

- **Standardized provider integration test process** — every linux-containerizable Provider MUST ship `internal/provider/builtin/<provider>/integration/{Dockerfile, integration.sh}`:
  1. **Dockerfile** — `ARG BASE=hams-itest-base:latest` + `FROM ${BASE}` + whatever runtime install the provider needs (node/python/go/rust/brew/…). Runtime install lives here, not in integration.sh, so docker layer caching makes repeated test runs cheap.
  2. **integration.sh** — sourced helpers at `/e2e/base/lib/{assertions,yaml_assert,provider_flow}.sh`. Required steps for package-like providers (apt, brew, cargo, goinstall, npm, pnpm, uv, vscodeext):
     - Set `HAMS_STORE`, `HAMS_MACHINE_ID`, `HAMS_CONFIG_HOME` per the integration-test sandbox.
     - Smoke: `assert_output_contains "hams --version" "hams version" hams --version`.
     - Invoke `standard_cli_flow <provider> <install_verb> <existing_pkg> <new_pkg>` — the helper walks the canonical lifecycle (seed install → re-install → install-new → refresh → remove) and fails fast on the first bad assertion. For providers without a PATH binary (bash, git-config, git-clone), define a shell function and export `POST_INSTALL_CHECK=<fn-name>` before calling the helper.
  3. **CI**: GitHub Actions `itest` matrix (`.github/workflows/ci.yml`) runs one container per provider via `task ci:itest:run PROVIDER=<name>`. `fail-fast: false` so one flaky provider doesn't mask the others.
  4. **Local**: `task ci:itest:run PROVIDER=<name>` (direct docker, fast iteration) or `task test:itest:one PROVIDER=<name>` (through `act`, full CI simulation). `task ci:itest` runs all in-scope providers sequentially.
  5. **Cache**: both the base image and the provider overlay are tagged with the sha256 of their Dockerfile (first 12 chars). `docker image inspect` gates the rebuild — tests don't recompile images unless the Dockerfile itself changed.
  Reference implementations:
  - `internal/provider/builtin/apt/integration/integration.sh` — package-like flow via `standard_cli_flow`.
  - `internal/provider/builtin/bash/integration/integration.sh` — declarative flow with `POST_INSTALL_CHECK` hook (custom check).
  - `internal/provider/builtin/git/integration/integration.sh` — two providers in one container (git-config + git-clone) with custom assertions.

- **DI boundary isolation principle**: code architecture MUST use dependency injection to isolate uncontrollable external boundaries (filesystem, network, package managers, OS APIs). Unit tests inject mock boundary-layer services and run without side effects on the host.

- **Local/CI isomorphism**: code style checks (golangci-lint), unit tests, and Docker-based E2E tests MUST all run identically on a developer's local machine and in GitHub Actions CI. No CI-only or local-only test paths. Use the same commands (Taskfile tasks) in both environments. Use [nektos/act](https://github.com/nektos/act) to execute `.github/workflows/` locally for isomorphic validation.

- **GitHub Actions invoke Taskfile tasks, never raw commands**: every `.github/workflows/*.yml` step that performs build, test, lint, or any other project action MUST go through [`go-task/setup-task`](https://github.com/go-task/setup-task) + a `task <name>` invocation. Never inline raw `go build`, `go test`, `golangci-lint run`, `docker build`, or equivalent shell commands in workflow steps. Reason: Taskfile is the single source of truth for how work is done; inlining commands in workflows creates silent drift between local `task <name>` and CI, breaking the Local/CI isomorphism guarantee. If a workflow needs a new capability, add a Taskfile task first, then call it from the workflow.

- **Integration & E2E tests — Taskfile `ci:*` tasks are the contract**: the Taskfile tasks `ci:itest:base`, `ci:itest:run PROVIDER=<name>`, `ci:itest`, `ci:e2e:run TARGET=<target>`, and `ci:integration:run` are the single source of truth for how integration/E2E tests execute. CI workflows (`.github/workflows/ci.yml`) invoke these tasks verbatim via `task <name>`; local developers call the same tasks for byte-for-byte parity. `act` (`task test:itest:one PROVIDER=<name>`, `task test:e2e:one TARGET=<target>`) simulates the full GitHub Actions runner locally when that parity guarantee needs extra scrutiny. Individual provider unit tests (non-Docker, DI-isolated) are the exception and run via `go test` directly.

- **Review findings are tasks**: when a Codex Review (`/codex:review`) or OpenSpec Verify (`/opsx:verify`) produces findings, record each finding as a checklist item under `openspec/changes/<id>/tasks/<capability>.task.md`, link from `tasks.md`, then for each finding use `/codex:rescue` to discuss context and fix strategy with Codex before implementing. Each fix gets its own atomic commit.

- **Prefer community packages over NIH**: before implementing any functionality, search for well-maintained open-source packages in the Go ecosystem that solve the same problem. Use established libraries instead of writing from scratch. Examples: prefer `bitfield/script` for shell execution, `charmbracelet/bubbletea` for TUI, `go-yaml/yaml` for YAML parsing, etc.

- **Test repos for E2E**: `--from-repo` must accept both remote GitHub URLs and local `.git` repo paths (local paths resolved first). For unit/E2E tests, prepare a stable `.git` repo in `.gitignore`'d test fixtures via bash scripts. Available test repos:
  - Local: `~/Project/Homelab/test-store.hams`
  - Remote: `https://github.com/zthxxx/test-store.hams.git`
