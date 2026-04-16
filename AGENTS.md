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

### Completed in cycle 1 (initial audit)

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

### Cycle 130 — Direct tests for `matchesPlatform`

- [x] `matchesPlatform` had no direct tests — only exercised transitively via DAG resolution and RunBootstrap dep-filter paths. A regression flipping the empty-string=all semantic would silently drop every unfiltered `DependsOn` entry from bootstrap. 3 tests: wildcards (empty + PlatformAll), current runtime.GOOS match, bogus-GOOS false. Mirrors cycle 109's `IsPlatformsMatch` gate but at the dep-level instead of manifest-level. (commit `5e55492`)

### Cycle 129 — Populate dev-sandbox spec Purpose + finish cycle 127 race fix

- [x] Two fixes. (1) `openspec/specs/dev-sandbox/spec.md` had literal `"TBD - created by archiving change dev-sandbox. Update Purpose after archive."` as the Purpose — a spec artifact defect from the archive workflow. Wrote proper Purpose explaining why dev-sandbox exists (host-safe reproducible environment for `hams apply` end-to-end testing, filling the gap between unit tests and CI-driven Dockerized integration tests). (2) Cycle 127's captureStdout mutex wasn't enough on its own — `TestCloneRemoteRepo_CanceledContextAborts` passes `os.Stdout` to go-git's `PlainCloneContext`'s `Progress` field (a READ of the global), and another Parallel test's `captureStdout` swap (WRITE) raced. Removed `t.Parallel()` from that test since it genuinely requires a stable `os.Stdout`. task check green. (commit `28bbeac`)

### Cycle 128 — selfupdate `IsUpToDate` version-comparison edges

- [x] Expanded `IsUpToDate` test coverage to 4 edges that matter for real user workflows: (1) current newer than latest (dev build ahead of stable) — `>=` semantics must hold so `self-upgrade` doesn't downgrade; (2) pre-release/build-metadata stripped (`1.0.0-rc1` vs `1.0.0` treated as equal, per the stripping comment) so `self-upgrade` doesn't churn on every rc tag; (3) different dot depths (`1.0` vs `1.0.0`) treated equal (missing parts default to 0); (4) non-numeric fallback (`dev` vs `dev` string-equal; `dev` vs `1.0.0` not up-to-date). 4 regression tests. selfupdate coverage: 76.4% → 79.2%. (commit `2506e1a`)

### Cycle 127 — Dedupe pluralize; fix captureStdout/captureStderr race

- [x] Two cleanups while verifying cycle 126. (1) `hams list` had inline `noun := "resources"; if count == 1 { noun = "resource" }` that duplicated the `pluralize` helper from cycle 125 — replaced. Drops 3 lines. (2) `captureStdout`/`captureStderr` swapped the global `os.Stdout`/`os.Stderr` without synchronization; two `t.Parallel()` tests calling either helper concurrently raced on that global. Go's `-race` detector flagged it during the task-check run after cycle 126. Added per-function mutexes (`captureStdoutMu`, `captureStderrMu`) to serialize the swap, and switched from `t.Cleanup` to `defer` inside the locked section so the restore happens while the lock is held (otherwise Cleanup would run AFTER the unlock and another test could race-restore first). (commit `9e95db4`)

### Cycle 126 — Dry-run text output uses correct singular grammar

- [x] Extension of cycle 125's pluralize helper to the dry-run preview output (`printDryRunActions`). Two lines had the same bug: `"no changes (1 resources already at desired state)"` and `"(1 resources unchanged)"`. Both now branch on count via `pluralize`. 3 regression tests cover singular (count=1), plural (count=2), and the mixed-install+skip case (`(1 resource unchanged)` after install lines). cli coverage: 67.8% → 68.8%. (commit `a615597`)

### Cycle 125 — `hams refresh` summary uses correct singular grammar

- [x] Mirror of cycle 36 (which fixed the same bug in `hams list`). `hams refresh` printed `"Refresh complete: 1 providers probed"` for single-provider runs — grammatically wrong. Extracted a `pluralize(count, singular, plural)` helper and applied to all 3 refresh-summary variants (happy path, save-failure path, probe-failure path) so each emits "provider" vs "providers" based on count. Regression test `TestRunRefresh_SingularProviderNoun` seeds a one-provider refresh and asserts the output contains `"1 provider probed"` AND does NOT contain `"1 providers probed"`. cli coverage: 67.4% → 67.8%. (commit `227830c`)

### Cycle 124 — `hams config list` now surfaces `store_repo`

- [x] Real UX bug. `hams config list` output omitted `store_repo` in both text and JSON formats. A user who set it via `hams config set store_repo github.com/zthxxx/hams-store` and then ran `config list` to verify saw NO mention of the value — only `config get store_repo` would retrieve it. This half-implementation made `store_repo` feel second-class compared to the other config fields all visible in list. Fix: add `store_repo` to the JSON object AND the text-format print loop. Text only emits the "Store repo:" line when `StoreRepo` is non-empty so fresh-install output stays tight. Regression test `TestConfigList_IncludesStoreRepo` seeds a config and asserts both formats contain the value. cli coverage: 65.5% → 67.4%. (commit `23013d0`)

### Cycle 123 — apt `pinStateOpts` direct tests for pin-format detection

- [x] `pinStateOpts` had 40% coverage — 3 branches (version-pin via `pkg=ver`, source-pin via `pkg/src`, bare-no-pin) plus the "explicit empty-version unpin" special case. A regression here would drop pins from state, so the next apply couldn't tell "installed from apt stable" vs "installed with an explicit version pin". 4 table-driven tests cover each branch including the `nginx=` empty-value case that represents "explicit unpin" (matches `strings.CutPrefix` semantics). apt coverage: 76.5% → 77.6%. (commit `f066910`)

### Cycle 122 — ansible HandleCommand usage-error + dry-run tests

- [x] `ansible.HandleCommand` had 0% coverage. Both the empty-args UserFacingError path and the dry-run preview path are trivially testable without exec-ing real ansible-playbook. Gate against the same class of bug as cycle 118's self-upgrade dry-run gate: a future refactor that drops the dry-run branch would silently run playbooks against the host. ansible coverage: 76.9% → 83.3%. (commit `d74114e`)

### Cycle 121 — `cloneRemoteRepo` honors context; Ctrl+C aborts clone promptly

- [x] Real UX bug, same class as cycle 19 (context propagation through provider handlers) but at the bootstrap layer. `cloneRemoteRepo` used `gogit.PlainClone` — the no-ctx variant — so `Ctrl+C` during a `hams apply --from-repo=<user/repo>` clone appeared to hang until the network TCP round-trip timed out (can be minutes). Root ctx already cancels on SIGINT (cycle 12's `signal.NotifyContext`), but the clone path didn't plumb it through. Fix: thread ctx through `runApply → resolveFromRepoStorePath → bootstrapFromRepo → cloneRemoteRepo → PlainCloneContext` / `PullContext`. Updated 2 pre-existing tests that called `bootstrapFromRepo` directly to pass `context.Background()`. Regression test `TestCloneRemoteRepo_CanceledContextAborts` pre-cancels the ctx and asserts the function returns a non-nil fast-fail error without going through the full network round-trip. cli coverage: 64.8% → 65.5%. (commit `53f40a6`)

### Cycle 120 — Direct tests for `isLocalPathAttempt` classification

- [x] `isLocalPathAttempt` decides whether `--from-repo=X` is a local path or GitHub shorthand — a classification error sends typo'd local paths down the clone branch (producing misleading "Repository not found" errors for what is actually a local typo), or vice versa. Coverage was 40% with no direct tests. Added 6 tests covering each rule: absolute `/` prefix (local even if nonexistent), `~/` prefix, relative `./`/`../` prefixes (gates against a future refactor that drops these in favor of stat-only), GitHub `user/repo` shorthand (not local), full URL `https://...` (not local), bare-name-that-stats-to-dir (local), bare-name-with-no-stat-hit (not local — falls through to clone path where cycle 72's friendly error surfaces). cli coverage: 64.6% → 64.8%. (commit `f6ca3b0`)

### Cycle 119 — `hams list` text output shows LastError for failed resources

- [x] Third in the cycles 116-119 list UX sweep. Text output previously showed `htop  failed` with no indication of WHY — user had to run `--json` or read the state YAML to see the last_error text. Now: `htop  failed (error: package not found in repository)`. The `(error: ...)` suffix is distinctive (grep/sed friendly) and only emits when LastError is non-empty so healthy rows stay compact. Regression test seeds a failed apt resource with last_error text and asserts the suffix appears. cli coverage: 64.5% → 64.6%. (commit `f4208fa`)

### Cycle 118 — self-upgrade dry-run branch regression gate

- [x] `runHomebrewUpgrade`'s dry-run branch had zero tests — a future refactor dropping the branch would silently run `brew upgrade` on a user who only asked for a preview. Same "dry-run has side effects" anti-pattern that cycles 39, 41, 84, 86 fixed in apply. Added `TestRunHomebrewUpgrade_DryRun`: asserts dry-run returns nil, stdout contains `"[dry-run] Would run: brew upgrade"`, AND stdout does NOT contain the live-run text "Detected Homebrew install" — if the dry-run branch accidentally falls through to the real exec path, the assertion fails. cli coverage: 64.2% → 64.5%. (commit `41c5ddb`)

### Cycle 117 — `hams list` text output shows Value for KV-Config

