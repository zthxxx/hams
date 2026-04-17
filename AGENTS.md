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

- [x] `hams --json config get` (cycle 236), `hams --json config list`, `hams --json apply|refresh|list`, `hams --json version` (cycle 181) all emit JSON. But `hams --json config set <key> <value>` and `hams --json config unset <key>` emitted the same plain-text `Set <key> = <value> (in <target>)` / `Unset <key> (from <target>)` as without `--json`. A CI script running `hams --json config set notification.bark_token abc | jq '.key'` failed because stdout was non-JSON. The principle that `--json` applies globally was broken on two widely-used config write paths. Fix: introduce `emitConfigSetResult` / `emitConfigUnsetResult` helpers in `internal/cli/commands.go` that branch on `flags.JSON` and emit `{"key", "value", "target", "dry_run"}` (set) / `{"key", "target", "dry_run"}` (unset) shapes — symmetric with the cycle-236 `config get --json` shape and the apply/refresh JSON conventions. Text mode preserves the pre-cycle-246 wording verbatim (no regression for human users). Covers both the real-write and `--dry-run` branches. Three regression tests: `TestConfigSet_JSONMode_EmitsStructuredResult`, `TestConfigSet_JSONMode_DryRunFlagsDryRunTrue` (includes assertion that the config file remains absent — JSON path still respects `--dry-run`'s no-mutation contract), `TestConfigUnset_JSONMode_EmitsStructuredResult`. All use the shared `assertConfigCmdJSON` helper that unmarshals stdout and checks each expected field. `task fmt lint test:unit` all green. (commit `0e4c30f`)

### Cycle 247 — `hams --json apply` no-providers-match path emits JSON, not prose

- [x] `runApply` has two "no providers match" exit paths: stage-1 filter excludes every provider (no hamsfile/state across the profile) and the state-only-without-prune-orphans drop. Both called `reportNoProvidersMatch` / `fmt.Println(...)` unconditionally. `hams --json apply --only=apt` on a profile with no apt artifacts dumped text to stdout; `hams --json apply ... | jq .` errored on invalid JSON. Fix: introduce `emitEmptyApplyJSON(applyStart time.Time) error` that calls `buildApplyJSONSummary` with zero-valued `ExecuteResult` / nil slice inputs. Both no-match paths now route through it when `flags.JSON` is set — identical shape to a healthy zero-work real apply (`success=true`, `dry_run=false`, all counts 0, elapsed_ms present). CI consumers iterate the same schema across empty and non-empty applies. Text mode preserves the pre-cycle-247 human-readable diagnostic (per spec §"Apply with `--only` outside the stage-1 result" — "exit 0 with a 'no providers match' message"). Regression test `TestRunApply_NoProvidersMatch_JSONMode` registers an alpha provider, writes NO hamsfile, runs with `flags.JSON=true`, asserts: stdout parses as JSON, contains no prose markers, `success=true`, all aggregate counts are 0, `elapsed_ms` present. `task fmt lint test:unit` all green. (commit `6a8833e`)

### Cycle 248 — `hams --json refresh` no-providers-match path emits JSON

- [x] Symmetric with cycle 247 on the refresh side. `runRefresh` had the same unconditional `fmt.Println("No providers match: ...")` when the stage-1 filter produced zero providers — `hams --json refresh --only=apt` on a profile with no apt artifacts dumped text through `jq .`. Fix: introduce `emitEmptyRefreshJSON(dryRun bool, refreshStart time.Time) error` that emits the refresh success-path shape with zero counts (`probed=planned=0`, `success=true`, empty `save_failures` + `probe_failed_providers`, `dry_run` preserved from the flag, `elapsed_ms` present). The stage-1 no-match branch routes through it when `flags.JSON` is set; text mode preserves the pre-cycle-248 "No providers match" diagnostic (including the branch that distinguishes stage-1 empty from stage-2 excluded). Regression test `TestRunRefresh_NoProvidersMatch_JSONMode` registers an alpha provider with no hamsfile, runs with `flags.JSON=true`, asserts stdout parses as JSON, no prose markers, `success=true`, `probed=planned=probe_failures=0`, `elapsed_ms` present. `task fmt lint test:unit` all green. (commit `dad9188`)

### Cycle 249 — `--config=<path>` with missing file hard-fails with UFE

- [x] `hams --config=<path>` silently fell back to built-in defaults when `<path>` did not exist. `config.Load` called `mergeFromFile(cfg, globalPath)`; a `NotExist` error was swallowed by `!os.IsNotExist(err)` guard — the SAME branch that correctly ignores missing default-path files (fresh install, no config yet). Result: a typo like `hams --config=~/myy.yaml apply` parsed the flag, applied the override to `paths.ConfigFilePath`, tried to read the file, quietly ignored the NotExist, fell through to the built-in defaults, and ran with surprising resolved values (empty `profile_tag`, empty `store_path`, etc.). The user's explicit request got dropped with zero feedback. Fix: convert `mergeFromFile` result from inline chain into an explicit `switch` with three branches — success (merged), real parse error (surface), and `paths.ConfigFilePath != "" && NotExist` (new — return `hamserr.NewUserError(ExitUsageError, "config file <path> does not exist", "Check the path spelling", "Create the file: touch <path>", "Or omit --config to use the default ...")`). `config` package now imports `hamserr` — no circular dep (verified: `hamserr` imports only `fmt`). Two regression tests: `TestLoad_ExplicitConfigFilePath_MissingHardFails` asserts the typo path errors with a message containing "does not exist"; `TestLoad_DefaultConfigFileMissingStillOK` confirms the default-path case (no `--config` flag, no file) still loads cleanly — no regression on the fresh-install workflow. `task fmt lint test:unit` all green. (commit `c10a66a`)

### Cycle 250 — `hams apply --from-repo=<X>` progress routed to stderr

- [x] `cloneRemoteRepo` wrote its "Downloading Hams Store to <path>", "Download Hams Store success", "Profile Store is <path> now" status messages via `fmt.Printf` (stdout) AND passed `Progress: os.Stdout` to go-git's clone — interleaving git transfer progress into stdout. `resolveFromRepoStorePath` did the same for its dry-run preview `"[dry-run] Would clone <repo>. Re-run without --dry-run..."`. Consequence: `hams --json apply --from-repo=<X>` (and `hams --json --dry-run apply --from-repo=<X>`) emitted prose BEFORE the final JSON summary. CI running `hams --json apply --from-repo=<X> | jq .` failed on invalid JSON. Fix: route all three bootstrap-layer diagnostics (clone status + dry-run preview) to `os.Stderr` and change `gogit.CloneOptions.Progress` to `os.Stderr`. Follows standard UNIX convention — stdout is for primary machine-consumable output (the JSON summary), stderr is for diagnostics / progress. Git itself writes progress to stderr, so this also aligns with user expectations for `hams apply --from-repo=X > result.log` (progress stays on terminal, summary goes to file). Regression test `TestResolveFromRepoStorePath_DryRunWouldCloneGoesToStderr` nests `captureStderr` inside `captureStdout`, invokes `resolveFromRepoStorePath` with `dryRun=true` and a repo that won't resolve locally, then asserts stderr contains `[dry-run] Would clone` AND stdout contains NEITHER. `task fmt lint test:unit` all green. (commit `9fc006f`)

### Cycle 251 — `hams --json --dry-run apply --from-repo=<X>` emits JSON when repo not cached

- [x] Cycle 250 fixed WHERE the "Would clone" diagnostic went (now stderr). But the underlying flow in `runApply` returned `nil` with zero bytes of stdout when `resolveFromRepoStorePath` signaled `done=true` (dry-run path without existing cache). `hams --json --dry-run apply --from-repo=<X> | jq .` still failed — now with "empty input" rather than "invalid prose". The correct JSON-mode answer for "nothing planned, would clone and then plan on next real run" is: emit the dry-run summary shape with zero planned actions. Fix: when `done=true` AND `flags.JSON`, route through `emitDryRunJSON(nil, nil, nil, elapsed_ms)` which produces `{"dry_run": true, "success": true, "planned_actions": [], "skipped_providers": [], "state_save_errors": [], "elapsed_ms": N}`. Text mode keeps `return nil` (stderr diagnostic from cycle 250 is the full signal for humans). Regression test `TestRunApply_DryRunFromRepoNotCached_JSONEmits` isolates HOME paths so the clone-cache lookup definitely misses, runs `runApply` with `flags.JSON=DryRun=true`, asserts stdout parses as JSON, `dry_run=true`, `success=true`. `task fmt lint test:unit` all green. (commit `2c22617`)

### Cycle 252 — Interactive profile prompts routed to stderr

- [x] `promptProfileInit` wrote `"Profile tag: "` and `"Profile Machine-ID: "` via `fmt.Print` (stdout). `ensureProfileConfigured` wrote `"Not Found Profile in config, init it at first"` via `fmt.Println` (stdout). An interactive `hams --json apply` on a fresh machine — TTY stdin, profile missing — interleaved all three prose lines into the JSON output surface, breaking `jq .`. The `term.IsTerminal(os.Stdin)` guard kept CI scripts (non-TTY) on the error path which already routed through `PrintError`; but interactive users were exposed. Fix: switch the three `fmt.Print`/`fmt.Println` calls to `fmt.Fprint(os.Stderr, ...)` / `fmt.Fprintln(os.Stderr, ...)`. Stderr is the conventional channel for prompts — interactive users still see the prompts on their terminal, and CI consumers redirecting stdout (`> result.json`) no longer get prose mixed with JSON. Regression test `TestPromptProfileInit_PromptsGoToStderr` pipes valid input through `os.Stdin`, nests `captureStderr` inside `captureStdout`, calls `promptProfileInit`, asserts stderr contains both prompt markers AND stdout contains NEITHER. `task fmt lint test:unit` all green. (commit `01f87b2`)

### Cycle 253 — Bootstrap consent prompt routed to stderr

- [x] `interactiveBootstrapPrompt` (triggered when a provider returns `BootstrapRequiredError` — e.g. Homebrew missing, user got no `--bootstrap` / `--no-bootstrap` flag, and stdin is a TTY) writes the script preview + side-effect summary + `[y/N/s]` question to `bootstrapPromptOut`. Pre-cycle-253 the default was `os.Stdout`, so an interactive `hams --json apply` hitting this path interleaved prompt prose into the JSON output surface. Symmetric gap to cycle 250 (clone progress) + cycle 252 (profile prompts) — stdout is reserved for the primary machine-consumable output; stderr is the channel for interactive prompts. Fix: change the default assignment from `os.Stdout` to `os.Stderr`. Tests override `bootstrapPromptOut` with their own `bytes.Buffer`, so they're unaffected. Regression test `TestBootstrapPromptOut_DefaultsToStderr` asserts the default identity is `os.Stderr` — since existing tests override the variable, they don't catch a regression of the default, so the invariant needed its own anchor. Coverage: `internal/cli` 77.1% → 77.6%. `task fmt lint test:unit` all green. (commit `846a671`)

### Cycle 254 — Apply failure lists sorted alphabetically in JSON + text

- [x] `failed_providers`, `skipped_providers`, `state_save_errors` in `hams apply --json` were emitted in DAG-iteration order. Kahn's topological sort (`internal/provider/dag.go:53`) runs `sort.Strings(queue)` on zero-indegree peers, so providers with no `DependsOn` come out alphabetical; but providers with dependency chains (e.g. `alpha` depends on `zeta`) emit in topo-order (zeta → alpha) — so `failed_providers: ["zeta", "alpha"]` instead of alphabetical. `probe_failed_providers` (cycle 232) and the text-mode failedProviders warning (cycle 235) already sorted; JSON `failed_providers` / `skipped_providers` / `state_save_errors` did not — an inconsistency that broke `diff` between two apply runs on DAG-chain configs. Fix: `sort.Strings(failedProviders); sort.Strings(skippedProviders); sort.Strings(stateSaveFailures)` once just before the `if flags.DryRun` branch so BOTH the dry-run path (through `emitDryRunJSON`) and the real-run path (through `buildApplyJSONSummary`) see the canonical alphabetical order. Drop the redundant local copy+sort in the text-mode `failedProviders` block (cycle 235) since the slice is now sorted upstream. Regression test `TestRunApply_JSONOutput_FailureListsAreSorted` constructs a DAG chain `zeta → alpha → beta` that forces DAG topo-sort order `zeta, alpha, beta` (non-alphabetical), runs apply with all three providers failing, asserts JSON `failed_providers = ["alpha", "beta", "zeta"]`. `task fmt lint test:unit` all green. (commit `0e00dde`)

### Cycle 255 — Hamsfile duplicate-app-across-tags validation (spec gap)

- [x] Schema-design spec §"Duplicate app identity across groups is rejected" mandates that a hamsfile declaring the same `app: git` under two tags SHALL exit with a validation error naming the duplicate and the tags. This was never implemented: `hamsfile.ListApps()` collected all apps without dedup, `ComputePlan` then silently folded duplicates via its within-loop `seen[id]` check, and the user never learned their edit was ambiguous. Drift attribution between tags became meaningless — state records one app but both tags "own" it. Fix: add `DuplicateAppError` + `(*File).ValidateNoDuplicateApps()` in `internal/hamsfile/hamsfile.go`. Walk every tag's sequence collecting apps; record which tags each app appears in (deduplicated within a tag, preserved across tags in document order); scan the doc a second time and return the FIRST cross-tag duplicate's `*DuplicateAppError{App, Tags}`. Same-tag repeats are intentionally NOT rejected — they fold via ComputePlan and rejecting mid-edit would break hand-fixing workflows. `runApply` calls `hf.ValidateNoDuplicateApps()` after the hamsfile Read + before `Plan`; on error, logs with slog.Error + appends to `skippedProviders` + continues so healthy providers still run. Three unit tests in hamsfile_test.go (`NoDupsReturnsNil`, `CrossTagDupRejected` asserting `.App="git"` and `.Tags=[dev-tool, term-tool]` in document order, `SameTagRepeatAccepted` boundary) + one integration test `TestRunApply_DuplicateAppAcrossTagsSkipsProvider` asserts `skipped_providers=["alpha"]` and that Plan was NOT called (validation short-circuits). `task fmt lint test:unit` all green.

## Rules

@.claude/rules/code-conventions.md
@.claude/rules/development-process.md
@.claude/rules/agent-behavior.md
@.claude/rules/docs-verification.md
