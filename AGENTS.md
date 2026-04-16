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
- **TUI**: BubbleTea alternate screen, collapsible logs, interactive popup for stdin.
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

### Cycle 3 (in progress)

- [x] Architectural audit (state, hooks, lock, sudo, OTel) — **two new drifts found**: hooks engine has zero parsers wiring it; OTel exporter has zero CLI integration. Both deferred to v1.1 (commit `ed1a5af`).
- [x] Homebrew CmdRunner DI refactor — 15.9% → 45.2% (commit `a9cebe7`).
- [x] Vscodeext FilePrefix self-correction — docs incorrectly said `code-ext.hams.yaml`; impl ships `vscodeext.hams.yaml` intentionally (commit `2ac1a58`).
- [x] Vscodeext CmdRunner DI refactor — 29.0% → 67.4% (commit `b70481b`).
- [ ] Hooks parsing implementation (would deliver the deferred feature; medium-large work).
- [ ] Ansible DI (lower priority, bash-chain provider).
- [ ] CLI workflow verification (E2E happy-path audit).

**Cycle 3 milestone**: 11 of 11 testable providers now have CmdRunner DI + apt-style U-pattern lifecycle tests. Coverage table:

| Provider | Before → After |
|----------|---------------|
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
| git | 1.8% → 23.0% |

Only ansible (bash-script chain, different shape) lacks DI-isolated tests.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
