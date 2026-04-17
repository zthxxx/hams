---
title: hams Testing Architecture Analysis
date: 2026-04-18
status: analysis-snapshot
---

# hams Testing Architecture — Full Analysis

This note aggregates the findings of three parallel research agents that audited the three runtime test tiers of the `hams` project. The goal is to answer, with file-path citations, the question: **what does every test in this project actually verify?**

The project's `CLAUDE.md` defines **four recognized test tiers**:

1. **Code standards** (lint) — `golangci-lint v2`, `eslint`, `markdownlint-cli2`, `cspell`, enforced by `lefthook` pre-commit and CI.
2. **Unit tests** (`go test`) — DI-isolated, pure-memory, zero side effects on the host.
3. **Integration tests** — one Docker container per provider, real package managers, real filesystem, hams binary bind-mounted read-only.
4. **E2E tests** — the full CI workflow (`.github/workflows/ci.yml`) executed via `act` against an OS matrix (Debian / Alpine / OpenWrt × amd64 / arm64 — intended; amd64 only in practice).

The First Principle is **Isolated Verification**: nothing is allowed to mutate the host during development or testing. All side-effecting verification is pushed inside throwaway containers, and all in-process verification is pushed behind dependency-injected boundaries.

---

## Tier 1 — Lint / Code Standards

Not behavioral verification, but listed as part of the pyramid. Enforced uniformly on developer machines (via `lefthook`) and in CI, using the **same Taskfile entry points** so that local and CI results match byte-for-byte.

| Tool | Purpose | Config |
|------|---------|--------|
| `golangci-lint v2` | 30+ Go linters, strict | `.golangci.yml` |
| `eslint 9` | JS/TS (docs site, scripts) | `eslint.config.ts` |
| `markdownlint-cli2` | Markdown | `.markdownlint.yaml` |
| `cspell` | Spell check | `cspell.yaml` |
| `lefthook` | pre-commit + pre-push hooks | `lefthook.yml` |

---

## Tier 2 — Go Unit Tests (`*_test.go`)

**Total inventory: 124 `*_test.go` files** (excluding `e2e/` and per-provider `integration/` directories).

### Contract

From `.claude/rules/code-conventions.md` and `development-process.md`:

- A unit test **must not** modify any file outside `t.TempDir()`.
- A unit test **must not** exec any real package-manager command.
- Every external boundary (fs / exec / network) exposed by a provider must be reachable through a constructor-injected Go interface so the test can substitute a fake.
- **Property-based testing (via `rapid`)** is preferred over example-based tests for invariant validation.

### Coverage Map by Package

| Layer | Representative package(s) | What it tests |
|-------|--------------------------|---------------|
| **CLI routing** | `internal/cli/` (15 files) | apply / bootstrap / filter / flags / store subcommand behaviour; both JSON and text output modes; stdout vs. stderr routing (`apply_test.go`, `bootstrap_consent_test.go`, `utils_test.go`) |
| **Provider core** | `internal/provider/` (18 files) | DAG topology & cycle detection (`dag_test.go`); Plan computation (`plan_test.go`); Executor dispatch (`executor_test.go`); Hook parsing & execution pipeline (`hooks_integration_test.go`, `hooks_parse_test.go`); rapid invariants (`property_test.go`) |
| **15 builtin providers** | `internal/provider/builtin/<p>/` (75+ files) | Per provider: a standard trio — `*_test.go` (basic API), `*_lifecycle_test.go` (install → refresh → remove state machine), `*_property_test.go` (rapid invariants + no-panic fuzz) |
| **Config & state** | `internal/config/`, `internal/state/`, `internal/hamsfile/` | YAML merge, state serialization, single-writer lock, explicit-`--config`-path validation |
| **Helper capabilities** | `internal/llm/`, `internal/notify/`, `internal/error/`, `internal/selfupdate/`, `internal/tui/` | LLM prompt/response parsing, Bark/Discord notification, `UserFacingError` code mapping, self-upgrade flow, BubbleTea TUI models (scaffolded but unwired in v1) |
| **Dev tooling** | `internal/devtools/watch/` | Dev-watcher (`fswatch`, engine, builder) |
| **SDK** | `pkg/sdk/` | External plugin-author-facing gRPC provider abstraction |

