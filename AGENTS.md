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

**Filed for next session:** `extend-bootstrap-to-chainable-providers`
(proposal + design + specs + tasks drafted at
`openspec/changes/extend-bootstrap-to-chainable-providers/`).
Critical-architect + power-user Agent-team debate landed on Option
C narrow extension — 4 providers (pnpm, duti, mas, ansible) adopt
cycle-5's `BootstrapRequiredError` pattern; 7 skipped with recorded
reasoning. Architect explicitly recommended: file and stop reactive
cycling, implement in a fresh session with a clear head. Validated
clean via `openspec validate --strict`.

Archived 2026-04-16:
`homebrew-bootstrap-opt-in` — critical-architect review of the 4
archived cycles surfaced a real spec/code divergence in the Homebrew
provider: `builtin-providers/spec.md:375-378` said hams SHALL
auto-bootstrap `brew` via `depend-on: bash`, but `homebrew.go:60-66`
just returned an error and `DependOn.Script` had no caller anywhere
in the tree (dead data since v1).

Autonomous architect-role + user-role Agent debate (position papers
preserved verbatim in `openspec/changes/homebrew-bootstrap-opt-in/design.md`)
converged on Option C — **explicit opt-in**. Core arguments:

- Option A (silent auto-bootstrap) violates least astonishment +
  trust; `curl | bash` is blocked by corporate proxies; macOS
  Xcode-CLT dialog blocks stdin; integration tests already pre-install
  brew because the team itself didn't trust the auto-path.
- Option B (error-only) drops fresh-Mac users off a cliff on day one,
  killing the "one-command restore" promise on the flagship platform.
- Option C gives receipts AND convenience: default = actionable
  error with the exact script text; `--bootstrap` flag = non-interactive
  consent; TTY prompt = `[y/N/s]` with Xcode-CLT gotcha warning.

Implementation (4 commits + docs):

1. `feat(provider)`: `RunBootstrap(ctx, p, registry)` + `BashScriptRunner`
   interface + `WithBootstrapAllowed`/`BootstrapAllowed` ctx helpers +
   `ErrBootstrapRequired` sentinel + `BootstrapRequiredError` typed
   error. Bash provider implements `RunScript` via `/bin/bash -c <script>`
   with stdin/stdout/stderr passthrough + a DI seam.
2. `feat(homebrew)`: Bootstrap returns `*provider.BootstrapRequiredError`
   when `brew` is missing. Provider stays pure (no registry dependency).
   `brewBinaryLookup` var as PATH-check seam for tests.
3. `feat(cli)`: `--bootstrap` / `--no-bootstrap` flags; `resolveBootstrapConsent`
   decision matrix (deny-flag / allow-flag / non-TTY-deny / TTY-prompt);
   interactive prompt shows script + sudo/Xcode/proxy warnings + `[y/N/s]`;
   `s` = skip-provider-for-this-run; mutual-exclusion = exit 2.
4. `chore`: lint fixes (errcheck/gocritic/nolintlint/revive) across the
   new code; scope decision §4 to skip the full end-to-end integration
   variant (would duplicate build-time `install.sh` coverage + add
   ~5min per CI run against a network dependency; 16 unit tests already
   cover every branch of the consent matrix + delegation path).
5. `docs`: apply page + homebrew provider page + README (en + zh-CN)
   all describe the default/--bootstrap/TTY behavior + Xcode-CLT gotcha.

Verification: `task check` green (fmt/lint/test); `task ci:itest:run
PROVIDER=apt` green (regression check against modified bootstrap loop);
`task ci:itest:run PROVIDER=homebrew` green (main pre-installed path
still end-to-end: seed install → re-install → install-new → refresh →
remove-via-hamsfile-delete). 13 spec scenarios all mapped to named
unit tests in the archived `tasks.md §6.8`. Archived with spec
deltas hand-applied (same workaround as the prior 4 cycles for the
openspec auto-sync header-matching bug on MODIFIED blocks).

**Post-archive code-reviewer pass** (superpowers:code-reviewer)
surfaced one CRITICAL + two WARNINGs + one NIT. All four landed
in-session as atomic fixes (same post-merge-NIT pattern as cycle 4's
commit `95bd349`):

