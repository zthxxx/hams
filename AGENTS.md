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

Ralph Loop: extended autonomous verification (cycles 11–28).

This block logs the cycles that ran after the user instructed the agent to continue verifying/fixing autonomously overnight. Each cycle is a self-contained fix (code or spec) with `task check` passing and an atomic commit. See the per-cycle headings below for specifics.

Status: `task check` passes with 0 lint issues across the full test suite. Coverage gains summary: `internal/cli` 37% → 44%, `internal/config` 74% → 77%, `internal/error` 36% → 100%, `internal/llm` 30% → 81%, `internal/provider/builtin/bash` 51% → 86%, `internal/provider/builtin/homebrew` 45% → 49%, `internal/provider/builtin/ansible` 18% → 77%, `internal/provider/builtin/defaults` 20% → 60%. Real user-facing bugs fixed across CLI ergonomics, context/signal handling, error surfacing, spec-impl reconciliation, and test coverage.

### Cycle 243 — Every CLI command honors `--debug` via root Before hook

- [x] Cycle 242 wired `SetupDebugOnly` into `routeToProvider`, so per-provider CLI commands honored `--debug`. But top-level commands (`hams config get key --debug`, `hams list --debug`, `hams store status --debug`, `hams version --debug`) bypass `routeToProvider` — they go through urfave/cli Action functions directly, so `--debug` was still parsed but ignored. Add a `Before:` hook on the root `cli.Command` that calls `logging.SetupDebugOnly(true)` when `--debug` is set. Fires once before every Action, so every command surface honors `--debug` uniformly. Implementation note: the hook only fires when `--debug` is set — the default branch leaves the caller-installed `slog.Default` alone, important for tests (e.g. `TestList_CorruptStateFileEmitsWarning`) that install their own capture handler before `app.Run`. Same conditional applied to the cycle-242 provider_cmd.go call site for symmetry. (commit `bb397ad`)

### Cycle 244 — `hams --json apply --dry-run` emits `planned_actions`

- [x] Cli-architecture spec §"Dry-run apply shows planned actions" mandates the output SHALL list all actions with their types and target resources. Text-mode dry-run already does (via `printDryRunActions`). But JSON-mode dry-run (cycle 237+) only emitted aggregates — `dry_run: true`, `success`, `skipped_providers`, `state_save_errors`, `elapsed_ms` — zero information about WHICH actions were planned per provider. A CI script running `hams --json apply --dry-run | jq '.planned_actions[] | select(.actions[].type == "install")'` had nothing to iterate. That directly contradicted the spec: the machine-readable dry-run surface was strictly less informative than the human-readable one. Fix: introduce `dryRunProviderEntry { Name, DisplayName, Actions }` captured inside the dry-run branch of `runApply` when `flags.JSON` is set (mirror of the `printDryRunActions` call for text mode). Route the slice into `emitDryRunJSON` via a new `plannedActions []dryRunProviderEntry` parameter. `marshalDryRunActions` shapes it into `[{provider, display_name, actions: [{type, id}]}]` for stable JSON consumption — uses `Action.Resource` string-cast when present (providers like bash that use tokens), else `Action.ID`, matching text mode's priority. Final JSON now includes `planned_actions: [...]`. Regression test extends `TestRunApply_DryRunJSONHasNoProse` to assert: array present, length 1, `provider=alpha`, `actions[0]={type:install, id:pkg-a}`. `task fmt lint test:unit` all green (0 issues, 33/33 packages, `internal/cli` coverage 44% → 77%). (commit `a35535b`)

### Cycle 245 — `PrintError` JSON-mode includes `error_code` for plain errors

- [x] Cli-architecture spec §"Error in JSON mode" mandates the JSON error object SHALL include `error_code` (the coarse category derived from `errorCodeFromExit`). `UserFacingError` paths carried it correctly — `NewUserError` / `NewUserErrorWithCode` both populate the field. But `PrintError`'s fallback path for non-`UserFacingError` errors constructed a bare `&UserFacingError{Code: ExitGeneralError, Message: err.Error()}` — zero-value `ErrorCode`. Since the struct tag is `json:"error_code,omitempty"`, the serializer stripped it entirely. CI/agent scripts parsing `error_code` saw it present on structured errors and absent on unstructured ones — a silent shape divergence between call sites. Fix: replace the bare struct literal with `hamserr.NewUserError(hamserr.ExitGeneralError, err.Error())`, which runs `errorCodeFromExit` and populates `ErrorCode = CodeGeneralError` uniformly. Two-line diff in `internal/cli/errors.go`. Regression test `TestPrintError_JSONMode_PlainErrorIncludesErrorCode` in `internal/cli/utils_test.go` wraps `errors.New("network blew up")`, calls `PrintError(err, jsonMode=true)`, unmarshals stderr, asserts `error_code="GENERAL_ERROR"` and `code=1`. `task fmt lint test:unit` all green. (commit `5596e69`)

### Cycle 246 — `hams --json config set|unset` emits structured result

- [x] `hams --json config get` (cycle 236), `hams --json config list`, `hams --json apply|refresh|list`, `hams --json version` (cycle 181) all emit JSON. But `hams --json config set <key> <value>` and `hams --json config unset <key>` emitted the same plain-text `Set <key> = <value> (in <target>)` / `Unset <key> (from <target>)` as without `--json`. A CI script running `hams --json config set notification.bark_token abc | jq '.key'` failed because stdout was non-JSON. The principle that `--json` applies globally was broken on two widely-used config write paths. Fix: introduce `emitConfigSetResult` / `emitConfigUnsetResult` helpers in `internal/cli/commands.go` that branch on `flags.JSON` and emit `{"key", "value", "target", "dry_run"}` (set) / `{"key", "target", "dry_run"}` (unset) shapes — symmetric with the cycle-236 `config get --json` shape and the apply/refresh JSON conventions. Text mode preserves the pre-cycle-246 wording verbatim (no regression for human users). Covers both the real-write and `--dry-run` branches. Three regression tests: `TestConfigSet_JSONMode_EmitsStructuredResult`, `TestConfigSet_JSONMode_DryRunFlagsDryRunTrue` (includes assertion that the config file remains absent — JSON path still respects `--dry-run`'s no-mutation contract), `TestConfigUnset_JSONMode_EmitsStructuredResult`. All use the shared `assertConfigCmdJSON` helper that unmarshals stdout and checks each expected field. `task fmt lint test:unit` all green.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
