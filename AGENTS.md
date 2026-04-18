# CLAUDE.md

Guidance for Claude Code agents working in this repository.

## Project Overview

name: **hams** (hamster) — declarative IaC environment management for macOS/Linux workstations.
Wraps existing package managers (Homebrew, pnpm, npm, apt, etc.) to auto-record installations
into YAML config files ("Hamsfiles"), enabling one-command environment restoration on new machines.

| Key | Value |
|-----|-------|
| Go module | `github.com/zthxxx/hams` |
| Go version | 1.26 |
| DI framework | [Uber Fx](https://github.com/uber-go/fx) |
| Entry point | `cmd/hams/main.go` |
| i18n framework | [go-i18n](https://github.com/nicksnyder/go-i18n) |
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
- **TUI**: BubbleTea models scaffolded at `internal/tui/` (alternate screen, collapsible logs, popup) but **unwired in v1**; CLI uses plain `slog` log lines. v1.1 will plug `tui.RunApplyTUI` into `runApply`. See `openspec/changes/archive/2026-04-17-defer-tui-and-notify/`.
- **OTel**: trace + metrics, local file exporter at `${HAMS_DATA_HOME}/otel/`.
- **Docs**: Nextra on GitHub Pages at `hams.zthxxx.me`.

14 builtin providers: Bash, Homebrew, apt, pnpm, npm, uv, goinstall, cargo, VS Code Extensions (`code`), git (unified config + clone), defaults, duti, mas, Ansible.

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

## Current Tasks

These tasks are derived from `docs/notes/dev-vs-loop-gap-analysis.md` —
spec-citable gaps between the current branch (`local/loop`) and the
reference remote branch (`origin/dev`). Every task below closes a
concrete SHALL in `openspec/specs/**` or a hard rule in CLAUDE.md. The
remote `dev` branch is read-only reference material — all work lands on
the current branch.

Each numbered bullet is **one** OpenSpec change. Follow the OpenSpec
workflow end-to-end: propose (`/opsx:new` or `/opsx:propose`) → design
(if non-trivial) → implement → verify → archive. Mark the checklist
item `[x]` only when the change is archived AND `task check` passes.

Atomicity rule: every listed task has a "verification" sub-item as its
last step. No parent task may be closed until that sub-item is green.

- [x] **1. Restore `go-git` fallback in auto-scaffold** (spec: `project-structure/spec.md:686-699`)
  Archived as `openspec/changes/archive/2026-04-18-storeinit-package-with-gogit-fallback/`. New `internal/storeinit/` package exposes `Bootstrap`, `Bootstrapped`, `DefaultLocation` + DI seams `ExecGitInit` / `GoGitInit`. Integration test E0 in `internal/provider/builtin/apt/integration/integration.sh` hides system git and asserts the "bundled go-git" log line fires.

- [ ] **2. Route all builtin-provider user-facing strings through i18n** (spec: `cli-architecture/spec.md:103-116` — "all user-facing strings (errors, help text, prompts) go through" the i18n catalogue)
  Today `rg 'i18n\.' internal/provider/builtin/ -g '!*_test.go'` returns **zero** results. Every `hamserr.NewUserError(…)`, every dry-run prose line, every `fmt.Fprintf(flags.Stdout(), …)` user message must round-trip through `i18n.T` / `i18n.Tf` with a typed key in `internal/i18n/keys.go`.
  - [ ] 2.1 Propose OpenSpec change `i18n-provider-catalog` (deliver as delta against `cli-architecture/spec.md` and `builtin-providers/spec.md`).
  - [ ] 2.2 Extend `internal/i18n/keys.go` with typed constants under `provider.<provider>.*` for every provider-emitted string. Naming: `provider.<name>.<verb>.<slot>` (e.g., `provider.apt.install.usage`).
  - [ ] 2.3 Fill `internal/i18n/locales/en.yaml` (canonical source) and `zh-CN.yaml` (complete translations).
  - [ ] 2.4 Wire each builtin provider's user-facing strings to `i18n.T`. Providers to cover: `apt`, `bash`, `brew`/`homebrew`, `cargo`, `git`, `goinstall`, `mas`, `npm`, `pnpm`, `uv`, `vscodeext`, `duti`, `defaults`, `ansible`.
  - [ ] 2.5 Log statements (slog) are exempt — English only for log records, user-visible prose is not.
  - [ ] 2.6 Integration test: each provider's CLI output changes under `LANG=zh_CN.UTF-8`. Add a shared assertion helper in `e2e/base/lib/i18n_assert.sh`.
  - [ ] 2.7 `task check` passes + archive the change.

- [ ] **3. Adopt the shared package-dispatcher abstraction in real providers** (`CLAUDE.md` → *Current Tasks* — "design shared abstractions … extending with a new provider is a matter of filling in a well-defined template, not reimplementing the pattern from scratch")
  Today `internal/provider/package_dispatcher.go` (190 LoC) and `internal/provider/baseprovider/` (from any dev-style port) are dead code — zero adopters in `internal/provider/builtin/`. A template with zero adopters is a guess, not an abstraction.
  - [ ] 3.1 Propose OpenSpec change `provider-shared-abstraction-adoption` with a `provider-system/spec.md` delta that adds a new SHALL: "New Package-class providers SHALL route install/remove/list through `provider.AutoRecordInstall` / `AutoRecordRemove` unless they document why in their spec delta."
  - [ ] 3.2 Pick `apt` as the proof-of-abstraction adopter. Port its `handleInstall` / `handleRemove` onto `provider.AutoRecordInstall` / `AutoRecordRemove` without regressing the apt-specific complex-invocation detection (`builtin-providers/spec.md:1034`).
  - [ ] 3.3 Batch-migrate `brew`, `pnpm`, `npm` — each must keep its provider-specific argument extractor but hand off the lock/install/record flow to the dispatcher.
  - [ ] 3.4 Batch-migrate `cargo`, `goinstall`, `uv`, `mas`, `vscodeext` — same pattern.
  - [ ] 3.5 Delete or consolidate any second shared helper (e.g., a `baseprovider`-shaped remnant) so there is exactly one "shared base" helper per category.
  - [ ] 3.6 Update `CLAUDE.md` → *Directory Conventions* with a one-line pointer to the shared dispatcher.
  - [ ] 3.7 Integration tests: each migrated provider's lifecycle (install → re-install → install-new → refresh → remove) continues to pass under `standard_cli_flow`.
  - [ ] 3.8 `task check` passes + archive the change.

- [ ] **4. Consolidate the store scaffold into a dedicated package** (`CLAUDE.md` → *package hygiene*)
  Partially overlaps with task 1; call out here to ensure the boundary stays clean even if task 1 lands incrementally. After completion, `internal/cli/` MUST NOT contain `//go:embed template/store` — embeds live in `internal/storeinit/`.
  - [ ] 4.1 Audit `internal/cli/` for any scaffold-shaped code and move it to `internal/storeinit/`.
  - [ ] 4.2 Keep CLI-surface helpers (flag parsing, EnterCommand wiring) in `internal/cli/`.
  - [ ] 4.3 `task check` passes.
  - [ ] 4.4 If task 1's change hasn't archived yet, fold this work into the same change so there is one coherent delta; otherwise archive a follow-up change.

- [ ] **5. Integration test: verify `hams git` passthrough is preserved for the real git verbs** (`builtin-providers/spec.md:69`)
  Current implementation at `internal/provider/builtin/git/unified.go` is correct; the test is a regression gate.
  - [ ] 5.1 Propose small OpenSpec change `git-passthrough-regression-test` (deltas optional — can attach to an existing archived change's tasks if that reads better).
  - [ ] 5.2 Extend `internal/provider/builtin/git/integration/integration.sh` to run `hams git status`, `hams git log -1`, `hams git rev-parse HEAD`, `hams git branch` and assert exit 0 + sensible output.
  - [ ] 5.3 `task check` passes + archive (or roll into task 3's archive if co-shipping).

- [ ] **6. Final verification pass** (`CLAUDE.md` → *Mandatory Verification Before Delivery*)
  - [ ] 6.1 `task check` (lint + all unit + integration tests) passes.
  - [ ] 6.2 `task test:e2e:one TARGET=debian-amd64` reproduces the full workflow under `act` locally and matches GitHub Actions.
  - [ ] 6.3 User-scenario smoke: fresh container without `git` on PATH, `hams apply --from-repo=https://github.com/zthxxx/test-store.hams.git` recovers the expected state; `hams brew install htop` on a fresh container with `brew` installed produces a committable single-commit `hams.config.yaml`+`default/Homebrew.hams.yaml` with no interactive prompts.
  - [ ] 6.4 Verify `rg 'i18n\.' internal/provider/builtin/ -g '!*_test.go'` is non-empty AND every string listed in `internal/i18n/keys.go` exists in both `en.yaml` and `zh-CN.yaml`.
  - [ ] 6.5 Verify `rg 'AutoRecordInstall|AutoRecordRemove' internal/provider/builtin/` has at least one adopter per package-like provider.

All tasks use the OpenSpec workflow. Use `/opsx:new` or `/opsx:propose`
to start a change, `/opsx:apply` to drive implementation, `/opsx:verify`
before archival, and `/opsx:archive` to close it out. Mark the
corresponding top-level `[x]` only when the change is fully archived.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
