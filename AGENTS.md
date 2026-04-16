# CLAUDE.md

Guidance for Claude Code agents working in this repository.

## Project Overview

name: **hams** (hamster) — declarative IaC environment management for macOS/Linux workstations.
Wraps existing package managers (Homebrew, pnpm, npm, apt, etc.) to auto-record installations
into YAML config files ("Hamsfiles"), enabling one-command environment restoration on new machines.

| Key | Value |
|-----|-------|
| Go module | `github.com/zthxxx/hams` |
| Go version | 1.24 |
| DI framework | [Uber Fx](https://github.com/uber-go/fx) |
| Entry point | `cmd/hams/main.go` |
| JS runtime | Bun (not Node.js) |
| JS package manager | pnpm (`saveExact: true`) |
| npm package name | `harms` |

## First Principle: Isolated Verification

hams is a real-world package management tool that modifies the host system. **You cannot trust any change to the host machine before development is complete and verification has passed.** Isolated verification is paramount.

You MUST simulate real scenarios (install, download, update, config read/write, etc.) to prove the entire flow works end-to-end — in these two isolated environments ONLY:

1. **DI-isolated unit tests** — inject mock boundaries for filesystem, exec, network. Zero side effects on the host.
2. **Docker-containerized E2E tests** — run via `act` against `.github/workflows/ci.yml`. Real package managers, real filesystems, real commands — but inside throwaway containers.

**Never run hams operations that mutate the host** (install packages, write config outside `t.TempDir()`, execute provider commands) during development or testing. If it touches the real machine, it belongs inside a container.

## Core Philosophy

1. **Declarative serialization of host state** — record what's installed into YAML, replay on new machines.
2. **Real-world pragmatism** — not NixOS-level strict; calls existing package managers; allows latest-version installs.
3. **CLI-first, auto-record** — `hams brew install git` installs AND records. No hand-editing config first.
4. **One-command restore** — `hams apply --from-repo=zthxxx/hams-store` on a fresh machine.
5. **Diff-based updates** — hand-edit YAML, `hams apply` diffs against state and reconciles.

## Dev Commands

All tasks via [go-task](https://taskfile.dev/) — run `task --list` for full list.

```bash
task setup    # Install dev tools
task build   # Build to bin/hams
task check   # fmt → lint → test
```

Single test: `go test -race -run TestFuncName ./path/to/package/...`

## Tooling

| Tool | Purpose | Config |
|------|---------|--------|
| golangci-lint v2 | Go linting (30+ linters, strict) | `.golangci.yml` |
| ESLint 9 | JS/TS linting (flat config) | `eslint.config.ts` |
| markdownlint-cli2 | Markdown linting | `.markdownlint.yaml` |
| cspell | Spell checking | `cspell.yaml` |
| lefthook | Git hooks (pre-commit: fmt+lint, pre-push: test) | `lefthook.yml` |
| GitHub Actions | CI pipeline | `.github/workflows/ci.yml` |

## Architecture at a Glance

- **Provider plugin system**: builtins compiled in Go, externals via `hashicorp/go-plugin` (local gRPC).
- **Terraform-style state**: `.state/<machine-id>/<Provider>.state.yaml`, single-writer lock, refresh-then-diff.
- **TUI**: BubbleTea models scaffolded at `internal/tui/` (alternate screen, collapsible logs, popup) but **unwired in v1**; CLI uses plain `slog` log lines. v1.1 will plug `tui.RunApplyTUI` into `runApply`. See `openspec/changes/2026-04-16-defer-tui-and-notify/`.
- **OTel**: trace + metrics, local file exporter at `${HAMS_DATA_HOME}/otel/`.
- **Docs**: Nextra on GitHub Pages at `hams.zthxxx.me`.

15 builtin providers: Bash, Homebrew, apt, pnpm, npm, uv, goinstall, cargo, VS Code Extensions (`code-ext`), git (config/clone), defaults, duti, mas, Ansible.

## Directory Conventions

| Variable | Default | Purpose |
|----------|---------|---------|
| `HAMS_CONFIG_HOME` | `~/.config/hams/` | Global config (`hams.config.yaml`) |
| `HAMS_DATA_HOME` | `~/.local/share/hams/` | Logs, OTel, cloned repos |

Store repo layout (profile-as-directory):

```text
<store>/
  hams.config.yaml              # Project config (git-tracked)
  hams.config.local.yaml        # Local overrides (not tracked)
  .state/<machine-id>/          # State files (.gitignore'd)
  <profile-tag>/                # e.g. "macOS", "openwrt"
    <Provider>.hams.yaml        # Shared config
    <Provider>.hams.local.yaml  # Machine-specific
```

## Build & Distribution

This project is designed exclusively for *nix environments, such as Linux(Debian/Alpine) and macOS.
It does not support, nor are there plans to support, Windows environments.

Static binary (`CGO_ENABLED=0`), targets: darwin/arm64, linux/amd64, linux/arm64.
Bundles go-git for fresh machines without git. Install: `curl | bash`, `brew install`, or GitHub Releases.

## Where to Find Details

- For developer, Function, Design in OpenSpec: `openspec/`
- For Agent: `.claude/`
- For user, how to use: `docs/`

## OpenSpec

This project uses [OpenSpec](https://openspec.dev) for spec-driven development.

### Directory Structure

- `openspec/specs/` — Current capabilities that ARE built and deployed; the authoritative "as-is" state.
  - `{capability}/spec.md` — Requirements for one capability, written as `SHALL`/`MUST` statements with `#### Scenario:` blocks.
  - Never edit specs directly — specs are updated only by archiving a deployed change.
  - One capability per folder; keep names noun-based and stable (e.g., `auth`, `billing`, not `add-auth`).

- `openspec/changes/` — In-flight proposals: features being designed, built, or reviewed but NOT yet deployed.
  - `{change-id}/proposal.md` — Why the change exists, what it affects, user impact.
  - `{change-id}/tasks.md` — Checklist of implementation steps; check off `- [ ]` as work progresses.
  - `{change-id}/tasks/{capability}.task.md` — Complex tasks broken down into an independent file, linked from the main `tasks.md`.
  - `{change-id}/design.md` — Optional; only for non-trivial technical decisions, tradeoffs, alternatives.
  - `{change-id}/specs/{capability}/spec.md` — Spec deltas using `## ADDED`/`## MODIFIED`/`## REMOVED` headers, NOT full rewrites.
  - Change IDs are kebab-case verbs (`add-oauth-login`, `refactor-payment-flow`); never reuse an ID.
  - One change = one coherent shippable unit; split if it spans unrelated capabilities.

- `openspec/archive/` — Completed changes moved here after deployment; their deltas have been merged into `specs/`. Agents do not need to read this.

### Core Principles

- **Specs lag reality intentionally** — they reflect shipped behavior, not aspirations; aspirations live in `changes/`.
- **Propose before coding** — create the change folder first so humans/AI align on intent before implementation drift.
- **Deltas, not rewrites** — change specs describe diffs against current specs, making review and merging tractable.
- **Scenarios are mandatory** — every requirement needs at least one `#### Scenario:` block, otherwise validation fails.

## Current Task

Ralph Loop: Verification cycle 2 — execute deferred follow-ups from `2026-04-16-verification-findings`

### Completed in cycle 1

- [x] Fix goconst lint errors across 7 provider files
- [x] Fix Taskfile bug: `task check` was calling `task test` (includes integration/e2e via act) instead of `task test:unit`
- [x] Fix markdown lint errors (MD032 blanks-around-lists)
- [x] Run `task check` — verify build/lint/test all pass (PASSING)
- [x] Verify all shipped specs match implementation via 4 parallel agents
- [x] Verify user workflow scenarios end-to-end (Install+record, Bootstrap, Apply, Refresh, Version pin all working)
- [x] Audit test design (uneven coverage; apt has 38 tests, cargo/npm have 2; no property-based in providers)
- [x] Audit architecture extensibility (URN module, go-plugin deferred, DI consistent in core)
- [x] Record findings as OpenSpec change with task breakdown (`openspec/changes/2026-04-16-verification-findings/`)
- [x] Remove dead CLIHandler interface (unused dead code in provider.go) — zero Go references remain
- [x] Final `task check` pass (verified 0 issues, all 28 test packages PASS)
- [x] Commit cycle-1 verification findings (10de4bd)

### Cycle 2: deferred follow-ups

- [x] **spec-reconciliation/naming**: Update `openspec/specs/builtin-providers/spec.md` to use `goinstall`/`code-ext`. Grep for stale references. (commit `6f9e533`)
- [x] **lucky-enrichment**: Architect investigation found Enricher has zero implementers; `--hams-lucky` is a silent no-op flag. **Decision: defer entire feature to v1.1**, document gap honestly in spec, keep scaffolding. (commit `f4c0f20`)
- [x] **property-based parser tests**: Add via `rapid` for cargo, npm, pnpm, uv, mas, vscodeext. (commit `3467967`)
  - **Real bug found and fixed**: `parseExtensionList` silently emitted empty/whitespace keys on malformed input — would corrupt the desired-vs-observed diff.
- [x] **Tier 3 tempdir-isolated tests**: git-config + git-clone apply/probe/remove via HOME redirect (no FakeCmdRunner refactor needed). (commits `703f66c`, `588b86e`)
- [x] **Tier 1 lifecycle tests CLOSED** — all 5 package-like providers (cargo, npm, pnpm, uv, goinstall) refactored to CmdRunner DI + apt-style U-pattern lifecycle tests. Coverage gains: cargo 28→69%, npm 23→68%, pnpm 30→71%, uv 32→70%, goinstall 14→62%. Commits f3dde9a, a972bd4, 5bad9cd, 682f22b.
- [x] **Tier 2 apply/probe DI tests CLOSED** — all 3 macOS-specific providers (mas, duti, defaults) refactored to CmdRunner DI + lifecycle tests. Coverage gains: mas 39→73%, duti 31→80%, defaults 20→59%. Commits 43be98b, 933080d, ff8fada.
- [x] Verify `task check` passes after each change. Atomic commit per fix.

### Cycle 2 summary

Cycle 2 closed both test-coverage tiers — 8 of 8 providers with package-manager semantics now have DI-isolated lifecycle tests. All commits pass `task check` (0 lint, 28/28 PASS). Zero real package-manager invocations from unit tests. First Principle (CLAUDE.md) enforced across the board.

Real bugs found and fixed during cycle:

- `parseExtensionList` silently corrupted diff on `@version`-prefix or tab-containing inputs (commit 3467967).

Architectural decisions documented:

- `--hams-lucky` flag deferred to v1.1; spec rewritten to match shipped reality (commit f4c0f20).
- DAG zero-indegree tie-breaking is alphabetical (priority list is inert for root-level providers); codified in `TestResolveDAG_ZeroIndegreePriority` + `provider-system/spec.md` delta (commit 10de4bd).

Spec corrections:

- `goinstall`/`code-ext` naming reconciled across 4 spec files + en/zh-CN docs + README variants (commit 6f9e533).

Total commits in cycle 2: 15+ (still growing — iteration 3 adds hooks+OTel defer).

### Cycle 14 — Malformed YAML errors surface correctly

- [x] **Swallowed config.Load error in `runApply`** — when resolving store path, a malformed `~/.config/hams/hams.config.yaml` was demoted to a generic "no store directory configured" error, hiding the real YAML parse failure. Now propagates the config.Load error as-is. (commit `cbd70f4`)
- [x] **Store-config errors include the file path** — `mergeFromStoreFile` returned a bare YAML error; `config.Load` now wraps with the project/local file path so users know which file to fix.
- [x] **Dropped triple-nested error message** — merge.go no longer adds its own "parsing <path>" wrap because the caller already names the path.
- [x] Regression tests: `TestLoad_MalformedGlobalYAMLSurfaces`, `TestLoad_MalformedStoreYAMLSurfaces`.

### Cycle 13 — UX papercuts surfaced by running `hams list` fresh

- [x] **`hams list` empty-state** — silently exited 0 with no output when zero resources existed. Indistinguishable from a hung command or a silently swallowed error. Added a message pointing users to `hams <provider> install` + `hams apply`. JSON mode unchanged (still `[]`). (commit `c7b1456`)
- [x] **Dedup `profile_tag`/`machine_id` WARN noise** — `config.Validate()` fired the "using 'default'" warnings on every `Load()`, and `hams list` calls `Load()` twice (once during provider registration, once during the command action). Duplicate log lines confuse users into thinking something is wrong. Guarded with `sync.Once` so each warning fires at most once per process. Exposed `ResetValidationWarnOnce()` for tests; added `TestValidate_WarnsOncePerProcess`. (commit `c7b1456`)

### Cycle 12 — CLI correctness sweep

Four related fixes turned up by following the Cycle 11 help-text audit downstream through the CLI layer (commit `1c41667`):

- [x] **Context propagation** — `routeToProvider` was calling `context.TODO()`, dropping the cancellation context from urfave/cli. Now forwards the real context, so provider handlers see caller cancellation.
- [x] **SIGINT/SIGTERM handling** — `Execute` now wraps root context with `signal.NotifyContext`; Ctrl+C cancels running provider commands instead of leaving them orphaned. `stop()` runs before `os.Exit`.
- [x] **Deterministic `hams --help`** — provider subcommand list was in Go-map iteration order (changed every run). Sorted alphabetically before registering. Reproducible output for users and for docs snapshots.
- [x] **Per-provider help heading fix** — `showProviderHelp` still said "Manage X packages" — same drift as Cycle 11 top-level help. Routed through `providerUsageDescription()`.
- [x] Regression tests: `TestRouteToProvider_ContextForwarded`, `TestNewApp_ProviderCommandsAreSorted` (20-run determinism check).

### Cycle 11 — UX fix: per-provider help-text Usage descriptions

- [x] **`hams --help` drift**: the per-provider Usage string was hardcoded to `fmt.Sprintf("Manage %s packages", displayName)` for every provider. Wrong for 7 non-package providers (git-config, git-clone, defaults, duti, bash, ansible, code-ext). Introduced `providerUsageDescription(name, displayName)` with per-provider switch + package-class fallback for future external plugins. Added 3 table-driven tests covering all branches (16 cases total) (commit `0545c15`).

### Cycle 10 — internal/cli coverage gains (utility paths)

- [x] **internal/cli: 39.5% → 42.0%** — added tests for previously-zero pure functions: `parseCSV` (--only/--except parsing, 3 cases), `validateProviderNames` (happy path + unknown providers with ExitUsageError + suggestion list per cli-architecture spec), `PrintError` (text mode, JSON mode, plain-error wrapping). Added reusable `captureStderr` helper (commit `9e1e387`).

### Cycle 9 — Coverage gains for error + llm packages

- [x] **internal/error: 35.7% → 100%** — added `TestErrorCodeFromExit_AllBranches` covering all 9 exit codes + fallbacks; `TestNewUserErrorWithCode` (explicit-code constructor); `TestNewUserError_AutoDerivesErrorCodeFromExit`; `TestUserFacingError_AsTargetType` (errors.As recovery for cmd/hams exit handler) (commit `cd78959`).
- [x] **internal/llm: 29.9% → 80.6%** — added `TestRecommend_NoLLMConfigured`, `TestEnrichAsync_PropagatesRecommendError`, `TestEnrichCollector_AddCollectAll`/`AddAfterCollect`, `TestReportErrors_Empty`/`WithFailures` (commit `cd78959`).

### Cycle 8 — Dead code removal + project-structure spec patch

- [x] **`internal/runner/` deleted** — generic Runner interface superseded by per-provider CmdRunner abstractions; zero callers across internal/, pkg/, cmd/. Also removed `WrapExecWithRunner` + `WrapExecPassthroughWithRunner` (~240 lines dead) (commit `abd0bc6`).
- [x] **`Formula/` + `examples/` added to project-structure spec** — both top-level dirs ship in the repo and are referenced by other specs (self-upgrade Homebrew channel + dev-sandbox), but were missing from the canonical layout (commit `0738457`).

### Cycle 7 — TUI + notify deferred + OTel attr tests

- [x] **TUI alternate-screen rendering deferred to v1.1** — `internal/tui/` ships ~500 lines of BubbleTea models but `RunApplyTUI` has zero callers. Same scaffolded-but-unwired pattern as lucky/hooks/OTel (commit `c7249d4`).
- [x] **Notification system deferred to v1.1** — `internal/notify/` ships full Channel/Manager/terminal-notifier/Bark scaffolding but `Manager.Send` has zero callers (commit `c7249d4`).
- [x] **CLAUDE.md TUI claim corrected** — was advertising TUI as a shipped feature; now honest about the deferral (commit `45ebf05`).
- [x] **OTel attribute conformance tests** — covered `AttachRootAttrs` + status→`hams.result` mapping with 3 new tests (table-driven over 4 status mappings) (commit `2ffe525`).

### Cycle 6 — OTel spec conformance + 3 spec-reconciliation fixes

- [x] **Provider-system spec table reconciled** — removed `system`/`file`/`download` (spec'd Builtin but unimplemented in v1); ansible relabeled from "External (v1-deferred)" to "Builtin" to match reality (commit `da33233`).
- [x] **`hams store status` subcommand** added — spec requires it but impl only had `hams store` default action (commit `98b643d`).
- [x] **Self-upgrade spec updated** — v1 impl has no confirmation prompt / no `--yes` flag; spec now honestly says "upgrade directly" with a v1.1 note for future `--confirm` opt-in (commit `98b643d`).
- [x] **OTel attribute conformance** — span attrs renamed to `hams.resource.*` / `hams.provider.*` / `hams.profile` / `hams.providers.count` / `hams.result` per observability spec. Root span gets `AttachRootAttrs(profile, count)` after CLI resolves the provider set. `hams.apply.duration` + `hams.probe.duration` metrics emitted. Provider-failures counter now semantically correct (1 per failing provider). Skipped resources get their own `hams.resource.skip` span. (commits `b0bd68c`, `a36de54`)

### Cycle 5 — OTel delivered (un-deferred)

- [x] **OTel CLI integration IMPLEMENTED** — opt-in via `HAMS_OTEL=1` env var (commit `1cfd54e`). Closes the deferral from commit `ed1a5af`.
  - `internal/cli/otel.go`: `maybeStartOTelSession(dataHome, operation)` → `otelSessionState` with `Session()` accessor + `End(ctx, status)`. Loose-boolean env parsing (true/yes/on/1).
  - `runApply` + `runRefresh` both wrap operations in root spans (`hams.apply` / `hams.refresh`) and pass the session to `provider.Execute`.
  - Named return `(retErr error)` in both so the defer tags the root span with `ok`/`error` based on final return.
  - End-to-end test proves `HAMS_OTEL=1 → trace JSON file appears under ${HAMS_DATA_HOME}/otel/traces/`.
  - Updated cli-architecture spec to reflect shipped reality with 3 user-facing scenarios.

### Cycle 4 — Hooks delivered (un-deferred)

- [x] **Hamsfile hooks parsing IMPLEMENTED** — full YAML → Plan → Execute → runHook pipeline works end-to-end (commit `1479129`). Closes the deferral from commit `ed1a5af`.
  - `internal/hamsfile/hooks.go`: `(*File).AppHookNode(appID)` walks YAML tree.
  - `internal/provider/hooks_parse.go`: `ParseHookSet(node)` + `PopulateActionHooks(actions, desired)`.
  - All 13 providers' `Plan()` updated to call `PopulateActionHooks`.
  - End-to-end tests prove hooks fire with real shell side effects (touch a marker file).
  - Removed the lint-warning fallback (hamsfile/lint.go, 300 lines) — no longer needed.
  - Updated schema-design + cli-architecture specs to reflect shipped reality.

**Only 1 v1.1 deferral remains**: `--hams-lucky` LLM enrichment (requires Enricher implementation on at least one provider — a feature ticket, not a verification task).

### Cycle 3 — COMPLETE

- [x] Architectural audit (state, hooks, lock, sudo, OTel) — **two new drifts found**: hooks engine has zero parsers wiring it; OTel exporter has zero CLI integration. Both deferred to v1.1 (commit `ed1a5af`).
- [x] Homebrew CmdRunner DI refactor — 15.9% → 45.2% (commit `a9cebe7`).
- [x] Vscodeext FilePrefix self-correction — docs incorrectly said `code-ext.hams.yaml`; impl ships `vscodeext.hams.yaml` intentionally (commit `2ac1a58`).
- [x] Vscodeext CmdRunner DI refactor — 29.0% → 67.4% (commit `b70481b`).
- [x] Hamsfile fail-loud warning when `hooks:` block declared (improves UX of the hooks-defer; commit `8abf8e9`).
- [x] CLI fail-loud warning when `--hams-lucky` passed (improves UX of the lucky-defer; commit `bee5e67`).
- [x] Ansible CmdRunner DI refactor — 17.9% → 70.5% (commit `efea1b8`).

**Cycle 3 milestone**: ALL 12 testable providers now have CmdRunner DI + apt-style U-pattern lifecycle tests. (bash is the script-host that other providers' Bootstrap chains target via `BashScriptRunner` — testing it requires a different shape and is out of scope for this DI refactor.)

| Provider | Before → After |
|----------|---------------|
| apt | 77.1% (already had DI) |
| cargo | 28.8% → 68.8% |
| npm | 23.4% → 67.7% |
| pnpm | 29.8% → 71.4% |
| uv | 31.5% → 70.0% |
| goinstall | 13.7% → 62.0% |
| mas | 38.6% → 72.7% |
| duti | 31.4% → 80.2% |
| defaults | 20.4% → 58.9% |
| homebrew | 15.9% → 45.2% |
| vscodeext | 29.0% → 67.4% |
| ansible | 17.9% → 70.5% |
| git | 1.8% → 23.0% |

UX improvements for the v1.1-deferred features (hooks, OTel, lucky):

- Users who pass `--hams-lucky` now see a fail-loud warning naming the affected provider.
- Users who add a `hooks:` block to a hamsfile see a fail-loud warning naming the affected entries.
- The OTel deferral has zero user-visible touch points (no flag, no config that suggests it works) and needs no warning.

Cycle 3 commits: 8.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