- [x] Sister fix to cycle 116 (which covered the JSON path). `hams list` text output was `<id>  <status> <version>` — for KV-Config providers (defaults/duti/git-config), Version is always empty, so a user who just ran `hams git-config set user.name zthxxx` then `hams list` saw `user.name=zthxxx  ok` with NO way to see the actual stored value without reading the state YAML. Fix: branch on which field is populated — Package rows keep the `<version>` suffix; KV-Config rows emit an `= <value>` suffix. The equals prefix is distinctive so the two classes visually separate in mixed output. Regression test seeds a git-config resource with value=zthxxx and asserts `= zthxxx` in the text output. cli coverage: 63.3% → 64.2%. (commit `bb7a90f`)

### Cycle 116 — `list --json` exposes Value + LastError

- [x] Real user-workflow gap. `hams list --json` only emitted `provider/name/id/status/version` — two script-relevant fields were hidden: (1) `value` for KV-Config-class resources (defaults/duti/git-config), the actual config value that's often WHY the user ran `list` (e.g. "what's my git user.name right now?"); (2) `last_error` for failed / hook-failed resources, so unattended-apply scripts can machine-parse the failure text without grep-ing slog. Both added as `omitempty` so existing consumers see no noise (Package rows get no empty `value`, healthy rows get no empty `last_error`). Regression test seeds one ok + one failed git-config resource and asserts both fields render AND that omitempty keeps `last_error` out of the ok row. cli coverage: 62.5% → 63.3%. (commit `af5f45a`)

### Cycle 115 — End-to-end gate for StateHookFailed on post-hook failure

- [x] Initially suspected a real bug: `executor.go`'s post-hook failure path had a comment "state is hook-failed" but the code didn't call `sf.SetResource(…, StateHookFailed, …)`, suggesting the state file would stay at `StatePending` from the pre-execution mark. Investigation revealed the internal `runPostHooks` (in `hooks.go`) already sets the correct state at the point of hook failure — the executor's handler intentionally doesn't duplicate the write. The executor path's comment was accurate; I misread it first. But the end-to-end behavior had ZERO tests gating it — if a future refactor accidentally removed the inner `sf.SetResource`, the state would silently revert to `StatePending` and confuse both `hams list` and auditors. Added `TestExecute_PostInstallHookFailure_SetsStateHookFailed` that wires a post-install hook that runs `false` and asserts: Install counter incremented, result.Errors captures the hook text, state ends at StateHookFailed (not pending/ok), LastError preserves the failure text. Updated the executor comment to explicitly point at hooks.go as the single source of truth. provider coverage: 74.8% → 75.2%. (commit `ae0d9fd`)

### Cycle 114 — Executor log messages: broken English verb tenses

- [x] Real UX bug visible in every `hams apply` run. `executor.go` built phase log messages via naive string concat: `slog.Info(phase+"ing", ...)` produced `"installing"` / `"updateing"` / `"removeing"`; `slog.Info(phase+"d", ...)` produced `"installd"` / `"updated"` / `"removed"`. So `installd`, `updateing`, and `removeing` shipped into every log line — not English, not greppable. Ops runbooks and docs use the correct English forms; an operator grep-ing for `"installing"`, `"removing"`, `"updating"` would miss every hams apply message. Fix: two lookup helpers (`phaseGerund`, `phasePastTense`) hard-code the English spellings for known phases with a fallback to the naive concat for future additions. 3 regression tests gate the spellings. Visible in the `task check` test output before the fix: `msg=installd provider=brew resource=git` and `msg=removeing provider=brew resource=git`. After: consistent installing/installed, updating/updated, removing/removed. provider coverage: 74.4% → 74.8%. (commit `968d224`)

### Cycle 113 — `AtomicWrite` error + edge-case coverage

- [x] Test-design cycle. `AtomicWrite` had two untested error branches and one untested edge case that all map to real user workflows. (1) Parent-is-a-file: a user typos a profile path as a filename, or a prior step wrote a file where a directory was expected — `MkdirAll` fails; `AtomicWrite` must surface a "creating directory" error, not silently continue. (2) Empty-data: writing zero bytes after `RemoveApp` drains a hamsfile must persist as an explicit empty-file marker, not error out (so subsequent reads see an empty document, not "file not found"). (3) Overwrite: the atomic rename must completely replace existing content with no hybrid state (regression gate if someone "optimizes" to in-place write). 3 regression tests. hamsfile coverage: 82.5% → 82.9%. (commit `5c3fd9f`)

### Cycle 112 — Lock corruption + missing-file error-shape tests

- [x] Small test-design cycle. `Lock.Acquire`'s corrupt-lock branch and `Lock.Read`'s missing-file + malformed-YAML branches were both untested. These are load-bearing error paths: Acquire's corruption branch keeps hams from silently overwriting a lock whose YAML is corrupt (the user needs to inspect/remove it manually), and Read's corruption branch distinguishes "stale from crash" vs "genuinely corrupt" for upstream callers that handle each differently. Added 3 regression tests: `TestLock_UnreadableLockFileErrors` (Acquire against corrupt YAML errors AND names the lock path), `TestLock_Read_MissingFile` (returns os.IsNotExist-wrapped for errors.Is), `TestLock_Read_MalformedYAMLSurfacesParseError` (mentions "parsing" in the error text). state coverage: 84.1% → 86.7%. (commit `9286de0`)

### Cycle 111 — `ComputePlan` now dedups duplicate desired entries

- [x] Real user-workflow bug. `ComputePlan` iterated the raw desired slice without a dedup guard. A common drift case — the same app listed under two hamsfile tags after a move-between-tags edit (e.g. moved `htop` from `cli:` to `dev:` but forgot to delete the old entry) — made `ListApps` return `[htop, htop]`, so ComputePlan emitted two `Install` actions for the same ID. Apply then ran `apt install htop` twice (idempotent but wasteful) AND the final summary showed `installed=2` instead of 1. Fix: add a `seen` set guard in the install-path loop; first-occurrence order is preserved so hooks and dry-run preview stay deterministic. The removal-path was already dedup-safe (iterates `desiredSet`, a map). 2 regression tests (exact-count + first-occurrence order). provider coverage: 74.2% → 74.4%. (commit `4a1b3dd`)

### Cycle 110 — Direct tests for `filterProviders` + `parseCSV`

- [x] `filterProviders` and `parseCSV` were only exercised transitively through apply/refresh command tests — no direct unit tests. A silent change to the CSV trimming behavior (e.g. a future "optimize" that loses empty-skip) would slip past existing coverage. Added 9 direct tests covering the full contract surface: both-flags-empty short-circuit, `--only`/`--except` mutual exclusion, only-filters-down, except-filters-out, case-insensitive matching, whitespace-part trimming, unknown-name usage errors for both flags (asserted via `errors.As` on the `UserFacingError` to inspect the suggestions list, since `Error()` returns only the top-line message), and `parseCSV` whitespace + empty-part handling. Test harness introduces a minimal `namedProvider` fake — only `Manifest()` is consulted by the filter, other Provider methods are no-op stubs. cli coverage: 60.6% → 62.5%. (commit `ec0526b`)

### Cycle 109 — Direct tests for IsPlatformsMatch + HookSet.HasAny

- [x] Small coverage cycle. Both functions sat at 66.7% with no direct tests — only indirectly exercised through provider dispatch and hook execution. Added explicit branch coverage for `IsPlatformsMatch` (empty/nil is wildcard, `PlatformAll` is wildcard, empty-string Platform is wildcard, non-matching platform returns false) and `HookSet.HasAny` (nil pointer safe, empty set false, PreInstall-only true, PostUpdate-only true). These test-gate subtle contracts — e.g., a future change that inverts the "empty platforms = all" default would silently hide every no-platform-filter provider from dispatch, but now fails the test suite instead. provider coverage: 73.6% → 74.2%. (commit `f217b18`)

### Cycle 108 — `hams store push` UX: empty-commit skip + `-m` flag

- [x] Real user-workflow bug, two-in-one. (1) Running `hams store push` right after `hams refresh` (which only touches `.state/` files, all gitignored) failed with git's `"nothing to commit, working tree clean"` bubbling through as an exec-exit-1 error — the happy path was an error. (2) Every commit was hardcoded to `"hams: update store"` — zero audit value. Fix: a `storePushRunner` DI seam routes all four git steps through an interface (`Status`, `AddAll`, `Commit`, `Push`). Before adding, `runStorePush` checks `git status --porcelain`; empty output short-circuits to a friendly "Nothing to commit — the store is clean. Skipping commit+push." and exits zero. `-m` / `--message` flag lets the user pass a real commit message (defaults to the old `"hams: update store"` for backward compat). 6 unit tests cover clean-tree skip, dirty-tree happy path + message forwarding, and each of the 4 error branches short-circuiting later steps. Previously 0% unit coverage on store-push because every path shelled out to real git; now fully DI-testable. cli coverage: 60.3% → 60.6%. (commit `6841eb7`)

### Cycle 107 — Bark notification URL segments `url.PathEscape`'d