### DI Isolation — Concrete Evidence

1. **Exec boundary.** Every provider exports a `CmdRunner` interface and a `FakeCmdRunner`. For Homebrew (`internal/provider/builtin/homebrew/command_fake.go`), formulae / casks / taps are **in-memory maps** replacing real `brew` calls. Tests seed initial state via `SeedFormula` / `SeedCask` and may inject failures via `installErrors`. Usage is a one-liner: `p := New(cfg, NewFakeCmdRunner())`.
2. **Filesystem boundary.** All store, config, and state files are routed through `t.TempDir()`. There is zero risk of polluting `~/.config/hams` or `~/.local/share/hams` from a unit test.
3. **Network boundary.** LLM tests mock prompt/response strings inline; `internal/selfupdate/` uses `httptest.Server` for the GitHub Releases flow; go-git clone never enters the unit-test layer.
4. **Hook execution — sandboxed not mocked.** `internal/provider/hooks_integration_test.go` deliberately lets the `touch $marker` command actually run, but `$marker` lives in `t.TempDir()`. This verifies the Hook → runner → shell pipeline end-to-end without depending on any host state.
5. **CLI output capture.** `internal/cli/utils_test.go` wraps `captureStderr` around `captureStdout` using `bytes.Buffer`. This is how recent Ralph Loop cycles (e.g. cycle 250–253) verified that clone progress, interactive prompts, and JSON summaries each route to the correct stream.

### Naming & Organization Conventions

- Test names follow `Test<Subject>_<Scenario>[_<Expected>]`, e.g. `TestInjectFlags_SkipsExisting`, `TestRunApply_DryRunJSONHasNoProse`.
- Subtests via `t.Run(...)` are used broadly, but not universally — simple property tests just use top-level functions.
- **No global `testutil` package.** Fakes and stubs are defined inside the package under test: `FakeCmdRunner` in `command_fake.go`, `execStubProvider` / `hookStubProvider` inline inside their respective `_test.go` files.
- **Rapid property tests are pervasive.** Each of the 15 builtin providers has a `*_property_test.go`; provider core has `property_test.go` (DAG acyclicity); state has property-driven cases. Covered invariants include: no-panic under fuzz, idempotency, topological-order stability, set-semantic dedup.

### Coverage Status (from `CLAUDE.md` cycles 243–255)

| Package | Before | After |
|--------|-------|-------|
| `internal/cli` | 37% | 44% |
| `internal/config` | 74% | 77% |
| `internal/error` | 36% | 100% |
| `internal/llm` | 30% | 81% |
| `internal/provider/builtin/bash` | 51% | 86% |
| `internal/provider/builtin/homebrew` | 45% | 49% |
| `internal/provider/builtin/ansible` | 18% | 77% |
| `internal/provider/builtin/defaults` | 20% | 60% |

There is no project-wide coverage floor; coverage is raised one weak package per Ralph Loop cycle.

---

## Tier 3 — Provider Integration Tests (Docker-in-Linux)

### Contract

Each Linux-containerizable provider ships its own `internal/provider/builtin/<provider>/integration/{Dockerfile, integration.sh}`. The `hams` binary enters the container **read-only** via bind mount. The container exercises real `apt-get install`, `brew install`, `pnpm add`, `cargo install`, etc., but is fully disposable — the host is never touched.

### Coverage Matrix

Of the 15 builtin providers, **12** have integration tests, **3** are macOS-only and deliberately excluded.

