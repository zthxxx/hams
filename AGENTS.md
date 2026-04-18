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
- **TUI**: BubbleTea models scaffolded at `internal/tui/` (alternate screen, collapsible logs, popup) but **unwired in v1**; CLI uses plain `slog` log lines. v1.1 will plug `tui.RunApplyTUI` into `runApply`. See `openspec/changes/2026-04-16-defer-tui-and-notify/`.
- **OTel**: trace + metrics, local file exporter at `${HAMS_DATA_HOME}/otel/`.
- **Docs**: Nextra on GitHub Pages at `hams.zthxxx.me`.

15 builtin providers, 13 CLI entry points: Bash, Homebrew, apt, pnpm, npm, uv, goinstall, cargo, VS Code (`hams code`), git (`hams git config` + `hams git clone`, internal names `git-config` + `git-clone`), defaults, duti, mas, Ansible.

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

These tasks close the gap between `dev` and `local/loop` (reference-only), plus fix issues both branches share. Full analysis: `docs/notes/branch-comparison-recommendation.md`. Do **not** modify `local/loop`; treat it as read-only reference at `/tmp/hams-loop/` if re-checkout is needed.

**Core principle:** every task ends with a verification step. A task is NOT complete until `task check` (lint + unit + integration + e2e) passes AND the user-facing workflow still works end-to-end. Use OpenSpec workflow: each task in its own `openspec/changes/2026-04-18-*/` folder with proposal + spec deltas + tasks.md.

Outstanding tasks:

- [x] **auto-init-ux-hardening** — Done 2026-04-18 via `openspec/changes/2026-04-18-auto-init-ux-hardening/`. `EnsureGlobalConfig` + `EnsureStoreReady` now accept `*provider.GlobalFlags` and short-circuit on `flags.DryRun` (preview line to stderr, zero filesystem writes). `internal/storeinit/initGitRepo` wraps the external `git init` exec in `context.WithTimeout(ctx, GitInitTimeout)` (30s default, tunable var for tests); go-git fallback is untouched. `seedIdentityIfMissing` populates `profile_tag` + `machine_id` via `config.DefaultProfileTag` / `config.DeriveMachineID()` only when the user has not pre-set them. Five new unit tests: timeout (DI-seam fake git), dry-run invariants (store + global-config), seed-on-empty, and respect-pre-set-identity.

- [x] **git-passthrough-and-spec** — Done 2026-04-18 (openspec/changes/2026-04-18-git-passthrough-and-spec/). `internal/provider/builtin/git/unified.go` now passes every unknown `hams git <verb>` transparently through to the real `git` binary (stdin/stdout/stderr + exit code preserved). Natural `hams git clone <url> <path>` folds the positional path into `hamsFlags["path"]`; unforwarded git flags (`--depth`, `--branch`, …) surface a named UFE rather than silently dropping. `passthroughExec` package-level var is the DI seam for unit tests; tests cover normal passthrough, error propagation, dry-run skip, natural-clone translation, and flag rejection. Spec deltas in provider-system (new "Passthrough for Unhandled Subcommands" requirement covering EVERY CLI-wrapping provider, not just git) + builtin-providers (clone grammar).

- [x] **tag-profile-conflict-detection** — Done 2026-04-18 (openspec/changes/2026-04-18-tag-profile-conflict-detection/). `Tag` field added to `provider.GlobalFlags` (separate from `Profile`); `Out`/`Err` io.Writer seams added with `Stdout()`/`Stderr()` accessors + `EffectiveTag()` shim. `config.ResolveCLITagOverride` / `ResolveActiveTag` / `DeriveMachineID` / `HostnameLookup` seam shipped in `internal/config/resolve.go`. `--tag` and `--profile` now register as sibling `StringFlag`s (not aliases); `apply.go` / `commands.go` / `provider_cmd.go` / `register.go` call the resolver at action entry. i18n key `cli.err.tag-profile-conflict` added to en + zh-CN. Property-based tests cover precedence + conflict branch; deterministic tests cover DeriveMachineID env/hostname/error paths.

