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

- [ ] **shared-abstraction-migration** — Actually migrate every package-like provider (apt, homebrew, cargo, goinstall, npm, pnpm, uv, mas, vscodeext) to use `baseprovider.LoadOrCreateHamsfile` / `HamsfilePath` / `EffectiveConfig`. Port `package_dispatcher.go` from `/tmp/hams-loop/internal/provider/package_dispatcher.go` into `dev`. Delete the duplicated `hamsfile.go` helper code in each provider. This closes the CLAUDE.md "design shared abstractions" requirement.
  - [ ] Port `package_dispatcher.go` + `PackageInstaller` interface into `internal/provider/`.
  - [ ] For each package provider, replace hand-written hamsfile helpers with `baseprovider` calls.
  - [ ] For each package provider, replace install/remove handlers with `AutoRecordInstall` / `AutoRecordRemove`.
  - [ ] Delete dead code; keep each provider's CmdRunner + extractor as the per-provider customization point.
  - [ ] Add a `provider.Passthrough(ctx, tool, args, flags)` helper that preserves stdio + exit code and honors `flags.DryRun`. Adopt in every CLI-wrapping provider so `hams <provider> <unhandled-verb>` defers to the real tool (closes the spec "wrapped commands MUST behave exactly like the original" gap at the first-level entry point).
  - [ ] Unit tests.
  - [ ] Verification: `task check` passes; integration tests exercise the passthrough path (e.g., `hams brew upgrade` behaves like `brew upgrade`).

- [ ] **i18n-fanout-all-userfacing** — Wrap every remaining `hamserr.NewUserError(...)` primary message and every user-visible `fmt.Print*` call with `i18n.T` / `i18n.Tf`. Audit scope: ~50 NewUserError + ~100+ fmt.Print call-sites. Add missing keys to `internal/i18n/locales/en.yaml` + `zh-CN.yaml`. Log records do NOT need i18n.
  - [ ] Grep audit: list every `hamserr.NewUserError(` and every user-facing `fmt.Print*` call-site.
  - [ ] Replace literals with typed i18n keys + add translations.
  - [ ] Enforce via a custom golangci-lint check or a unit test that scans AST for literal strings in the whitelisted call-sites.
  - [ ] Verification: `task check` passes; catalogue-coherence test covers every new key.

- [ ] **docs-sync** — Grep all `docs/content/**` + `README*.md` + `AGENTS.md` + `openspec/specs/**` for stale references (`code-ext`, `git-config`, `git-clone`, `vscodeext.hams.yaml`, `--profile` as canonical, `act` as default integration path). Rewrite to reflect the shipped `dev` state after all previous tasks land.
  - [ ] en docs sweep.
  - [ ] zh-CN docs sweep.
  - [ ] AGENTS.md / README.md / README.zh-CN.md sweep.
  - [ ] openspec/specs/** sync (delta specs from change folders merge here on archive).
  - [ ] Run `docs/` verification process (`.claude/rules/docs-verification.md`) — build site + playwright smoke.
  - [ ] Verification: `pnpm build` in `docs/` succeeds; no broken links.

- [ ] **final-verification** — After all above land, run the full `task check` + manual restore-from-store flow (fresh container, `curl | bash`, `hams apply --from-repo=<local-test-repo> --tag=macOS`) end-to-end. Confirm zero interactive prompts, all providers apply, state records correctly.

Each task lands as its own OpenSpec change (`openspec/changes/2026-04-18-<task-id>/`) with proposal.md + specs deltas + tasks.md. When verification passes, invoke the `openspec-archive-change` skill to archive it, then check off the item here. Commit granularly — one logical commit per sub-task where feasible. Push after each major task (top-level checklist item) lands + verifies green.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