| Provider | Has `integration/` | Dockerfile runtime | `integration.sh` strategy |
|---------|:------------------:|-------------------|---------------------------|
| apt | yes | debian-slim (apt preinstalled) | `standard_cli_flow apt install jq btop` + extra `htop` scenarios |
| bash | yes | no extra runtime | Declarative URN + custom `POST_INSTALL_CHECK`; hand-rolled hamsfile + apply |
| cargo | yes | `rust:1-slim-bookworm` | `standard_cli_flow cargo install tokei just` |
| git (config + clone) | yes | no extra runtime | Two sub-flows in one container: `git-config` global key/value + `git-clone` repo clone |
| goinstall | yes | Go toolchain | `standard_cli_flow goinstall install github.com/rakyll/hey@latest ...`; custom `POST_INSTALL_CHECK` parses module → binary name |
| homebrew | yes | Linuxbrew + non-root `brew` user | `sudo -u brew` wrapper + `BREW_RUN` indirection into `standard_cli_flow` |
| npm | yes | Node.js + npm | `standard_cli_flow npm install serve nodemon` |
| pnpm | yes | pnpm + Node | `standard_cli_flow pnpm add serve nodemon` |
| uv | yes | Python + uv | `standard_cli_flow uv install ruff httpie` |
| vscodeext | yes | VS Code CLI | `standard_cli_flow code-ext install <ext-id> ...`; custom `POST_INSTALL_CHECK` via `code --list-extensions`; `STATE_FILE_PREFIX=vscodeext` |
| ansible | yes | ansible-playbook | One-shot `hams ansible <playbook>` + declarative `ansible.hams.yaml`; marker-file assertions instead of `standard_cli_flow` |
| defaults | **no** | — | macOS-only (`defaults write` / plists) |
| duti | **no** | — | macOS-only (file-association manager) |
| mas | **no** | — | macOS-only (Mac App Store CLI) |

Because `git-config` and `git-clone` share one container, the CI `itest` matrix lists **11 jobs**, not 12.

### The Standard 5-Step Lifecycle

Most providers delegate to `standard_cli_flow <provider> <verb> <existing_pkg> <new_pkg>` defined in `e2e/base/lib/provider_flow.sh`:

1. **Seed install.** Write `<p>.hams.yaml` → `hams <p> <verb> <existing_pkg>` → `hams apply` → assert `state=ok`, capture `first_install_at`.
2. **Re-install.** Install the same package again → assert `first_install_at` is unchanged while `updated_at` is lexically greater.
3. **Install new.** Pre-check: `POST_INSTALL_CHECK $new_pkg` must return non-zero (not installed). Then install. Post-check: must return zero. Assert `state=ok`, `first_install_at` set, `removed_at` absent.
4. **Refresh.** `hams refresh --only=<p>` → assert `updated_at` advanced again.
5. **Remove.** Delete the `new_pkg` entry from the hamsfile → apply → assert `state=removed`, `removed_at` populated.

Providers without a PATH binary (bash, git-config, git-clone) export a shell function and `POST_INSTALL_CHECK=<fn>` before calling the helper, replacing the default `command -v`.

### Shared Shell Libraries (`e2e/base/lib/`)

- **`assertions.sh`** — `assert_success`, `assert_output_contains`, `run_smoke_tests` (CLI `--version` / `--help` / per-provider help), `create_store_repo`, plus provider-specific helpers (`verify_bash_marker`, `verify_git_config`, `verify_config_roundtrip`, `verify_idempotent_reapply`).
- **`yaml_assert.sh`** — `yq`-powered `get_yaml_field`, `assert_yaml_field_eq`, `assert_yaml_field_present` / `_absent`, and `assert_yaml_field_lex_gt` (lexical comparison, used for verifying monotonic timestamps).
- **`provider_flow.sh`** — the orchestrator for the 5-step lifecycle. Environment contract: `HAMS_STORE`, `HAMS_MACHINE_ID` (required); `STATE_FILE_PREFIX` and `POST_INSTALL_CHECK` (optional).

### Image Caching

- Base image `hams-itest-base:<sha12>` is tagged by the 12-character SHA of `e2e/base/Dockerfile`.
- Per-provider image `hams-itest-<provider>:<sha12>` is tagged by the combined SHA of base + provider Dockerfile, so a base change invalidates all downstream images.
- Rebuild is gated by `docker image inspect`; stale images are garbage-collected via `grep + xargs docker rmi`.