- `fix(apply)` commit `2c39ad5` — **C1** actionable error body
  (spec/code divergence: shipped `UserFacingError.Suggestions` didn't
  name the binary, script, or remedy that the spec scenario
  explicitly requires); **W2** skip-cascade (TTY `s` answer left
  DAG-dependent providers like vscodeext probing against a
  just-skipped brew prereq); **N3** platform-gate the Xcode-CLT
  warning on Linux hosts.
- `fix(homebrew)` commit `452f938` — **W1** PATH augmentation on
  the retry path. install.sh writes brew to `/opt/homebrew/bin`
  (Apple Silicon) or `/home/linuxbrew/.linuxbrew/bin` (Linuxbrew),
  neither on the hams process's $PATH. Without augmentation, every
  fresh-Mac `hams apply --bootstrap` would complete install.sh only
  to abort with "provider still unavailable after bootstrap". The
  existing integration test hid the gap by pre-setting PATH in
  `integration.sh:19`; real users don't get that. Fix augments
  os.Getenv("PATH") with the canonical install locations and
  re-checks before returning the error. Two new unit tests lock in
  both the success retry AND the still-missing fallback (no
  error-swallowing).

Net: 3 post-archive commits + 3 new unit tests for the code-reviewer
findings. All 4 reviewer findings addressed end-to-end.

---

Five cycles archived this session (2026-04-15 + 2026-04-16):

1. `fix-apt-cli-state-write-and-htop-rename` (2026-04-15) — apt CLI state-write + bat→htop rename + two-stage scope gate + per-provider docker integration matrix.
2. `clarify-apply-state-only-semantics` (2026-04-15) — `hams apply --prune-orphans` opt-in destructive reconciliation for state-only providers. Default skip preserved.
3. `apt-cli-complex-invocations` (2026-04-15) — apt CLI now auto-records `nginx=1.24.0` and `nginx/bookworm-backports` as structured `{app, version, source}` hamsfile entries on the imperative install path; state carries symmetric `requested_version` / `requested_source` fields.
4. `fix-apt-pin-apply-path` (2026-04-15) — closes cycle-3's three correctness gaps so pinning works on the **declarative + restore** paths too: Plan reads pins from the hamsfile via the new `(*File).AppFields(name)` helper; pinned actions carry the install token in `Action.Resource` (state stays keyed on the bare name); `AddAppWithFields` upgrades existing bare entries in place; executor populates `Action.StateOpts` so state records the pin after a successful install.

Codex review fed each cycle's design (5 rounds total). Each round surfaced P2 findings → architect+user agent debate → in-session fix or new openspec proposal. Pattern: rounds 1-3 narrowed-then-extended the apt auto-record contract until grammar-aware recording was a deliberate spec extension; round 4 closed the apply-path gap that the cycle-3 spec scenarios promised but the implementation didn't deliver; round 5 found two more cycle-4 gaps (Skip-without-drift loses pin on hash-promotion; multi-arch package syntax `pkg:arch` rejected by parser) and both landed in-session as cycle-4-spec-mandated correctness. Net: the canonical hams workflow (hand-edit YAML + apply, fresh-machine restore) now actually honors apt pins on every documented path.

A holistic outside code-review at session end (superpowers code-reviewer) confirmed: NO ship-blockers. The work is correct on every path the user will touch. Three NITs were noted around state-pin field residuals — all three landed in-session as commit `95bd349 fix(apt): clear pin fields on remove + unpin so audit trail stays truthful`:

- `hams apt install nginx=1.24.0` then `hams apt remove nginx` now clears `requested_version` on the StateRemoved row (no more lying audit trail).
- `hams apt remove nginx=1.24.0` (the symmetric install-token form) keys state on bare `nginx` (no orphan `nginx=1.24.0` row).
- Hand-edit unpin (`{app: nginx, version: "1.24.0"}` → `{app: nginx}` + apply) now clears the stale `requested_version` from state via Plan's Skip branch stamping explicit clears that fire on hash-promotion.

3 new unit tests (U36-U38) lock in the audit-truth invariant.

Reviewer's architectural retrospective: cycle 3 was under-scoped (assumed declarative path was "just plumbing", missed the `AppFields` API extension needed by Plan). Cycle 4 framed itself as cycle 3's correctness fix, but the archive structure presents them as peer features. Future improvement: scope the next pinning-shaped change "end-to-end across imperative + declarative + restore" in one spec rather than across two cycles.

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