- [x] Real user-workflow bug. `barkChannel.Send` only replaced spaces in title/message (`strings.ReplaceAll(..., " ", "%20")`). Any `#`, `?`, or `/` in the payload would be interpreted by HTTP clients as a URL fragment separator, query delimiter, or path segment — truncating / splitting the notification silently. Concrete trigger: a message like `"installed 5, failed 1 (#issue-42)"` loses everything after `#`. Fix: pass each path segment through `url.PathEscape` (encodes `#`/`?`/`/` but preserves RFC-3986 sub-delims like `&`/`=` that are valid in path segments). Introduce overridable `barkBaseURL` package var so tests can point at `httptest.NewServer` instead of `api.day.app` — the Send path was previously 0% covered because no DI seam existed. 4 new HTTP-roundtrip tests: happy path asserts URL shape, URL-escaping regression-gate asserts `%23` and `%2F` appear AND raw `#`/`?`/`/` do NOT, non-200 surfaces an HTTP-code error, unreachable host surfaces a network error. notify coverage: 60% → 96%. (commit `561a8be`)

### Cycle 106 — Property-based tests for 5 providers + .gitignore fix

- [x] Test-design cycle. Per CLAUDE.md testing convention ("property-based > example-based"), 5 providers lacked property tests: git, defaults, duti, homebrew, goinstall. Added ~20 rapid-based property tests across parser helpers that production feeds arbitrary input to (state-file IDs after merge conflicts, brew stdout after schema changes, user CLI args with random flag orderings). Properties: no-panic on arbitrary input, idempotency, prefix/round-trip invariants, fail-closed on malformed input. Hit one real bug in MY own property-test design (joined-lines vs split-lines mismatch caught by rapid's shrinker: `["A\nA"]` joined then parsed differs from pre-join expectation) — demonstrates property tests catching issues example tests miss by construction. Also: `.gitignore` had `testdata/rapid/` without `**/` prefix, so it only matched at repo root — `internal/provider/builtin/duti/testdata/rapid/` was tracked. Fixed by prefixing with `**/`. homebrew coverage: 67.5% → 68.2%. (commits `77c3c04`, `ac25b61`)

### Cycle 105 — Dead code cleanup + stringer tests

- [x] Maintenance cycle. `Registry.All()` had zero callers anywhere in the codebase (grep across `internal/`, `pkg/`, `cmd/`, all `*.go` including tests). Deleted per "continuous garbage collection" rule. Added tests for `ResourceClass.String`, `ActionType.String`, and `BootstrapRequiredError.Error` / `Unwrap` — these implement user-facing formatting that the consent flow and dry-run preview rely on being stable, but previously sat at 0% coverage, so a rename would have silently garbled log messages without any test catching it. `errors.Is` / `errors.As` round-trip also asserted. provider coverage: 70.2% → 73.6%. (commit `95565e0`)

### Cycle 104 — git-config `set`/`remove`/`list` verb routing per spec

- [x] Real user-workflow bug caught by comparing spec vs implementation. `builtin-providers.md §git-config` documents four CLI shapes (`set <key> <value>`, `remove <key>`, `list`, plus bare `<key> <value>`), but the implementation only accepted bare — so a user typing `hams git-config set user.name zthxxx` (the canonical form per spec/docs) hit a cryptic "scope.key=value" error because "set" was parsed as a key. `remove` and `list` were also broken. Fix: `HandleCommand` now routes on the first arg — `set <key> <value>` → doSet (same semantics as bare, kept for backward compat), `remove <key>` → `runner.UnsetGlobal` + delete matching hamsfile entry + `StateRemoved` tombstone (tombstone-only when no prior entry, so future apply won't re-assert a stale value), `list` → prints desired-vs-observed diff. Usage error now lists all four shapes. 8 new U-series tests (U10-U17: set verb happy path, set wrong arity, remove verb deletes + marks state, remove without prior entry tombstone, remove wrong arity, set dry-run, remove dry-run, list runs clean). git coverage: 53.3% → 59.7%. (commit `5ba5171`)

### Cycle 103 — duti CLI auto-records `<ext>=<bundle-id>`

- [x] Real user-workflow bug, third in the auto-record-gap sweep (cycle 101 git-config, 102 defaults, 103 duti). `hams duti pdf=com.adobe.acrobat.pdf` exec'd the `duti` binary directly (bypassing the `CmdRunner` DI seam) and never recorded the association to hamsfile/state. Fix: `HandleCommand` recognizes the canonical single-arg `<ext>=<bundle-id>` shape, routes through `p.runner.SetDefault`, and auto-records `{app: "<ext>=<bundle-id>"}` + `StateOK` (with bundle-id as value). Same-ext different-bundle-id invocations replace the stale entry in place. Raw duti flags (e.g. `-s <bundle> .pdf all`) still pass through to exec for power-user escape-hatch access without touching hamsfile/state. `Provider` struct gains `*config.Config` field; `register.go` and `bootstrap_invariant_test.go` updated for the signature change. 9 U-series tests (U1 no-args, U2 dry-run, U3 happy path, U4 same-ext replacement, U5 runner error short-circuit, U6 independent exts, U7 `--hams-local`, U8 raw passthrough skips record, U9 malformed resource ID). duti coverage: 82.1% → 87.0%. (commit `9885718`)

### Cycle 102 — defaults CLI write/delete auto-record

- [x] Real user-workflow bug, same class as cycle 101. `hams defaults write com.apple.dock autohide -bool true` exec'd the `defaults` binary directly (bypassing the `CmdRunner` DI seam) and only attempted to set a `preview-cmd` on hamsfile entries that ALREADY existed (via `hamsfile.Read`, not `LoadOrCreateEmpty`). Result: a fresh `hams apply` on another machine wouldn't reproduce the setting. `hams defaults delete` wasn't wired at all. Fix: `HandleCommand` now splits on the verb — `write` / `delete` route through `p.runner.Write` / `p.runner.Delete` (DI seam) and auto-record; other verbs (e.g. `read`) pass through to exec without bookkeeping. Write records `{app: "domain.key=type:value"}` with `preview-cmd: defaults write …` to the hamsfile and `StateOK` (with value) to state. Same-key-different-value invocations replace the stale entry in place. Delete removes the matching hamsfile entry and marks the state resource `StateRemoved`; a delete with no prior hamsfile entry still records a `StateRemoved` tombstone for audit / future-apply semantics. Runner errors short-circuit the record path so the hamsfile never claims a setting `defaults` rejected. 10 U-series unit tests (U1 no-args, U2 dry-run, U3 write happy path, U4 same-key replacement, U5 write error short-circuit, U6 delete round-trip, U7 delete tombstone without prior write, U8 read passthrough doesn't record, U9 `--hams-local` routing, U10 preview-cmd round-trip). defaults coverage: 60.4% → 81.9%. (commit `ae9fa4e`)

### Cycle 101 — git-config auto-records to hamsfile + state

- [x] Real user-workflow bug surfaced by reading the spec against the code. `hams git-config user.name zthxxx` ran `git config --global` on the host but never persisted the entry to the hamsfile/state. Result: a fresh `hams apply` on another machine wouldn't reproduce the setting — breaking the "CLI-first, auto-record" contract that git-config is supposed to embody (spec `builtin-providers` §git-config). Fix: introduce `git.CmdRunner` DI seam for all outbound `git config` calls (Apply, Probe, Remove AND HandleCommand); `HandleCommand` now writes `{app: "user.name=zthxxx"}` to the hamsfile and a `StateOK` resource to state after the git call succeeds. A re-run with a DIFFERENT value for the SAME key replaces the old entry in place (old → `StateRemoved`, new → `StateOK`) so the hamsfile stays single-valued per key, matching git's overwrite semantics. Runner errors short-circuit the record path so the hamsfile never claims a setting git didn't accept. `--hams-local` routes the entry into the `.hams.local.yaml` variant, mirroring apt. 9 unit tests (U1 no-args, U2 one-arg, U3 dry-run, U4 happy path, U5 idempotent re-run, U6 same-key different-value replacement, U7 runner error short-circuit, U8 independent keys coexist, U9 `--hams-local` routing) mirror the apt U-series; existing `git_apply_test.go` integration-style tests continue to exercise the real runner. git-config coverage: 1.8% → 53.3%. (commit `2f1bc6f`)

### Cycle 100 — `--from-repo` and `--store` now mutually exclusive

- [x] Real user-workflow bug caught by manual end-to-end test. `hams apply --from-repo=X --store=/my/path` silently honored only `--from-repo` — the clone always lands in `${HAMS_DATA_HOME}/repo/<user>/<name>/`, never the user's `--store` path. User intent with both flags is genuinely unclear (redirect clone? prefer local path?), so rather than pick a winner and silently drop the other, hams now errors with `ExitUsageError` naming both flags + the actual clone location. Matches the existing `--bootstrap`/`--no-bootstrap` and `--only`/`--except` mutually-exclusive precedents. Regression test `TestRunApply_FromRepoAndStoreAreMutuallyExclusive` asserts the error shape and that `${HAMS_DATA_HOME}` is surfaced in the suggestions. (commit `5380007`)

### Cycle 99 — ATTEMPTED, REVERTED — empty hamsfile was not a bootstrap-skip signal

- [~] Tried to treat empty hamsfiles (`cli: []`) as "no hamsfile present" so bootstrap wouldn't demand brew on a machine with no brew entries. But the existing 4 bootstrap tests asserted the opposite contract: a committed placeholder hamsfile IS intentional — it signals "this provider is used on this profile, even if no packages yet". Revert was correct. Lesson: when a fix breaks existing tests, read the tests first — they're encoding the intentional behavior.

### Cycle 98 — hamsfile.ListApps filters empty/whitespace entries

- [x] Real user-workflow bug caught by manual end-to-end test. A malformed hamsfile entry like `- app: ""` or `- app: "  "` (from git merge conflict residue, accidental edit, or yaml round-trip) would flow through `ListApps` → `ComputePlan` → `install ""` / `install <spaces>` on apt/brew/etc. Shell errors blamed the package manager, not the hamsfile. Fix: `hamsfile.ListApps` now `strings.TrimSpace`s the value before appending and skips empty-after-trim values silently. All callers (Plan, CLI install paths, `hams list`) inherit the fix. Regression test `TestListApps_SkipsEmptyAndWhitespaceEntries`. (commit `9c13d3b`)

### Cycle 97 — homebrew handleInstall/Remove on CmdRunner seam + U1-U7 tests

- [x] Follow-up to cycle 96. The state-write feature landed but lost coverage (57.8% → 54.8%) because homebrew's `handleInstall` shelled out via `provider.WrapExecPassthrough`, which isn't DI-testable. Fix: switch `handleInstall` and `handleRemove` to drive `p.runner.Install` / `p.runner.Uninstall` per-package (same shape as cargo/npm/pnpm/uv/goinstall/mas/vscodeext); `--cask` flag now routes `isCask=true` through the runner. Extracted `tagCask = "cask"` const (goconst). Added 7 `TestHandleCommand_U*` tests mirroring apt's U1-U7: install records both hamsfile+state, idempotent re-install, install-failure leaves both untouched, remove deletes+marks state removed, remove-failure preserves both, dry-run skips everything, `--cask` routes under "cask" tag AND forwards `isCask=true`. Coverage recovers and tops baseline: 54.8% → 67.5%. (commit `3e06b89`)

### Cycle 96 — `hams brew install` now writes state (not just hamsfile)

- [x] Closes the homebrew half of CP-1's auto-record gap. `hams brew install git` (cycle 52's work) wrote `Homebrew.hams.yaml` but NOT `Homebrew.state.yaml`, so `hams list --only=brew` returned empty immediately after install because list reads state only — users had to run `hams refresh` to see the resource. Fix: added `statePath` + `loadOrCreateStateFile` helpers (line-for-line port of apt's U12-U15 pattern), updated `handleInstall`/`handleRemove` to load both files, `SetResource(pkg, StateOK/StateRemoved)`, and `Write + Save` in order. Coverage dipped 57.8% → 54.8% because the state-write branch is not yet DI-testable — homebrew's `handleInstall` still uses `provider.WrapExecPassthrough` instead of the CmdRunner seam; switching it is a clean follow-up that lets apt-style U1-U5 tests exercise the path. (commit `0d08ae7`)

### Cycle 95 — provider arg parser handles `--flag=value` forms symmetrically

- [x] Follow-up to cycle 94 (which fixed the top-level error path). The same bug class lived in `parseProviderArgs` and `stripGlobalFlags`: only bare `--json` / `--debug` / `--dry-run` / `--no-color` matched. So `hams apt --json=true install foo` leaked `--json=true` into passthrough, which `ParseVerb` then treated as the verb, which fell through to apt-get as `--json=true` — apt-get rejected it with "option is not understood". Fix: new `boolFlagMatch(arg, flag, *target)` helper that consumes bare, `=true`, `=1`, `=false`, `=0` forms. Replaces the four existing case branches in both parsers so the two entry points stay in lockstep. Regression test `TestParseProviderArgs_BoolFlagEqualsForm` — 7 table-driven cases covering all forms plus the "unknown --flag=value stays in passthrough" invariant. (commit `5847a8e`)

### Cycle 94 — `--json=true` / `--json=false` now parsed at error boundary

- [x] Real user-workflow bug caught by manual end-to-end test. urfave/cli accepts all of `--json`, `--json=true`, `--json=false`, `--json=1`, `--json=0` for BoolFlag. But the top-level `Execute()` error path in `root.go` scans `os.Args` directly (urfave's parsed value is unreachable from the os.Exit path) and only matched the bare `--json` form. Result: `hams --json=true apply --store=/nope` silently emitted the text error instead of the JSON object users expect from scripts. Fix: extracted `hasJSONFlag(args)` helper handling all five forms with right-wins semantics (matching urfave/cli). Nine-case table-driven test `TestHasJSONFlag_AllForms` covers the full grid including "embedded in other args doesn't match" (e.g., `--jsonx` negative case). `internal/cli` coverage: 59.1% → 59.7%. (commit `ac20935`)

### Cycle 93 — `hams refresh --profile=<typo>` symmetric validation

- [x] Follow-up to cycle 92. Same silent-no-op-on-typo bug existed in refresh: `hams --profile=Typo refresh` printed "No providers match" + exit 0. Fix mirrors apply: when `flags.Profile` is explicit, stat `<store>/<profile>`; return `ExitUsageError` if missing. Regression test `TestRunRefresh_ExplicitProfileNotFoundEmitsUserError`. (commit `c749869`)

### Cycle 92 — Explicit `--profile=<typo>` errors cleanly instead of silent skip

- [x] Real user-workflow bug symmetric to cycle 87's store_path validation. `hams --profile=Linux apply` when `<store>/Linux` doesn't exist used to print `"No providers match: no hamsfile or state file present..."` + exit 0, indistinguishable from a genuinely empty profile. A typo like `Linux` vs `linux` or `Macos` vs `macOS` silently became a no-op. Fix: when `flags.Profile` is explicitly set (via CLI flag), `os.Stat` `<store>/<flag>` and return `UserFacingError{Code: ExitUsageError}` if missing. The check fires ONLY for the explicit-flag case, NOT when profile_tag comes from hams.config.yaml — users shouldn't be forced to create empty profile dirs just to run apply. Two regression tests: `TestRunApply_ExplicitProfileNotFoundEmitsUserError` (strict path) and `TestRunApply_ConfigProfileSilentlyEmptyIsNotAnError` (lenient path). Also updated cycle-75's `TestRunApply_NonTTYWithProfileFlagButNoMachineID` to stub the profile dir (was relying on the pre-cycle-92 no-op behavior). (commit `c41812a`)

### Cycle 91 — Promote `--store` override to `config.Load` (fix all callers at once)

- [x] Follow-up to cycle 90. The fix in `runRefresh` worked but only for that one caller; the same pattern (`flags.Store` needing to override `cfg.StorePath` from the merged config) applies to every `config.Load` callsite — `listCmd`, `storeCmd`, `configCmd` get/set/list/edit, and `register.loadBuiltinProviderConfig`. Fixing them one-by-one would drift again next time someone adds a new caller. Fix: capture the original `storePath` argument in `config.Load` as `explicitStoreOverride` and apply it AFTER the level 2-4 merges. Semantics: "non-empty `storePath` wins over config's `store_path`". The cycle-90 ad-hoc patch in `runRefresh` is now dead code — replaced with a NOTE comment pointing at the central fix. Regression test `TestRunRefresh_FlagStoreOverridesConfig` (cycle 90) still asserts the invariant; it now also implicitly guards every other `config.Load` caller. (commit `ddb157e`)

### Cycle 90 — `hams refresh --store` now actually overrides config `store_path`

- [x] Real user-workflow bug caught while manually verifying cycle 88's validation: `hams refresh --store=/alt` silently refreshed the config-derived store instead of /alt. Root cause: `config.Load` only populates `cfg.StorePath` from the `storePath` arg when `cfg.StorePath` was initially empty. If the global config already had `store_path: /tmp/store`, cfg.StorePath stayed /tmp/store and the --store flag was effectively ignored for refresh (which uses cfg.StorePath). apply dodged the bug because it uses `flags.Store` directly via `storePath := flags.Store` at the top of `runApply`. Fix: after `config.Load` in `runRefresh`, explicitly overwrite `cfg.StorePath` from `flags.Store` when set. One line, matches the user-expected precedence "--store always wins". Regression test `TestRunRefresh_FlagStoreOverridesConfig` (global config has a valid store_path, --store points at a non-existent dir, assert refresh surfaces the --store path in the error — proving flag precedence). internal/cli coverage 59.0% → 59.1%. (commit `46e5c74`)

### Cycle 89 — `--config=~/path` / `--store=~/path` now expands tilde

- [x] Real user-workflow bug caught by manual end-to-end test. `hams --config=~/my.yaml` and `hams --store=~/my-store apply` produced the same silent-fallback-to-defaults behavior as cycle 85's `store_path: ~/…` in hams.config.yaml — shells do NOT expand `~/` inside `--flag=~/...` syntax (tilde expansion only fires at the start of a separate argument), so the literal `~/my.yaml` reached `paths.ConfigFilePath` and never matched the real file. Fix: exported `config.ExpandHome` (renamed from the internal helper), applied in `resolvePaths()` to both `flags.Config` and `flags.Store` (mutates `flags` in place so downstream readers see the same expanded value). The invariant is now "any hams input that names a local path supports `~/` regardless of whether it comes from a config file, a separate CLI arg, or a --flag=value form". Three regression tests in `root_test.go`; `internal/cli` coverage 58.5% → 59.0%. (commit `3055ff4`)

### Cycle 88 — `hams refresh` also validates store_path (symmetric to apply)

- [x] Follow-up to cycle 87. The same "store_path doesn't exist → silent failure" issue affected `hams refresh` too: it printed `"No providers match: no hamsfile or state file present for any registered provider."` and exited 0, giving the user no hint that their `store_path` was misaimed. Fix: after `config.Load` in `runRefresh`, stat `cfg.StorePath`; if missing or not a directory, return the same `UserFacingError{Code: ExitUsageError}` that apply now produces. The invariant is now "any command that needs a valid store_path complains immediately when one is missing, not via a downstream symptom." Regression test `TestRunRefresh_NonexistentStorePathEmitsUserError`. (commit `8823aff`)

### Cycle 87 — store_path validation: clear error instead of misleading ".lock" wording

- [x] Real user-workflow bug found by manual end-to-end test. When `cfg.StorePath` or `--store` names a directory that doesn't exist, `runApply` propagated a confusing `"creating lock directory: mkdir /nonexistent: permission denied"` error from `state.NewLock.Acquire`. The attached suggestion was `"Remove /nonexistent/.state/m1/.lock if the previous run crashed"` — completely wrong: the user's real problem was that `store_path` itself was pointing at nothing, but the surface blamed the `.state/<machine-id>/.lock` subpath (a symptom, not the cause). Users would chase a stale-lock hypothesis that didn't exist. Fix: after resolving `storePath` and BEFORE lock acquisition, `os.Stat` the path. If missing OR not a directory, return `UserFacingError{Code: ExitUsageError}` naming the bad path verbatim with three recovery suggestions (fix config / clone-from-repo / `hams store init`). Two regression tests (`TestRunApply_NonexistentStorePathEmitsUserError` and `TestRunApply_StorePathIsFileNotDir`) assert the error shape + exit code + that the downstream `".lock"` wording never appears. (commit `98e51f2`)

### Cycle 86 — `--dry-run --from-repo` no longer clones when nothing is cached

- [x] Real user-workflow bug caught by manual end-to-end test: `hams apply --dry-run --from-repo=zthxxx/hams-store` actually cloned the remote repo to `${HAMS_DATA_HOME}/repo/zthxxx/hams-store/` — a full network call + disk write — even though dry-run promises zero filesystem/network side effects. Symmetric with the already-fixed `--dry-run --bootstrap` branch (cycle `6f8cbeb`). Fix: new `resolveFromRepoStorePath(repo, paths, dryRun)` helper picks between real clone and preview: (a) non-dry-run → `bootstrapFromRepo` as before; (b) dry-run + already on disk (either as direct local path or cached clone at `${DataHome}/repo/<user>/<name>/.git`) → reuse for accurate preview; (c) dry-run + nothing on disk → print "Would clone X. Re-run without --dry-run to clone and preview the plan." and signal the caller to return nil. 4 regression tests (`TestPreviewExistingStoreFromRepo_*` + `TestRunApply_DryRunFromRepoSkipsCloneWhenNotCached`); the end-to-end one explicitly asserts the `dataHome/repo/<user>/<name>` directory is NOT created. `internal/cli` coverage: 58.0% → 58.5%. (commit `9992e41`)

### Cycle 85 — `store_path: ~/…` now expands to the real home directory

- [x] Real user-workflow bug caught by manual end-to-end test. A user who writes `store_path: ~/Project/hams-store` in `~/.config/hams/hams.config.yaml` — the most natural thing to type — got the literal string `~/Project/hams-store` stored in `cfg.StorePath`. `cfg.ProfileDir()` then produced `~/Project/hams-store/macOS`, which never matches anything on disk, so every `hams apply` printed "No providers match: no hamsfile or state file present for any registered provider." and silently exited 0 with no clue that the `~` was the real issue. Fix: after the config merge in `config.Load`, run the new `expandHome(path)` helper (uses `os.UserHomeDir`, returns input unchanged when no `~/` prefix) on `cfg.StorePath`. bootstrap.go's `resolveLocalRepo` already had this expansion for `--from-repo` — the invariant is now "any hams input that names a local path supports `~/`". Tests: `TestLoad_StorePathTildeExpansion` (end-to-end: fake HOME + config file + Load → expanded path match) and `TestExpandHome_NoTildePrefix` (unchanged for absolute/relative/empty/`~login`). Coverage `internal/config` 88.3% → 89.7%. (commit `7f27d6a`)

### Cycle 84 — `hams apply` no longer silently exits 0 after Ctrl+C

- [x] Real user-workflow bug: when the user pressed Ctrl+C (SIGINT) or the process received SIGTERM mid-apply, `provider.Execute` saw ctx.Done and returned with `ctx.Err()` in its Errors slice — but `runApply`'s per-provider for-loop kept iterating, each provider bailing the same way. The final `merged := MergeResults(...)` showed `Failed == 0` because cancellation is accounted for in Errors, not Failed, and the summary print claimed "hams apply complete: 0 installed, ..." followed by exit code 0. A shell running `hams apply && echo ok` would print `ok` after the user hit Ctrl+C. Fix: after the provider loop, check `ctx.Err()`; if non-nil, return `UserFacingError{Code: ExitPartialFailure}` whose message names the interruption and whose suggestions point the user at `hams refresh` + re-run. Partial state was already persisted correctly by cycle 51's per-iteration `sf.Save`; this fix only addresses the silent-success surface. Regression test `TestRunApply_InterruptedContextReturnsPartialFailure` starts with a pre-cancelled context, asserts `Apply` is never called, and verifies the error shape + exit code + suggestions. (commit `1b66482`)

### Cycles 78–83 — Auto-record gap closed: npm/pnpm/uv/goinstall/mas/vscodeext

Continuing the series from cycle 77 (cargo pilot), six more Package-class providers now satisfy the CP-1 auto-record contract. Every provider wire is the same shape — new `hamsfile.go` helper, `cfg *config.Config` field on `Provider`, `New(cfg, runner)`, `handleInstall` / `handleRemove` loop runner then write hamsfile, 10 `TestHandleCommand_U*` regression tests per provider. Atomic-on-failure semantics keep hamsfile honest; flag filtering (`packageArgs`/`crateArgs`/`toolArgs`/`appIDArgs`/`extensionArgs`) keeps cargo/npm/pnpm/uv/go/mas/code flag tokens out of the recorded resource names.

Coverage gains (per provider):

- **npm** (commit `4c89814`) 69.6% → 79.2%
- **pnpm** (commit `e24e12b`) 73.2% → 81.7%
- **uv** (commit `76022d6`) 71.8% → 80.4%
- **goinstall** (commit `7caeb3f`) 64.2% → 76.4% — preserves `injectLatest` so bare module paths become `<path>@latest` before *both* runner call AND hamsfile write; no `uninstall` verb, so only install-branch auto-records.
- **mas** (commit `7de53aa`) 74.4% → 82.3%
- **code-ext** (commit `ba3bb3e`) 69.1% → 80.6%

With cargo (cycle 77, 70.6% → 81.0%), every Package-class provider (9 in total — homebrew, apt, pnpm, npm, uv, goinstall, cargo, code-ext, mas) now writes the hamsfile on successful CLI-first install/remove. Only `homebrew` still writes hamsfile-but-not-state on the CLI path; its state-file upgrade is tracked separately (out of scope for this change; apt's U12-U15 pattern is the reference).

### Cycle 77 — Auto-record gap pilot (cargo): CLI install now writes hamsfile

- [x] Implemented the pilot for openspec change `2026-04-16-package-provider-auto-record-gap`: `hams cargo install <crate>` and `hams cargo remove <crate>` now append/delete entries in `<profile>/cargo.hams.yaml` via the apt-style pattern. Prior behavior exited 0 without recording — silently violating CP-1 and the core "CLI-first, auto-record" philosophy. Added `cfg *config.Config` field to `cargo.Provider`; new `internal/provider/builtin/cargo/hamsfile.go` (port of apt's). HandleCommand now drives the CmdRunner seam (not `WrapExecPassthrough`) so the auto-record side-effects are DI-testable. 10 new `TestHandleCommand_U*` tests: U1 install-records, U2 idempotent, U3 failure leaves hamsfile untouched (not even created), U4 remove deletes, U5 remove-failure preserves, U6 dry-run skips both, U7 multi-crate all-recorded, U8 multi-crate atomic-on-failure, U9 cargo-flags filtered, U10 flags-only → usage error. Coverage 70.6% → 81.0%. Pattern locked in for the 6 follow-up providers (npm/pnpm/uv/goinstall/mas/vscodeext). (commit `39f8f4c`)

### Cycle 76 — OpenSpec: document the Package-provider auto-record gap

- [x] Investigation finding: 7 of 9 Package-class providers (cargo, npm, pnpm, uv, goinstall, mas, vscodeext) invoke the underlying package manager on `hams <provider> install` but do NOT append the entry to the provider's hamsfile, silently violating the core "CLI-first, auto-record" philosophy (CLAUDE.md) and CP-1 (`openspec/specs/builtin-providers/spec.md:41-71`). Only apt fully satisfies CP-1; homebrew records the hamsfile but not state. User impact: `hams cargo install ripgrep` exits 0 yet leaves no hamsfile record, so `hams apply --from-repo=…` on a fresh machine restores nothing. Documented the fix plan in a new openspec change `2026-04-16-package-provider-auto-record-gap/` (proposal.md, tasks.md, spec delta adding a new Requirement + 6 Scenarios). Implementation deferred to subsequent cycles, one provider per atomic commit starting with cargo pilot. (commit `f6821c7`)

### Cycle 75 — Non-TTY profile init: clear UserFacingError instead of cryptic EOF

- [x] Real user-workflow bug: `runApply` called `promptProfileInit()` unconditionally when either `profile_tag` or `machine_id` was empty. On CI / cloud-init / piped-stdin invocations, bufio's `ReadString('\n')` immediately returned io.EOF and the error propagated as `"profile init: reading profile tag: EOF"` — users could not tell from the message that they needed to set profile_tag/machine_id explicitly. The `store init` command already had a TTY check (commands.go:571); `runApply` did not. Fix: extracted `ensureProfileConfigured(paths, storePath, cfg)` in apply.go that picks TTY prompt vs UserFacingError based on `term.IsTerminal(os.Stdin.Fd())`. Non-TTY path returns a `UserFacingError{Code: ExitUsageError}` whose message names exactly which keys are missing and whose suggestions teach `hams config set profile_tag <tag>` / `hams config set machine_id $(hostname)`. Refactor also silences a nestif complexity violation. Two regression tests: `TestRunApply_NonTTYWithoutProfileEmitsUserError` (both keys missing) and `TestRunApply_NonTTYWithProfileFlagButNoMachineID` (only machine_id missing after --profile override); both assert the error shape and that `EOF` never leaks into the surface. (commit `ce0c6b7`)

### Cycle 74 — Coverage tests for config set gating and routing

- [x] Two new tests in `internal/config/config_test.go`: `TestIsValidConfigKey` covers the whitelist used by `hams config set` (profile_tag, machine_id, store_path, store_repo, llm_cli — plus negative cases: typos, sensitive-pattern leaks, empty); `TestWriteConfigKey_GlobalVsLocal` covers the sensitive-vs-nonsensitive routing (non-sensitive → global YAML, sensitive like `notification.bark_token` → local YAML, with cross-file leak checks). Both functions were 0% covered despite being the core of `hams config set`. `internal/config` coverage: 77.4% → 88.3%. No behavior change — coverage only. (commit `4db29d5`)

### Cycle 73 — Regression tests for cycle 72 error-transform + refactor

- [x] Lifted the cycle-72 inline transform into a pure `transformCloneError(repoURL, err)` helper (was untestable inside cloneRemoteRepo because it shells out to a real git remote). Two unit tests: "Repository not found" gets *UserFacingError with 3 suggestions and no "authentication" leak; other errors (e.g. "dial tcp: connection refused") propagate verbatim with the url-prefix wrap. No behavior change — coverage only. (commit `e97b3f4`)

### Cycle 72 — `--from-repo` friendly error for missing remote repo

- [x] **Real UX confusion**: go-git reports missing public GitHub repos as "authentication required: Repository not found" — users chased credential issues when the real cause was a typo in the URL. Detect the "Repository not found" substring and emit a targeted UserFacingError with three suggestions (verify URL / configure git credentials for private / use absolute path for local). Other go-git errors still propagate verbatim. (commit `35443f5`)

### Cycle 71 — `list --json` adds spec-required `name` field

- [x] **Spec drift**: cli-architecture §"List in JSON format" says each element SHALL contain `provider`, `name`, `status`, `version`. My prior `listResource` had `id` (the full URN like `urn:hams:apt:htop`) but no short `name` — JSON consumers had to parse URNs themselves. Added `name` field via new `shortName(id)` helper that strips the `urn:hams:<provider>:` prefix; `id` retained for scripts that want the unique handle. Regression test covers 7 cases including bare names and malformed URNs. (commit `03dacc2`)

### Cycle 70 — Regression test for cycle 69 missing-store_path detection

- [x] `TestStoreStatus_MissingStorePath` asserts three invariants after pointing `store_path` at a ghost directory: (1) output contains "does NOT exist", (2) output contains "hams store init" suggestion, (3) the normal "Profile tag:" / "Machine ID:" status block is suppressed (so misleading derived paths don't appear). (commit `1aa47a6`)

### Cycle 69 — `store status` detects non-existent `store_path`

- [x] If `store_path` pointed at a missing directory, `store status` printed derived paths + "Hamsfiles: (profile dir not found)" with no indication that the ROOT was missing. Added an upfront `os.Stat` probe; text mode emits a loud "(does NOT exist)" header + actionable hints (`store init` or fix config), JSON mode exposes `store_path_exists: bool`. Normal-case output unchanged. (commit `20df10c`)

### Cycle 68 — Regression test for cycle 67 dual-sink slog

- [x] `TestSetup_DualSink` redirects `os.Stderr` to a pipe, calls Setup, emits a `slog.Info` with a SIGIL marker, drains both sinks, asserts both captured it. Prevents a future refactor from silently dropping either sink (live feedback OR persistent capture). (commit `54ec4df`)

### Cycle 67 — Dual-sink slog: stderr AND file (fix for cycle 65 regression)

- [x] **Caught my own bug**: `logging.Setup` replaced the default slog handler with one writing ONLY to the file. After cycle 65 wired this into apply/refresh, users saw no live progress — `hams apply` appeared to hang while silently writing to the log file. Swapped the handler's writer for `io.MultiWriter(os.Stderr, logFile)` so both destinations receive the same output. Live feedback restored, file capture preserved. (commit `68e66ab`)

### Cycle 66 — Regression test for cycle 65 log-file wiring

- [x] `TestRunRefresh_CreatesSessionLogFile` asserts runRefresh creates the month-bucket log file at `${HAMS_DATA_HOME}/<YYYY-MM>/hams.<YYYYMM>.log` with non-zero content. Even the no-providers-match early-return path triggers SetupLogging first, so the regression guard covers the common case. (commit `4dd1338`)

### Cycle 65 — `SetupLogging` wired into apply+refresh

- [x] **Real scaffolded-but-unwired finding**: `cli.SetupLogging` was defined but had ZERO callers. Users got stderr output only — no rolling log file at `${HAMS_DATA_HOME}/<YYYY-MM>/hams.<YYYYMM>.log` despite spec references and the tui-logging "sticky header shows log file path" scenario pre-supposing its existence. Wired into `runApply` + `runRefresh` with deferred cleanup; short read-only commands (list, config get, version) unchanged. Verified apply dry-run now creates the file with session slog lines. (commit `dddecb0`)

### Cycle 64 — notify Channel.Name() contracts (52% → 60%)

- [x] Added `TestDesktopNotifier_Name` and `TestBarkChannel_Name` covering the previously 0%-covered `.Name()` methods (used by Manager.Notify's per-channel slog.Info). `Bark.Send()` intentionally left uncovered — DI'ing the hardcoded https URL now would lock an API shape before v1.1 un-defers the notification wiring. Added a clear comment pointing to the deferral. (commit `18dc477`)

### Cycle 63 — state ResourceOptions + FormatPID tests (72.6% → 84.1%)

- [x] `WithValue`, `WithCheckCmd`, `WithCheckStdout`, `FormatPID` were all at 0%. Added table-driven `TestResourceOptions_Values` covering all three setters, and `TestFormatPID` covering both the bare-int fallback (negative PID) and the `/proc/<pid>/cmdline` happy path via `os.Getpid()`. (commit `7eb825f`)

### Cycle 62 — i18n Tf nil-localizer fallback (90.7% → 92%)

- [x] `Tf` has an early-return branch when `localizer` is nil (Init not called / failed). Every UI caller depends on it passing through the msgID rather than panicking. Added regression test that swaps `localizer = nil` and asserts the guarantee. (commit `8dac8ad`)

### Cycle 61 — `LoadOrCreateEmpty` + `ListApps` coverage (74% → 82%)

- [x] Both public helpers were at 0% despite being called by every provider. Added 5 tests: missing-file creates parent dir and returns fresh File; existing-file loads apps verbatim; parent-is-a-file surfaces ENOTDIR; ListApps walks all tags and returns both `app:` and `urn:` values; empty/nil-root returns nil without panic. (commit `f147c80`)

### Cycle 60 — `store status --json` emits machine-parseable payload

- [x] Matches cycle 59's pattern. Refactored status computation (hamsfiles count, git status, git changes count) into local variables, then branch on flags.JSON. Added `git_changes` as an integer so scripts can detect "any uncommitted changes?" without parsing the string. (commit `6a3db54`)

### Cycle 59 — `config list --json` emits machine-parseable output

- [x] `--json` global flag was honored by `hams list` and error output, but `hams config list --json` silently printed text. Added a flat JSON object emission covering the same fields as text output. Text path unchanged. (commit `5cfc09e`)

### Cycle 58 — Regression test for cycle 55 filter-excluded distinction

- [x] `TestList_FilterExcludedAll_DistinctMessage` seeds a state with one ok resource, runs `list --status=hook-failed`, asserts the filter-excluded message appears and the empty-store install hint does NOT. (commit `267d9f4`)

### Cycle 57 — Regression tests for cycle 56 store status

- [x] `TestStoreStatus_SpecCompliantOutput` asserts the four spec-required lines (store path, profile tag, machine-id, hamsfiles) appear. `TestStoreStatus_WithGitRepo` `git init`s the store, runs status, asserts the Git status line is present with "uncommitted" or "clean". (commit `8c0fe48`)

### Cycle 56 — `store status` surfaces profile tag, machine-id, git changes

- [x] Spec drift (cli-architecture §"Store command"): spec required `hams store status` to display profile tag, machine-id, and git status; impl only printed store path + derived profile/state dirs + hamsfile count. Added explicit `Profile tag:` / `Machine ID:` lines and a conditional `Git status:` line (clean / N changes / fail-open on non-git stores). 5s timeout on the git call guards against hangs. (commit `979c012`)

### Cycle 55 — `list` distinguishes empty-state from filter-excluded-all

- [x] `hams list --status=hook-failed` against a store with 5 tracked resources (none in that status) printed "No managed resources found. Run 'hams install ...'" — misleading because the user HAS resources. Added `hadAnyResources` tracker; split the final output into two branches (truly-empty vs. filter-matched-zero) with appropriate hints. (commit `dc28d96`)

### Cycle 54 — `--status` filter validates values

- [x] `hams list --status=failled` (typo) silently matched zero resources and printed "No managed resources found" — indistinguishable from an empty store. Validated each comma-separated value against the 5 defined `ResourceState` constants; typos return `ExitUsageError` with the unknown value and the valid-state list. Multi-value filters still work. (commit `4e399a8`)

### Cycle 53 — Panic recovery in parallel Probe goroutines

- [x] Matches cycle 51's pattern but for `ProbeAll`: a panic in any provider's Probe would take down the whole parallel refresh, even though healthy providers' probes had completed. `defer recover()` in each goroutine body now logs the panic and omits the provider from the results map — runRefresh (cycle 40) surfaces the mismatch via ExitPartialFailure. (commit `6752155`)

### Cycle 52 — `brew untap` for tap removals (was silently broken)

- [x] **Real UX bug**: Homebrew's `Remove` always called `brew uninstall <id>`. For tap-format IDs (`user/repo`), brew rejects that with "No installed keg or cask" — so a user who deleted `homebrew/cask-fonts` from their hamsfile saw the removal marked as failed forever; the tap stayed registered. Added `Untap` to `CmdRunner` (real + fake) and routed Remove through it when `isTapFormat` is true. Tests U11/U12 lock both branches. (commit `5fab924`)

### Cycle 51 — Panic recovery in apply loop (data-integrity)

- [x] **Real data-integrity issue** (from agent-assisted audit): if a provider's Apply method panics mid-loop (buggy provider, OOM in runner), in-memory state updates for successful actions were lost because `sf.Save` hadn't run. Next apply would re-attempt already-installed resources. Wrapped each provider's Execute+Save in an IIFE with `defer recover()`: log panic context → best-effort `sf.Save` → re-throw panic. Regression test simulates a provider succeeding on action 1 then panicking on action 2, asserts state contains action 1. (commit `27bbb35`)

### Cycle 50 — Regression test for cycle 49 store_repo resolution

- [x] `TestRunApply_AutoResolvesStoreFromConfigRepo` asserts runApply resolves the store via `store_repo` when no `--from-repo`/`--store`/`store_path` is set. Uses a local bare-repo fixture so the test is network-free. (commit `7bb688f`)

### Cycle 49 — `store_repo` config field actually used

- [x] **Real spec drift**: schema-design spec lists `store_repo` as a REQUIRED config field that "points to the hams store repository". But the impl only read it back via `config get store_repo` for display — never used it to resolve the store. Users who followed the spec saw "no store configured" despite having it set. Fixed: when no `--from-repo`/`--store`/`store_path` is present, `store_repo` is now treated as the effective `--from-repo` (lowest precedence). (commit `b035a03`)

### Cycle 48 — `runRefresh` test coverage (45.8% → 49.8%)

- [x] `runRefresh` had zero coverage despite being a top-level command. Added three tests: flag-exclusion (cycle 38), no-providers-match happy path, and ExitPartialFailure on state-load corruption (cycles 40/43/47 regression guard). (commit `350ed9a`)

### Cycle 47 — Remaining silent state-save failures propagated

- [x] Two more silent-log save paths after cycle 46: (1) apply's pre-apply refresh phase called sf.Save with log-only failure — now appends to the same `stateSaveFailures` slice so the final summary covers both probe-phase and install-phase save failures. (2) runRefresh's own probe loop had the same silent-log pattern — added a `saveFailures` slice, extended the "Refresh complete" output to show save failures alongside probe failures, returns ExitPartialFailure. After cycles 39/40/43/46/47 every state.Save/state.Load failure surfaces to exit code + terminal. (commit `89553b0`)

### Cycle 46 — State-save failures reported in final summary

- [x] After a successful install, if `sf.Save()` fails (disk full, mid-run permission change), only a slog.Error fired and apply reported "complete" with exit 0 — scripts couldn't detect state drift. Now tracked in a `stateSaveFailures` slice, surfaced in the final summary with a user-friendly hint about "next apply may re-execute these resources", and included in the ExitPartialFailure condition. (commit `fdc09cd`)

### Cycle 45 — State-corruption fix propagated to CLI handlers (apt, brew)

- [x] Same silent-reset bug as cycle 43 in the per-provider CLI paths (`hams apt install`, `hams brew list`). apt's `loadOrCreateStateFile` changed signature from `*state.File` → `(*state.File, error)`; missing-file still synthesizes, corruption propagates with path context. homebrew's `handleList` now returns the wrapped error instead of silently showing "all desired as additions". Together with cycle 43, every production state.Load call distinguishes ErrNotExist from destructive errors. (commit `667552b`)

### Cycle 44 — Regression test for cycle 43 state-corruption fix

- [x] `TestApply_CorruptedStateFile_SkipsProviderNotSilentReset` asserts three invariants: (1) runApply returns `ExitPartialFailure`, not nil; (2) zero apply actions ran against the synthesized-empty state; (3) the corrupt state file is preserved verbatim on disk so users can inspect it. (commit `4c888ee`)

### Cycle 43 — Silent state-reset on corrupted state file (CRITICAL DATA)

- [x] **Real data-integrity bug**: `state.Load` returns a wrapped error for any read/parse failure. Both `apply.go` and `probe.go` swallowed all errors and substituted `state.New()` — a corrupted state file would silently reset to empty, losing drift tracking for every tracked resource and potentially re-triggering installs. Distinguished `errors.Is(err, fs.ErrNotExist)` (first-run, OK) from other errors (corruption, permission). Corrupt state now skips the provider with a clear ERROR log and surfaces via the cycle 39/40 ExitPartialFailure flow. (commit `5fac677`)

### Cycle 42 — git-clone Plan coverage (23% → 43%)

- [x] 3 tests for git-clone's Plan + cloneParseResources: structured-fields path, legacy `"remote -> path"` scalar fallback, non-mapping-root error. Biggest git-provider coverage uplift in this run — remaining uncovered code (Apply/handleAdd/HandleCommand) needs real git execution and is covered by Docker integration tests. (commit `4e07068`)

### Cycle 41 — regression test for cycle 39 dry-run exit semantics

- [x] Added `TestApply_DryRun_SkippedProvider_ReturnsPartialFailure` — simulates a provider whose Plan returns an error and asserts runApply returns `*UserFacingError{Code: ExitPartialFailure}` whose message mentions "dry-run". Locks in the cycle 39 fix from the test side. (commit `fbc23e2`)

### Cycle 40 — `refresh` returns non-zero when probes fail

- [x] **Twin of cycle 39** — `hams refresh` printed "(1 probe error(s); see log for details)" but returned nil (exit 0). Scripts couldn't detect probe failures. Now returns ExitPartialFailure with a slog-pointer suggestion. Happy-path unchanged. (commit `795aca1`)

### Cycle 39 — `apply --dry-run` reports skipped providers

- [x] **Real silent bug**: `hams apply --dry-run` exited 0 even when a hamsfile failed to parse. CI preview scripts missed broken hamsfiles until the actual apply ran. Non-dry-run correctly returned ExitPartialFailure; dry-run silently swallowed it. Parity restored: dry-run now prints the skipped-provider warning AND returns ExitPartialFailure with a targeted suggestion. (commit `68bf644`)

### Cycle 38 — `--only`/`--except` exclusion checked before config load

- [x] `hams apply --only=X --except=Y` with no store returned "no store configured" first, then the user fixed it and got "mutually exclusive" on the second attempt. Moved the exclusion check to the top of runApply/runRefresh (alongside the existing `--bootstrap/--no-bootstrap` check). Downstream filterProviders still guards for programmatic callers. (commit `26d5117`)

### Cycle 37 — homebrew Plan + caskApps coverage (49.3% → 57.0%)

- [x] 4-app hamsfile (2 cli + 2 cask) asserts that Plan correctly attaches `BrewResource{IsCask:true}` to cask-tagged packages (needed by Apply's `--cask` injection) and leaves cli-tagged packages with nil Resource. Also covers the empty-Root short-circuit. (commit `6d28ca8`)

### Cycle 36 — `hams list` singular/plural grammar

- [x] `hams list` printed "apt (1 resources):" for a single resource. Tiny but visible polish. Branch on `len(filteredIDs) == 1` for "resource" vs "resources". Also fixed a stray bare URL in AGENTS.md from cycle 35. (commit `0d6adfe`)

### Cycle 35 — `--from-repo=/local/path` surfaces local errors

- [x] `hams apply --from-repo=/tmp/not-a-git-repo` printed a misleading "cloning `https://github.com//tmp/...`: Repository not found" because local-repo failure fell through to the GitHub-shorthand path. Added `isLocalPathAttempt()` to distinguish unambiguously-local inputs (prefix `/`, `~/`, `./`, `../`, or stat-visible) from remote shorthand. Local-looking inputs now surface the real local error. Regression test covers 3 cases. (commit `fa414e1`)

### Cycle 34 — selfupdate 0%-entry-point tests (68.9% → 76.4%)

- [x] 3 tests for `NewUpdater`, `CurrentVersion`, `LatestRelease`. `LatestRelease` gets a full httptest round-trip asserting both Version and Assets are mapped from the GitHub API JSON — complements the existing `LatestVersion` tests that only covered tag-name extraction. (commit `c19b6c2`)

### Cycle 33 — `printConfigKey` coverage (42.7% → 45.5%)

- [x] 3 tests for the typed-fields switch, typo rejection, and the sensitive-key-no-file silent-exit path. Added a `captureStdout` helper mirroring the existing `captureStderr`. (commit `1bbdd1a`)

### Cycle 32 — `ensureStoreIsGitRepo` + `localConfigPath` tests

- [x] Added unit tests for the two helpers introduced in cycles 18/27 — both were at 0% coverage. 3 subtests for the git-repo check (`.git`, bare HEAD, plain dir), and a simple routing check for the config-path helper. Pure functions, cheap regression guards. (commit `da06d44`)

### Cycle 31 — Plan coverage for remaining 6 providers (batch)

- [x] cargo/goinstall/npm/pnpm/uv/vscodeext all had Plan at 0% despite being called on every apply. Added a uniform `TestPlan_WrapsComputePlanWithHooks` to each (dedicated `plan_test.go` for 5, U10 in cargo's lifecycle file). Coverage gains: cargo 68→71, goinstall 62→64, npm 67→70, pnpm 71→73, uv 70→72, vscodeext 67→69. Every v1 provider's Plan wrapper is now regression-guarded. (commit `97a0be3`)

### Cycle 30 — duti Plan coverage (79.5% → 82.1%)

- [x] TestU11_Plan for duti, matching the mas/ansible/defaults pattern. All 4 macOS-scoped Plan wrappers now covered. (commit `185f760`)

### Cycle 29 — mas Plan coverage (72% → 74%)

- [x] Added `TestU11_Plan_WrapsComputePlanWithHooks` following the cycle 22 pattern for ansible/defaults. Guards against accidental short-circuiting of ComputePlan/PopulateActionHooks in mas's Plan wrapper. (commit `01bdaad`)

### Cycle 28 — `hams self-upgrade` honors `--dry-run`

- [x] Global `--dry-run` advertised but `self-upgrade` ignored it — ran `brew upgrade` or actually downloaded+replaced the binary. Both channels now print a preview (no side effects). Binary path still resolves the latest release (read-only) so it can show "Would upgrade from vA to vB". Threads `*provider.GlobalFlags` end-to-end. (commit `5a31463`)

### Cycle 27 — Friendly error when `store push/pull` runs in a non-git dir

- [x] **Raw git error surfaced**: `hams store push` in a non-git store showed "fatal: not a git repository" + "git add: exit status 128" — user had no idea what to do. Added `ensureStoreIsGitRepo(storePath)` that checks for `.git/` or `HEAD` and returns a UserFacingError with two suggestions (`git init` or `hams apply --from-repo=`). Gate both push and pull. (commit `59ae404`)

### Cycle 26 — Observability spec reconciled with HAMS_OTEL reality

- [x] **Session-creation scenarios said unconditional**: spec said `hams apply` always creates an OTel session; impl requires `HAMS_OTEL=1` (cycle 5 un-deferral). Added a "v1 enablement gate" paragraph + `HAMS_OTEL=1` qualifier on relevant scenarios + new "OTel disabled by default" scenario.
- [x] **`otel.exporter` config field doesn't exist**: spec had scenarios about `otel.exporter: otlp` in `hams.config.yaml`, but no such field is plumbed through. Marked scenarios as "(v1.1)" and added a "v1 status" note that the file exporter is selected unconditionally. Spec-only change. (commit `e2ba20a`)

### Cycle 25 — `hams store init` prompts for profile tag on TTY

- [x] Spec required prompting for initial profile tag on init; impl silently defaulted to "default". Added TTY-guarded prompt (non-TTY falls through for scriptability). Persists both `profile_tag` and `machine_id` to global config so subsequent apply doesn't re-prompt. Persist-failure degrades to WARN (store still usable). (commit `581575f`)

### Cycle 24 — `hams store init` fixes + .gitignore generation

- [x] **Missing `.gitignore`**: spec requires `.state/` + `*.local.*` patterns to prevent state/local overrides leaking into git. Users would commit machine-id + secrets. Added idempotent `.gitignore` creation on init.
- [x] **Scope violation latent**: initial store config was a marshaled `Config{}` copying `profile_tag`/`machine_id` from the global layer. For users with profile_tag set globally, init wrote a machine-scoped field into the store file, which then failed `validateStoreScope` on next load. Replaced with a commented placeholder explaining the scope rule. Empty-string noise gone. (commit `c3020ce`)

### Cycle 23 — `hams version` subcommand wires up the detailed build info

- [x] `version.Info()` had zero production callers (scaffolded-but-unwired, same pattern as lucky/TUI/notify but small enough to close fully). Users filing bug reports needed the full "semver (commit) built <date> <goos>/<goarch>" string; `--version` only returned the brief form. Added `hams version` subcommand routing to `version.Info()`. Added `TestNewApp_VersionSubcommandAvailable`. (commit `e3a5d81`)

### Cycle 22 — Plan coverage for ansible + defaults

- [x] Ansible `TestU9_Plan_AttachesPlaybookPathAsResource` (70.5% → 76.9%) — verifies Plan decorates each action.Resource with the URN (string), required by Apply's type assertion.
- [x] Defaults `TestU10_Plan_WrapsComputePlanWithHooks` (58.6% → 60.4%) — populated + empty subtests. Both previously 0%-covered Plan functions now exercised end-to-end. (commit `b156196`)

### Cycle 21 — Bash Plan + bashParseResources coverage (51% → 86.5%)

- [x] Added `TestPlan_ParsesAndEnrichesActions` — drives the bash provider's Plan → bashParseResources flow with a YAML hamsfile containing two URNs (one with check, one with sudo+remove). Asserts actions enriched with parsed Resource, including sudo-prefix on cached remove command. Biggest v1 coverage jump so far. (commit `413bd6e`)

### Cycle 20 — Homebrew helper coverage (45.2% → 49.3%)

- [x] **4 pure helpers uncovered**: `isTapFormat`, `parseInstallTag`, `packageArgs`, `hasCaskFlag`. Added table-driven tests with edge cases that document intended behavior (e.g., `isTapFormat("user/repo.git") = false`, `parseInstallTag("a,b") = "a"`). No production changes. (commit `03d1955`)

### Cycle 19 — Context propagation through every provider HandleCommand

- [x] **Signal handling broken at provider boundary**: cycle 12 wired SIGINT/SIGTERM into the root context and forwarded it to provider `HandleCommand`, but every single provider dropped the context by calling `exec.CommandContext(context.Background(), …)` or `WrapExecPassthrough(context.Background(), …)`. Ctrl+C during a long `brew install` / `pnpm add` / etc. did nothing. Threaded `ctx` through all 12 v1 providers (ansible, cargo, defaults, duti, git-config, git-clone, goinstall, mas, npm, pnpm, uv, vscodeext). No production code references `context.Background()` anymore. (commit `10f2897`)

### Cycle 18 — `config list` surfaces the local-overrides path

- [x] **Invisible .local.yaml**: after cycles 16/17 users could set/get `notification.bark_token`, but `hams config list` never told them where those values landed. Added a "Local overrides:" line using the same routing helper (`localConfigPath`) as WriteConfigKey/ReadRawConfigKey. Minimal fix — full per-key source-annotated listing per cli-architecture spec §"List all config values" is a larger refactor deferred to a future cycle. (commit `23fff82`)

### Cycle 17 — Symmetric `config get/set` for sensitive keys

- [x] **`config get notification.bark_token` rejected**: users could `set` but not `get` an arbitrary sensitive key — total info asymmetry. Added `config.ReadRawConfigKey(paths, storePath, key)` that uses the same routing as `WriteConfigKey`. `printConfigKey` now falls through to it for sensitive-pattern keys. Returns `(value, found, err)` so callers distinguish "unset" from "error". Tests: `TestReadRawConfigKey_SensitiveFromStoreLocal`, `TestReadRawConfigKey_UnsetReturnsFalse`. (commit `57bfc98`)

### Cycle 16 — Sensitive config key gating matches the schema-design spec

- [x] **`hams config set` accepted only 5 hardcoded keys**: the spec requires `notification.bark_token` (and similar) to auto-route to `hams.config.local.yaml`, but the `set` command rejected any key not in the `ValidConfigKeys` whitelist. Loosened the gate: accept whitelisted keys OR keys matching a sensitive pattern. Typos still rejected. (commit `910165c`)
- [x] **Missing "key" pattern in `sensitivePatterns`**: schema-design spec lists `token, key, secret, password, credential` but impl only had 4 of them — `api_key` fell through as non-sensitive. Added "key" with an explanatory comment; expanded `TestIsSensitiveKey_SubstringMatch` to cover `api_key` and `openai_api_key`.
- [x] Manual verification of `hams config set notification.bark_token abc123` and `openai_api_key sk-xxx` confirmed routing + 0600 perms on the `.local.yaml` file.

### Cycle 15 — Platform consistency between internal and CLI registries

- [x] **Platform mismatch leaked into `hams --help`**: internal registry silently skipped `defaults`/`duti`/`mas` on Linux (so `apply --only=defaults` correctly said "unknown provider"), but the CLI dispatch registry advertised them anyway. Linux users saw macOS-only commands in help and then exec-failed with "executable not found". Exported `provider.IsPlatformsMatch`; apply the same platform filter in `registerBuiltins` before calling `RegisterProvider`. Added `TestRegisterBuiltins_FiltersCLIByPlatform` with runtime.GOOS-aware assertions. (commit `d843bfd`)

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