### Execution Entry Points

- **`task ci:itest:base`** — build the shared base image when its Dockerfile changed.
- **`task ci:itest:run PROVIDER=<name>`** — build the provider overlay (if needed), bind-mount `bin/hams-linux-amd64` → `/usr/local/bin/hams:ro`, bind-mount `e2e/base/lib/` → `/e2e/base/lib:ro`, bind-mount the provider's `integration/` → `/integration:ro`, then run `bash /integration/integration.sh`.
- **`task ci:itest`** — iterate all in-scope providers sequentially with `fail-fast: false`.
- **`task test:itest:one PROVIDER=<name>`** — same flow but driven through `act` for byte-identical CI simulation.

### CI Integration

`.github/workflows/ci.yml` declares an `itest` job with `needs: [build]`, `strategy.fail-fast: false`, and `matrix.provider: [apt, ansible, bash, cargo, git, goinstall, homebrew, npm, pnpm, uv, vscodeext]`. Each step is a `go-task/setup-task` + `task ci:itest:run` invocation — **never a raw shell command** (per the repo rule "GitHub Actions invoke Taskfile tasks, never raw commands").

---

## Tier 4 — End-to-End Tests

### Contract

An E2E run exercises **the hams CLI plus every provider relevant to the target OS**, end-to-end across `init → install → uninstall → restore-from-store`. The reference scenario is "a brand-new machine bootstraps itself via `hams apply --from-repo=<X>`".

### Target Matrix

| Target | Base image | Providers exercised | Extra assertions |
|--------|-----------|---------------------|------------------|
| **debian** | `debian:bookworm-slim` | apt + bash + git-config | Store-scope rejection test: writing a machine-scoped field to store config must fail and point the user to the global config |
| **alpine** | `alpine:3.20` | bash + git-config | — |
| **openwrt** | `alpine:3.20` (proxy) | bash + git-config | — |

The OpenWrt target uses Alpine 3.20 as a proxy because the real OpenWrt container image is too minimal (`opkg` unavailable in userspace).

### E2E Walkthrough (Debian, `e2e/debian/run-tests.sh:1-52`)

1. Load shared assertion library and Debian-specific scope-rejection helper.
2. Run smoke tests: CLI `--version` / `--help` / per-provider help.
3. `create_store_repo /tmp/test-hams-store $FIXTURE $MACHINE_ID` — initialize a real git repo from `e2e/fixtures/debian-store/`.
4. `apt-get update` to seed the package index.
5. `hams apply --from-repo=$STORE_DIR --only=apt,bash,git-config`.
6. Assert `jq --version` succeeds (apt installed jq).
7. Assert `/tmp/hams-e2e-marker` exists (bash provider fired its setup script).
8. Assert `git config --global --get e2e.hams.test == true` (git-config provider took effect).
9. Idempotency check: re-run apply → no side effects.
10. Config round-trip: `hams config set` / `get` / restore the original value.
11. `hams list --only=apt,bash,git-config` output format check.
12. **Scope-rejection test** — inject `profile_tag` (a machine-scoped field) into store config; hams must fail and the error must direct the user to the global config.

Alpine and OpenWrt follow steps 1–11 with the apt and scope-rejection steps removed.

### act + CI Execution Paths

- **Full local run.** `task test:e2e` → `act push --container-architecture linux/amd64 -j build -j e2e`. `act` spins up a runner container and replays `.github/workflows/ci.yml` verbatim.
- **Single target local.** `task test:e2e:one TARGET=<debian|alpine|openwrt>` → adds `--matrix target:<name>`.
- **CI** (`.github/workflows/ci.yml:141-168`) — the `e2e` job has `needs: [build]` and `matrix.target: [debian, alpine, openwrt]`; each step is a Taskfile invocation.
- **Direct-docker fast path.** `task ci:e2e:run TARGET=<target>` (`Taskfile.yml:234-255`) skips `act` and runs docker directly, still honoring the same SHA-12 content-addressed image cache.