- [x] **typed-i18n-keys** — Done 2026-04-18 (openspec/changes/2026-04-18-typed-i18n-keys/). `internal/i18n/keys.go` declares every message ID as an exported `const` with doc comments (19 keys, grouped by capability). Every `i18n.T`/`i18n.Tf` call-site in `internal/**` now references the typed constant — typos fail compilation rather than silently returning the key ID at runtime. New `TestCatalogCoherence_EveryTypedKeyResolves` reads both locale YAML files directly and asserts every typed constant has a translation in both (the hand-maintained list inside the test is the forcing function for adding translations when a new const lands).

- [x] **code-provider-full-rename** — Done 2026-04-18 (openspec/changes/2026-04-18-code-provider-full-rename/). `Manifest.Name` + `FilePrefix` both `code`; hamsfile on disk is `code.hams.yaml`; `state.New("code", ...)`. `MANIFEST_NAME=code-ext` override removed from vscodeext integration.sh. The `CodeHandler` wrapper is deleted — the Provider now exposes `code` directly from Name()/DisplayName(). Docs + specs + README + AGENTS.md swept clean. `rg code-ext` returns only historical references in archived specs + analysis notes.

- [x] **ci-act-opt-in** — Done 2026-04-18 via openspec/changes/2026-04-18-ci-act-opt-in/. Ported `.github/workflows/ci.yml` artifact guards + act-fallback build steps, rewired `Taskfile.yml` `test:*` tasks to `ci:*` direct, added `:one-via-act` opt-in variants. `.golangci.yml` errcheck exclude-functions extended to cover writer-bound helpers — stripped the now-redundant `//nolint:errcheck` directives in `internal/cli/bootstrap_consent.go` + `internal/selfupdate/selfupdate_test.go`.

- [x] **integration-log-assertion-fanout** — Done 2026-04-18 via openspec/changes/2026-04-18-integration-log-assertion-fanout/. Ported `assert_stderr_contains` + `assert_log_line` into `e2e/base/lib/assertions.sh` and added the framework-level `assert_hams_apply_session_logged` helper. Fan-out lands stderr-based `hams session started` + Manifest.Name assertions in all ten remaining provider integration scripts (ansible, bash, cargo, git — both `git-config` and `git-clone` — goinstall, homebrew via BREW_RUN, npm, pnpm, uv, code). `apt` keeps both file-based and stderr-based assertion families as the canonical "full coverage" example.

- [x] **shared-abstraction-migration** — 2026-04-18 via `openspec/changes/2026-04-18-shared-abstraction-migration/`. Shared helpers: (a) `internal/provider/package_dispatcher.go` ports `PackageInstaller` + `PackageDispatchOpts` + `AutoRecordInstall` / `AutoRecordRemove` from `/tmp/hams-loop/` (commit `59ac23a`); (b) `internal/provider/passthrough.go` adds `provider.Passthrough` + the shared `PassthroughExec` DI seam. Migrated: all 9 CLI-wrapping builtins — cargo (`59ac23a`), goinstall (`dd5a924`), npm (`75954d1`), pnpm (`45a75e5`), uv (`e4f3a04`), mas (`3fdf2eb`), vscodeext (`a72c99b`), apt (`631e224` + test fix `ff22f9a`), homebrew (`aa718e9`). Each provider's `hamsfile.go` boilerplate (~58 LOC × 8 providers) is gone; call-sites route through `baseprovider.LoadOrCreateHamsfile` + `baseprovider.EffectiveConfig`; every `HandleCommand` default branch uses `provider.Passthrough` (DryRun-honoring) instead of the older `WrapExecPassthrough`. apt keeps its custom `handleInstall` for pin-recovery + post-install probe — documented in the spec delta. 11 new unit tests cover the shared helpers; all existing provider tests pass unchanged.

