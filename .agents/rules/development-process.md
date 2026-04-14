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
    - `brew`: `bat`
    - `pnpm`: `serve`
    - `bash`: `git config --global rerere.autoUpdate true`
  - Prioritize implementing the smallest verifiable slice first.

- **Verification is the most important process** in hams development. It has three tiers:
  1. **Code standards** (lint): `golangci-lint`, `eslint`, `markdownlint`, `cspell` — enforced by lefthook pre-commit and CI.
  2. **Unit tests**: DI-isolated, no real filesystem or host environment impact. Pure parsing + mock tests. Commands that need real external execution (`hams config set`, `hams apply --from-repo`, `self-upgrade`) MUST design unit tests that run in isolated Docker containers.
  3. **Integration E2E tests**: isomorphic with `.github/workflows/` CI. Use Dockerfiles to test across CPU platforms (amd64/arm64) and systems (Debian/Alpine/macOS). Dev machines have Docker installed; target runtime environments do NOT.

- **DI boundary isolation principle**: code architecture MUST use dependency injection to isolate uncontrollable external boundaries (filesystem, network, package managers, OS APIs). Unit tests inject mock boundary-layer services and run without side effects on the host.

- **Local/CI isomorphism**: code style checks (golangci-lint), unit tests, and Docker-based E2E tests MUST all run identically on a developer's local machine and in GitHub Actions CI. No CI-only or local-only test paths. Use the same commands (Taskfile tasks) in both environments. Use [nektos/act](https://github.com/nektos/act) to execute `.github/workflows/` locally for isomorphic validation.

- **Integration & E2E tests run exclusively through `act`**: all integration and E2E tests MUST execute via `act` against `.github/workflows/ci.yml` — never by invoking `docker build`/`docker run` directly in Taskfile or scripts. This ensures the local execution path is byte-for-byte identical to the CI pipeline. Individual provider unit tests (non-Docker, DI-isolated) are the exception and run via `go test` directly. Taskfile entry points: `task ci:e2e` (all targets), `task ci:e2e:one TARGET=<target>` (single target), `task ci:integration`.

- **Review findings are tasks**: when a Codex Review (`/codex:review`) or OpenSpec Verify (`/opsx:verify`) produces findings, record each finding as a checklist item under `openspec/changes/<id>/tasks/<capability>.task.md`, link from `tasks.md`, then for each finding use `/codex:rescue` to discuss context and fix strategy with Codex before implementing. Each fix gets its own atomic commit.

- **Prefer community packages over NIH**: before implementing any functionality, search for well-maintained open-source packages in the Go ecosystem that solve the same problem. Use established libraries instead of writing from scratch. Examples: prefer `bitfield/script` for shell execution, `charmbracelet/bubbletea` for TUI, `go-yaml/yaml` for YAML parsing, etc.

- **Test repos for E2E**: `--from-repo` must accept both remote GitHub URLs and local `.git` repo paths (local paths resolved first). For unit/E2E tests, prepare a stable `.git` repo in `.gitignore`'d test fixtures via bash scripts. Available test repos:
  - Local: `~/Project/Homelab/test-store.hams`
  - Remote: `https://github.com/zthxxx/test-store.hams.git`