### Store Fixtures

- `e2e/fixtures/{debian,alpine,openwrt}-store/` — each is a full `hams.config.yaml` + `test/<p>.hams.yaml` tree. `create_store_repo` copies it into the container, runs `git init && git commit`, and writes a global config that points at it.
- `github.com/zthxxx/test-store.hams` — the public "what a real user's store looks like" example, cited in docs but **not** required by any E2E run.
- `~/Project/Homelab/test-store.hams` — a local developer fixture (optional).

### Blind Spots vs. Declared Goals

| Dimension | `CLAUDE.md` goal | Reality | Gap |
|-----------|------------------|---------|-----|
| OS | Debian / Alpine / OpenWrt | 3 targets present | OpenWrt is Alpine-backed |
| CPU | amd64 + arm64 | amd64 only | **arm64 entirely missing** (`act --container-architecture linux/amd64` is hard-coded) |
| macOS | Not in E2E by design | N/A | macOS relies on unit tests + manual testing |

No explicit `TODO: arm64` markers exist, but the Taskfile matrix structure is designed to accept additional targets/arches.

---

## Tier Boundaries — What Each Layer Is Responsible For

| Dimension | Unit (Tier 2) | Integration (Tier 3) | E2E (Tier 4) |
|-----------|---------------|-----------------------|---------------|
| **Scope** | CLI dispatcher + one provider's pure logic | hams CLI + **one** provider's real lifecycle | hams CLI + **many** providers across an OS |
| **Runtime** | Host Go runtime | Docker container (debian-slim base + overlay) | Docker container (per-OS base) |
| **Side effects** | Zero (all DI) | Real inside container, zero on host | Real inside container, zero on host |
| **Speed** | Seconds, parallel | ~10–60 s per provider | ~1–3 min per target |
| **What it verifies** | Plan / DAG / Executor / CmdRunner interface behaviour, state serialization, error codes | Full 5-step lifecycle per provider, exact state-file field values | `--from-repo` bootstrap, cross-provider cooperation, idempotency, config-scope isolation |
| **What it does NOT test** | Real command behaviour, real disk-write races | Cross-provider orchestration, OS diversity | Single-function branches, rapid property invariants |

**One-line definition of each layer:**

- Unit tests prove that internal contracts do not drift.
- Integration tests prove that a single provider runs its full lifecycle in a real container.
- E2E tests prove that one `hams apply --from-repo=<X>` command turns a clean container into the desired environment.

None of the three touches the host. The First Principle — **Isolated Verification** — is enforced by two physical mechanisms: DI boundary isolation (Tier 2) and container isolation (Tiers 3 and 4).

---

## Key File Path References

For navigation:

- Test rules — `.claude/rules/development-process.md`, `.claude/rules/code-conventions.md`
- CI workflow — `.github/workflows/ci.yml`
- Taskfile — `Taskfile.yml` (search `ci:itest:*`, `ci:e2e:*`, `test:e2e:*`)
- Shared shell libs — `e2e/base/lib/{assertions,yaml_assert,provider_flow}.sh`
- Integration base image — `e2e/base/Dockerfile`
- E2E target roots — `e2e/{debian,alpine,openwrt}/`
- Per-provider integration — `internal/provider/builtin/<provider>/integration/`
- Representative unit-test fakes — `internal/provider/builtin/homebrew/command_fake.go`, `internal/provider/hooks_integration_test.go`

## Source Agents

This note was produced by dispatching three parallel research agents (via the `superpowers:dispatching-parallel-agents` workflow) against the three runtime test tiers. Each agent returned a focused report; this document is the merged, cross-verified synthesis.

- Agent A — Go unit tests (`*_test.go`, 124 files).
- Agent B — per-provider Docker integration tests (`internal/provider/builtin/*/integration/`).
- Agent C — OS-level E2E tests (`e2e/`, `.github/workflows/ci.yml` E2E job).
