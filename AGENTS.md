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

- [x] **2. Route all builtin-provider user-facing strings through i18n** (spec: `cli-architecture/spec.md:103-116`)
  Archived as `openspec/changes/archive/2026-04-18-i18n-builtin-provider-catalog/`. All 14 builtin providers now route `NewUserError` + dry-run previews through `i18n.T` / `i18n.Tf`; shared `provider.*` catalog keys (60+ entries) in `en.yaml` + `zh-CN.yaml`; `TestProviderKeysResolve{English,Chinese}` regression tests catch missing yaml entries at build time. Shared helpers `provider.{UsageRequiresResource, UsageRequiresAtLeastOne, DryRunInstall, DryRunRemove, DryRunRun}` consolidate the dominant pattern into one-liner call sites. Follow-up 2.14.a (git/clone.go + git/git.go internals, ~12 sites) noted in the archived change.

- [x] **3. Adopt the shared package-dispatcher abstraction in real providers** (`CLAUDE.md` → *Current Tasks*)
  Archived as `openspec/changes/archive/2026-04-18-provider-shared-abstraction-adoption/`. `cargo` is the reference adopter — its `handleInstall` / `handleRemove` delegate to `provider.AutoRecordInstall` / `AutoRecordRemove`. The dispatcher's user-facing strings route through i18n. `provider-system/spec.md` gains a SHALL requiring Package-class providers to use the dispatcher by default, with a documented exemption process for signature mismatches. Follow-up 5.1–5.8 (migrate npm/pnpm/uv/goinstall/mas/vscodeext, design batch + extra-arg dispatcher variants for apt/brew) tracked in the archived change's tasks.md.

- [x] **4. Consolidate the store scaffold into a dedicated package** — subsumed into task 1 (archived via `2026-04-18-storeinit-package-with-gogit-fallback`). `internal/cli/template/` is deleted; the only `//go:embed template` in the code base is at `internal/storeinit/storeinit.go`.

- [x] **5. Integration test: verify `hams git` passthrough is preserved for the real git verbs** — shipped as part of task 3's commit; the test block at the end of `internal/provider/builtin/git/integration/integration.sh` runs `hams git status`, `hams git rev-parse HEAD`, `hams git log -1`, `hams git branch --show-current` against a freshly-inited repo and asserts exit 0 for each.

- [x] **6. Final verification pass** (`CLAUDE.md` → *Mandatory Verification Before Delivery*)
  - [x] 6.1 `task check` (fmt + lint + full test suite, race detector on) passes on the final commit. The suite composition (per `Taskfile.yml`) is unit + integration + e2e; the final run exits 0 with the tail line "=== All OpenWrt E2E tests passed ===".
  - [x] 6.2 The e2e pipeline that `task check` drives is the same pipeline CI runs via the `.github/workflows/ci.yml` matrix — Local/CI isomorphism preserved. The new integration hooks (apt E0 for the go-git fallback, git passthrough for `status/rev-parse/log/branch`) are exercised on every run.
  - [x] 6.3 User-scenario smoke: the apt E0 integration test is a container smoke of "fresh machine without git, `hams apt install htop`" — the exact scenario `project-structure/spec.md:686-699` promises. `standard_cli_flow` covers `hams brew install / remove / apply --only=<provider>` shape across every in-scope package provider.
  - [x] 6.4 `rg 'i18n\.' internal/provider/builtin/ -g '!*_test.go'` is non-empty (17 files) AND `TestProviderKeysResolve{English,Chinese}` verifies every key declared in `keys.go` resolves in both locales.
  - [x] 6.5 `rg 'AutoRecordInstall|AutoRecordRemove' internal/provider/builtin/` returns `cargo/cargo.go` (reference adopter). The other 6 matching-signature providers (`npm`, `pnpm`, `uv`, `goinstall`, `mas`, `vscodeext`) remain follow-up 5.1–5.6 in the archived change — same refactor pattern, one atomic commit each.

All tasks use the OpenSpec workflow. Use `/opsx:new` or `/opsx:propose`
to start a change, `/opsx:apply` to drive implementation, `/opsx:verify`
before archival, and `/opsx:archive` to close it out. Mark the
corresponding top-level `[x]` only when the change is fully archived.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
