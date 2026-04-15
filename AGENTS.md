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
task setup    # Install dev tools       task build   # Build to bin/hams
task test     # Tests with -race        task lint    # All linters
task fmt      # gofmt + goimports       task check   # fmt → lint → test
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

15 builtin providers: Bash, Homebrew, apt, pnpm, npm, uv, go, cargo, VSCode Extension, git (config/clone), defaults, duti, mas, Ansible.

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

Active proposal: `apt-cli-complex-invocations` — proposal + builtin-providers spec delta only, awaiting human review before implementation. Surfaced from codex-review round 3 on the second archived cycle. Scope: extend apt CLI auto-record with structured version/release pinning fields in the hamsfile + state schema, replacing today's "complex invocation; skip auto-record" short-circuit for those two cases (dry-run flags stay skipped).

Two cycles archived this session:

1. `fix-apt-cli-state-write-and-htop-rename` (2026-04-15, archive at `openspec/changes/archive/2026-04-15-fix-apt-cli-state-write-and-htop-rename/`) — apt CLI state-write + bat→htop rename + two-stage scope gate + per-provider docker integration matrix.
2. `clarify-apply-state-only-semantics` (2026-04-15, archive at `openspec/changes/archive/2026-04-15-clarify-apply-state-only-semantics/`) — `hams apply --prune-orphans` opt-in destructive reconciliation for state-only providers (when the user has deleted the hamsfile). Default behavior preserved (skip).

Three rounds of codex-review surfaced 6 P2 findings; all 4 in-session-fixable findings landed as atomic commits (`fcc3415`, `5c96462`, `f7a2020`, `e9bef58`). The 2 remaining (state-only apply semantics, apt grammar-aware recording) became deferred openspec proposals (`clarify-apply-state-only-semantics` — implemented + archived; `apt-cli-complex-invocations` — proposed, pending review).

Summary of the most recent (clarify-apply-state-only-semantics) cycle:

- [x] Codex review on the prior cycle's branch surfaced 2 P2 findings; an autonomous architect+user agent debate decided per-finding. P2 #1 (apt CLI flag passthrough + multi-pkg atomicity) → fixed in-session at commit `fcc3415` (widened `CmdRunner.Install/Remove` to `args []string`, added U18 + U19 unit tests). P2 #2 → deferred to this new spec because the destructive default flip warrants explicit scenarios + an opt-in path.
- [x] `/opsx:new` + `/opsx:continue` produced the full 4-artifact set (proposal, design, cli-architecture spec delta, tasks).
- [x] `/opsx:apply` implemented `hams apply --prune-orphans`: new `hamsfile.NewEmpty(path)` helper, runApply branches into the prune path when stateOnly && pruneOrphans, stamps the synthesized empty-doc hash on observed.ConfigHash so ComputePlan generates remove-actions (the existing `lastConfigHash != ""` guard would otherwise suppress them since CLI install handlers never set ConfigHash).
- [x] 4 unit tests (default skip, prune removes, no state file no-op, hamsfile-present no-op) + apt itest E6 (real apt-get install→delete hamsfile→apply with/without flag) all green.
- [x] en + zh-CN docs updated with explicit "destructive; default off" warnings.
- [x] `/opsx:verify` — 0 critical / 0 warning; all 7 scenarios mapped to code or tests.
- [x] `/opsx:archive` — archived with `--skip-specs` (same auto-sync header bug as prior cycle); cli-architecture delta applied manually.

Summary of the earlier (fix-apt-cli-state-write-and-htop-rename) cycle:

- [x] `/opsx:new fix-apt-cli-state-write-and-htop-rename` + `/opsx:continue` (proposal → design → specs → tasks).
- [x] `/opsx:apply` — implemented in atomic commits: apt CLI handler writes state directly (new DI: `statePath` + `loadOrCreateStateFile`), `bat`→`htop` rename across specs/examples/README/docs/E2E fixtures, two-stage scope gate (`provider.HasArtifacts` stage-1 before `--only`/`--except` stage-2) in both `runApply` and `runRefresh`, per-provider docker integration-test scaffolding (`hams-itest-base` + per-provider Dockerfile/integration.sh with SHA-keyed cache, shared `standard_cli_flow` helper, `task ci:itest:run PROVIDER=<name>`).
- [x] All 11 linux-containerizable providers shipped their `integration/{Dockerfile, integration.sh}`: apt (canonical), ansible, bash, cargo, git (config + clone in shared container), goinstall, homebrew (non-root brew user workaround), npm, pnpm, uv, vscodeext.
- [x] `/opsx:verify` — 0 critical, 0 warning; spec deltas mapped to code.
- [x] Local docker verification of the full itest matrix on OrbStack (2026-04-16): all 11 providers green end-to-end. Three last-mile fixes surfaced and landed as atomic commits:
  - `fix(mas)`: extract `cliName` const (pre-existing goconst regression).
  - `fix(homebrew)`: `os.IsNotExist` doesn't traverse `%w`-wrapped errors; switched to `errors.Is(err, fs.ErrNotExist)`, matching apt.
  - `fix(itest/homebrew)`: `bash -lc` is non-interactive, `.bashrc` early-returns and the linuxbrew shellenv never ran; replaced with `env -i` + explicit PATH; added `apply --only=brew` after each CLI mutation (brew doesn't write state from CLI like apt does); step 5 now uses hamsfile-delete + apply so removal runs once.
  - `fix(itest/vscodeext)`: tunnel `code` CLI cannot install extensions; switched to Microsoft's apt repo with a root-safe `/usr/local/bin/code` wrapper.
- [x] `/opsx:archive` — archived with `--skip-specs` (auto-sync hit the same internal header-matching bug as last cycle on tables inside MODIFIED blocks); deltas then applied to main specs manually (builtin-providers, cli-architecture, dev-sandbox, schema-design) and committed.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