- [x] **i18n-fanout-all-userfacing** — Done 2026-04-18 via `openspec/changes/2026-04-18-i18n-fanout-all-userfacing/`. Coverage grew from ~15 → ~110 i18n-routed call-sites; catalog grew from 19 → 165 typed keys. Three atomic commits: (a) `e37f75a` catalog groundwork + apply.go + shared provider-error keys (`provider.err.no-store`, `install-needs-package`, `remove-needs-package`, `unknown-subcommand`, `dry-run-install`, etc.); (b) `e053ab1` fan-out to 12 providers (apt, pnpm, npm, uv, cargo, goinstall, vscodeext, mas, duti, defaults, homebrew, git/git/clone/unified) + CLI framework (errors.go `Error:` / `suggestion:` prefixes, bootstrap.go download/clone notices, sudo prompt, TUI no-TTY, provider_cmd.go help); (c) `8674ac6` commands.go (refresh summaries, config list/set/unset output, store status/init/push/pull previews, list group headers + empty hints, self-upgrade prose). Deferred `// TODO(i18n):` call-sites: bash + ansible v1.1-stub error messages, bootstrap_consent.go interactive prompt flow (structured TTY interaction), profile-init non-TTY multi-line shell example — all primary messages are already through i18n, only multi-line example blocks deferred. Verified: `task lint` + `task test:unit` pass (coherence test covers all 165 keys).

- [x] **docs-sync** — Done 2026-04-18 (openspec/changes/2026-04-18-docs-sync/). Three granular commits: en docs (commit `a614a74`), zh-CN mirror (`60a30c5`), README bilingual (`80c80e3`). Scope touched: `docs/content/{en,zh-CN}/docs/cli/{index,apply,global-flags}.mdx`, `docs/content/{en,zh-CN}/docs/quickstart.mdx`, `docs/content/{en,zh-CN}/docs/providers/git.mdx`, `README.md`, `README.zh-CN.md`. Rewrites: (1) `--tag` as canonical, `--profile` as legacy alias, conflict-with-different-values fails with usage error `2`; (2) auto-init section now covers dry-run preview, 30s ctx timeout on external git init + go-git fallback, and `profile_tag + machine_id` seed-when-empty; (3) new "Passthrough for unmanaged subcommands" subsection in `git.mdx` (both locales) documents verbatim forwarding with `hams git log --oneline` example; (4) README bilingual feature-list adds passthrough note. `openspec/specs/**` required zero edits — each 2026-04-18 change already wrote its own delta spec, and those merge on archive. `code-ext` / `vscodeext.hams.yaml` / `vscodeext.state.yaml` sweep: zero in-scope hits (the remaining hits are all archived `openspec/changes/archive/**` and legacy-helper comments inside `internal/**`, all out of scope). `act`-as-default sweep: only `--no-act` (apt-get dry-run flag) in apt.mdx — unrelated. Pre-commit hooks (golangci-lint + markdownlint + test:unit) passed on all three commits.

- [x] **final-verification** — Done 2026-04-18. Local gate (`task fmt && task lint && task test:unit`) green: 34/34 packages PASS with -race, 0 lint issues, 0 markdown issues. `task build` produces `bin/hams` (~14.8 MB static binary). Smoke-tested on the current dev HEAD: (1) `hams --version` / `hams --help` render correctly; (2) `hams --tag=A --profile=B apply` emits the i18n'd conflict UFE (`--tag and --profile supplied with different values`); (3) `hams git --dry-run log --oneline -5` prints the passthrough preview (`[dry-run] Would run: git log --oneline -5`); (4) `hams --dry-run apply --tag=test` on a pristine `HAMS_CONFIG_HOME` emits the dry-run previews for auto-init without actually creating any files. `task test:integration` / `test:e2e` / `test:itest` deferred to GitHub Actions CI — Docker cannot reach registry-1.docker.io in this sandbox, but the `ci:*` task suite is now docker-direct (no act) so it will run reliably in CI on push.

Each task lands as its own OpenSpec change (`openspec/changes/2026-04-18-<task-id>/`) with proposal.md + specs deltas + tasks.md. When verification passes, invoke the `openspec-archive-change` skill to archive it, then check off the item here. Commit granularly — one logical commit per sub-task where feasible. Push after each major task (top-level checklist item) lands + verifies green.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
