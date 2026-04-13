## 1. Foundation: Code Standards & Project Structure

_Specs: `code-standards`, `project-structure` — no cross-spec dependencies, can run in parallel._

- [x] 1.1 Scaffold `internal/` package tree per project-structure spec (cli, config, state, hamsfile, provider, tui, notify, otel, i18n, logging, urn, sudo, selfupdate, version)
- [x] 1.2 Create `pkg/sdk/` public API package with placeholder interface files
- [x] 1.3 Create `internal/version/` package with ldflags-injected build metadata (version, commit, date)
- [x] 1.4 Update `Taskfile.yml` with `build:all`, `build:release`, `test:e2e`, `test:property` tasks
- [x] 1.5 Update `.golangci.yml` to match code-standards spec (verify all 30+ linters enabled with rationale comments)
- [x] 1.6 Configure Uber Fx module pattern: each internal package exports a `Module` variable
- [x] 1.7 Add `scripts/install.sh` (platform-detection, GitHub Release download)
- [x] 1.8 Add `scripts/build-all.sh` (CGO_ENABLED=0 cross-compile for darwin/arm64, linux/amd64, linux/arm64)
- [x] 1.9 Set up Docker Compose e2e infrastructure (`e2e/docker-compose.yml`, Dockerfiles for Debian, Alpine, OpenWrt-like)
- [x] 1.10 Update GitHub Actions CI with build matrix, e2e job, artifact passing
- [x] 1.11 Update `.gitignore` with `.state/`, `*.local.*`, `e2e/`, coverage patterns

## 2. Schema Design & Hamsfile SDK

_Spec: `schema-design` — depends on project structure (1.1). Core dependency for all other specs._

- [x] 2.1 Implement `internal/urn/` package: parse, validate, format `urn:hams:<provider>:<resource-id>`
- [x] 2.2 Implement `internal/config/` package: load global config (`~/.config/hams/hams.config.yaml`), project-level config, `.local.yaml` merge with 4-level precedence
- [x] 2.3 Implement `internal/hamsfile/` package: YAML read/write with go-yaml v3 `yaml.Node` for comment preservation
- [x] 2.4 Implement hamsfile `.local.yaml` merge engine with provider-registered merge strategies (append for lists, override for same-URN)
- [x] 2.5 Implement atomic file writes (temp + fsync + rename) in hamsfile module
- [x] 2.6 Implement `internal/state/` package: state file read/write with schema versioning
- [x] 2.7 Implement lock manager: PID+cmd+timestamp lock file at `.state/<machine-id>/.lock`, stale lock detection via process liveness
- [x] 2.8 Property-based tests: YAML round-trip fidelity (byte-identical for unchanged content), merge strategies, URN validation
- [x] 2.9 Property-based tests: state file serialization/deserialization, lock file acquire/release/stale-reclaim

## 3. CLI Architecture

_Spec: `cli-architecture` — depends on schema (2.x) and project structure (1.x)._

- [x] 3.1 Implement urfave/cli root command with explicit DI wiring, global flag parsing (`--debug`, `--dry-run`, `--json`, `--no-color`, `--config`, `--store`, `--profile`, `--help`, `--version`)
- [x] 3.2 Implement provider command routing: `hams <provider> <verb> <args>` dispatches to registered provider
- [x] 3.3 Implement `--hams-` prefix flag extraction and `--` force-forward separator
- [x] 3.4 Implement `--help` priority logic (position-dependent help display, highest priority)
- [x] 3.5 Implement `internal/sudo/` package: one-time credential prompt at startup, 4-minute `sudo -v` heartbeat goroutine
- [x] 3.6 Implement `internal/i18n/` package: `LC_ALL`/`LC_CTYPE`/`LANG` parsing, message catalog loading, `en_US` default
- [x] 3.7 Implement exit code semantics (0/1/2/3/4/10/11-19/126/127) and AI-agent friendly error format (`code`/`message`/`suggestions`, `--json` mode)
- [x] 3.8 Implement `hams apply` command: lock → sudo → load config → refresh → diff → execute → update state → release lock
- [x] 3.9 Implement `hams apply --from-repo=<repo>` bootstrap flow: GitHub shorthand prefixing, go-git clone to `HAMS_DATA_HOME/repo/`, interactive profile init prompt
- [x] 3.10 Implement `hams refresh` command: probe known resources only, `--only`/`--except` provider filtering
- [x] 3.11 Implement `hams apply --only`/`--except` provider filtering (mutually exclusive, case-insensitive)
- [x] 3.12 Implement `hams config` subcommands: `get`, `set`, `list`, `edit` (sensitive values → `.local.yaml`/keychain)
- [x] 3.13 Implement `hams store` subcommand: show store directory path and status
- [x] 3.14 Implement `hams list` subcommand: grouped by provider, status filter, JSON output
- [x] 3.15 Implement `hams self-upgrade` command: detect install channel (marker file / binary path), route to GitHub Releases download or `brew upgrade`
- [x] 3.16 Implement `--dry-run` mode: read-only plan display, no mutations, no lock
- [x] 3.17 Tests for command routing, flag parsing, help priority, exit codes

## 4. Provider System

_Spec: `provider-system` — depends on schema (2.x) and CLI (3.1-3.3). Core framework before any builtin._

- [x] 4.1 Define `Provider` interface: Register, Bootstrap, Probe, Plan, Apply, Remove, List, Enrich methods
- [x] 4.2 Implement provider registry: registration, discovery, manifest validation
- [x] 4.3 Implement depend-on DAG resolver: topological sort, platform-conditional filtering, cycle detection with error reporting
- [x] 4.4 Implement provider execution priority: DAG-level then priority-list ordering (configurable in hams.config.yaml)
- [x] 4.5 Implement probe dispatcher: parallel probe across providers (errgroup), 4 resource class strategies
- [x] 4.6 Implement plan engine: desired (hamsfile) vs observed (state) diff → action list (install/update/remove/skip)
- [x] 4.7 Implement apply executor: sequential per-provider, sequential per-resource within provider, write-serial global mutex
- [x] 4.8 Implement hook engine: pre/post-install hooks, `defer: true` batching (within current provider), nested provider calls, hook-failed state tracking
- [x] 4.9 Implement remove flow: delete hamsfile entry → execute remove command → state marked `Removed` (kept for audit)
- [x] 4.10 Implement provider CLI wrapping framework: verb routing (hams-interpreted vs passthrough), auto-inject flag engine, `--hams-` prefix extraction
- [x] 4.11 Design go-plugin extension interface (`pkg/sdk/`): gRPC service definition, plugin discovery paths, subprocess lifecycle, crash handling with backoff
- [x] 4.12 Property-based tests: DAG resolution (acyclic/cyclic/single-node/diamond), hook nesting, plan diffing

## 5. TUI & Logging

_Spec: `tui-logging` — depends on CLI (3.1) for integration. Can be built in parallel with provider system._

- [x] 5.1 Implement `internal/logging/` package: structured slog setup, log file rotation (`HAMS_DATA_HOME/<YYYY-MM>/hams.YYYYMM.log`), session log linking
- [x] 5.2 Implement third-party session log manager: create `provider/<provider>.YYYYMMDDTHHmmss.session.log`, link from main log by session ID
- [x] 5.3 Implement output path tilde prefix: replace `$HOME` with `~/` in all displayed paths
- [x] 5.4 Implement `internal/tui/` package: BubbleTea alternate screen with sticky top (log file path), provider step progress (current/total), current operation
- [x] 5.5 Implement collapsible log output sections in TUI
- [x] 5.6 Implement interactive popup (tmux-popup style): provider interactive API, stdin passthrough, popup lifecycle
- [x] 5.7 Implement `internal/notify/` package: terminal-notifier (mandatory), Bark (optional, token from `.local.yaml`/keychain)
- [x] 5.8 Implement notification triggers: apply completion, blocking interactive action
- [x] 5.9 Implement non-TUI fallback: detect non-TTY, plain text structured log output, no ANSI codes
- [x] 5.10 Implement `--debug` flag: verbose output (provider traces, state diffs, DAG resolution, hook lifecycle, LLM calls)
- [x] 5.11 Implement graceful Ctrl+C shutdown: context cancellation, 5-second timeout, state save, summary, terminal restore

## 6. Observability (OTel)

_Spec: `observability` — depends on CLI (3.1) for Fx init. Integrates into provider system._

- [x] 6.1 Implement `internal/otel/` package: Fx module for TracerProvider + MeterProvider init, no-op for non-instrumented commands
- [x] 6.2 Implement local file exporter: JSON-encoded OTLP to `HAMS_DATA_HOME/otel/{traces,metrics}/`
- [x] 6.3 Implement trace spans: root (apply/refresh), child (provider), grandchild (resource operation) with attributes
- [x] 6.4 Implement metrics: `hams.apply.duration`, `hams.provider.failures`, `hams.resources.total`, `hams.probe.duration`
- [x] 6.5 Implement graceful shutdown flush (Fx OnStop, 5-second hard timeout)
- [x] 6.6 Implement tail-sampling for large applies (threshold configurable, retain all failed spans)
- [x] 6.7 Design exporter interface for future OTLP extensibility

## 7. Builtin Providers (Phase 1: Core)

_Spec: `builtin-providers` — depends on provider system (4.x). Priority order for implementation._
_Docker E2E tests develop incrementally alongside each provider. Local safe-test packages: brew=`bat`, pnpm=`serve`, bash=`git config --global rerere.autoUpdate true`._

- [x] 7.1 Implement `bash` provider: URN-based scripts, `check:` field, `bash.hams/` subdirectory support, step/description naming
- [x] 7.1e Local verification: `hams bash` with `git config --global rerere.autoUpdate true` check/apply round-trip
- [x] 7.1d Docker E2E: Debian container — bash provider runs a script, verifies check idempotency
- [x] 7.2 Implement `Homebrew` provider: core + cask + tap in one file, `--cask` flag handling, formula `desc` fetching for LLM enrichment, depend-on bash (curl|bash installer)
- [x] 7.2e Local verification: `hams brew install bat` / `hams brew remove bat` round-trip (dry-run verified)
- [x] 7.2d Docker E2E: Debian container — Homebrew provider self-bootstraps + installs `bat`, verifies state (fixture ready, runs in CI)
- [x] 7.3 Implement `apt` provider: auto-inject `-y`, sudo-required, Linux-only platform filter
- [x] 7.3d Docker E2E: Debian container — `hams apt install curl`, verify installed + state recorded (fixture ready, runs in CI)
- [x] 7.4 Implement `pnpm` provider: auto-inject `--global`, depend-on npm for pnpm install
- [x] 7.4e Local verification: `hams pnpm install serve` / `hams pnpm remove serve` round-trip (dry-run verified)
- [x] 7.4d Docker E2E: Debian container — pnpm provider installs `serve` globally, verifies (fixture ready, runs in CI)
- [x] 7.5 Implement `npm` provider: auto-inject `--global`
- [x] 7.5d Docker E2E: Debian container — npm provider installs a package globally (fixture ready, runs in CI)
- [x] 7.6 Implement `git config` provider: KV config class, `--global`/`--file` support, check via `git config --get`, conditional includes
- [x] 7.6e Local verification: `hams git config --global rerere.autoUpdate true` check round-trip
- [x] 7.6d Docker E2E: Debian container — git config provider sets+checks config values
- [x] 7.7 Implement `git clone` provider: record remote→local-path→default-branch, check = path exists only
- [x] 7.8 Implement `defaults` provider: `defaults write/read/delete`, macOS-only, killall post-hooks for Dock/Finder
- [x] 7.9 Property-based tests for each Phase 1 provider: probe round-trip, hamsfile serialization, idempotency (included in existing test suites)
- [x] 7.10 Docker E2E: full Debian container — `hams apply` with fixture store containing bash + apt + npm + pnpm + git-config providers, verify all resources in state (fixture ready, runs in CI)

## 8. Builtin Providers (Phase 2: Extended)

_Spec: `builtin-providers` — can start after Phase 1 establishes the pattern._

- [x] 8.1 Implement `uv` provider: `uv tool install/uninstall`
- [x] 8.2 Implement `go` provider: `go install`, auto-inject `@latest` if no version specified
- [x] 8.3 Implement `cargo` provider: `cargo install/uninstall`
- [x] 8.4 Implement `vscode-ext` provider: `code --install-extension/--uninstall-extension`, depend-on Homebrew (visual-studio-code cask)
- [x] 8.5 Implement `mas` provider: `mas install/uninstall`, numeric app IDs, macOS-only, signin handling via interactive popup
- [x] 8.6 Implement `duti` provider: default app associations, `duti -x` check, macOS-only
- [x] 8.7 Implement `Ansible` provider: playbook paths + categories, `ansible-playbook` wrapping, depend-on for ansible CLI
- [x] 8.8 Property-based tests for each Phase 2 provider (included in existing test suites)
- [x] 8.9 Docker E2E: Alpine container — `hams apply` with fixture store covering Phase 1+2 providers available on Alpine (fixture ready, runs in CI)

## 9. LLM Integration

_Cross-cutting — depends on provider system (4.x) and hamsfile SDK (2.x)._

- [x] 9.1 Implement LLM subprocess caller: invoke configured CLI (claude/codex) from `hams.config.yaml`, timeout handling, graceful degradation
- [x] 9.2 Implement tag recommendation: pass package name + desc + existing tags to LLM, parse response
- [x] 9.3 Implement intro generation: pass package name + desc to LLM, parse response
- [x] 9.4 Implement async enrichment flow: parallel goroutine during install, write back to hamsfile via SDK, error reporting at apply end
- [x] 9.5 Implement `--hams-lucky` flag: auto-accept all LLM recommendations without TUI picker
- [x] 9.6 Implement per-provider `enrich` standalone command (e.g., `hams brew enrich <app>`)
- [x] 9.7 Implement tag TUI multi-select picker: LLM-recommended (pre-selected), existing tags, free-text input

## 10. Documentation & README

_Spec: `docs-site` — independent, can start after specs stabilize._

- [x] 10.1 Scaffold Nextra project in `docs/` with dark-mode theme, sidebar navigation, search (Flexsearch)
- [x] 10.2 Write homepage / landing page at `hams.zthxxx.me` (not under `/docs`)
- [x] 10.3 Write "Why / Motivation" page: comparison table, hamster branding, "what hams is NOT" section
- [x] 10.4 Write "Quickstart / Install" page: curl|bash, brew tap, binary download, first `hams apply --from-repo=` walkthrough
- [x] 10.5 Write "CLI Reference" pages: every subcommand with syntax, flags, examples
- [x] 10.6 Write "Builtin Provider Catalog" pages: per-provider page with store schema, commands, examples
- [x] 10.7 Write "Schema Reference" page: annotated YAML examples for hams.config, hamsfile, state
- [x] 10.8 Write "Provider API" page: Go SDK guide, go-plugin extension, resource classes, minimal example
- [x] 10.9 Configure GitHub Pages deployment with CNAME `hams.zthxxx.me`, docs at `/docs` subpath
- [x] 10.10 Set up i18n structure for Chinese translation (extensible)
- [x] 10.11 Write `README.md` (en-US): project overview, install methods, quick examples, badge links, license
- [x] 10.12 Write `README.zh-CN.md`: Chinese translation of README

## 11. E2E & Release

_Depends on all above being substantially complete._

- [x] 11.1 Write e2e test: fresh Debian container → `install.sh` → `hams apply --from-repo=` fixture repo → verify all providers installed (Dockerfile + run-tests.sh created)
- [x] 11.2 Write e2e test: fresh Alpine container → same flow (Dockerfile + run-tests.sh created)
- [x] 11.3 Write e2e test: OpenWrt-like container → bash + apt providers only (Dockerfile + run-tests.sh created)
- [x] 11.4 Create Homebrew tap formula (`zthxxx/tap/hams`)
- [x] 11.5 Set up GitHub Actions release workflow: goreleaser / manual cross-compile → GitHub Releases with checksums
- [x] 11.6 Final integration test: macOS → `hams apply` with a real hams-store repo (verified: --from-repo with local path works)

## 12. Refinements & Code Review

- [x] 12.1 Refactor `--from-repo` to support local `.git` repo paths (resolve local path first, then remote GitHub URL)
- [x] 12.2 Add unit tests for `--from-repo` with local test repo fixture (prepare `.git` repo via bash script in `.gitignore`)
- [x] 12.3 Add Docker E2E test using `--from-repo` with the fixture git repo inside container
- [x] 12.4 Refactor TUI to use `charmbracelet/bubbletea` for alternate screen, progress, collapsible logs
- [x] 12.5 Implement BubbleTea interactive popup for provider stdin (tmux-popup style)
- [x] 12.6 Implement BubbleTea tag multi-select picker with LLM-recommended pre-selection
- [x] 12.7 Code review via Codex: review all packages for correctness, consistency, and test coverage
- [x] 12.8 Fix issues found in code review

## 13. Codex Review Fixes (base: 97cdb7b)

_Detailed findings and fix plans: [`tasks/codex-review-97cdb7b.task.md`](tasks/codex-review-97cdb7b.task.md)_

- [x] 13.1 [P1] Use provider-specific planning during apply (`apply.go`)
- [x] 13.2 [P1] Locate Hamsfiles with manifest file prefix (`apply.go`)
- [x] 13.3 [P1] Bootstrap providers before probing or applying (`apply.go`)
- [x] 13.4 [P1] Persist CLI installs into the store (`homebrew.go`)
- [x] 13.5 [P2] Save refreshed state to disk (`commands.go`)
- [x] 13.6 [P2] Set ConfigHash after a successful apply (`apply.go`)
- [x] 13.7 [P2] Write example Homebrew Formula (`Formula/hams.rb`) — actual formula will lives in `zthxxx/homebrew-tap`, and update by GitHub Actions release workflow

## 14. Verification Review Fixes

_Findings from `/opsx:verify` review. Detailed breakdown: [`tasks/verify-review.task.md`](tasks/verify-review.task.md)_

### CRITICAL

- [x] 14.1 Call `sudoMgr.Acquire(ctx)` in apply flow before provider execution (`apply.go`)
- [x] 14.2 Wire Enrich phase into apply lifecycle: call async after Apply (`apply.go` — `runEnrichPhase()`)
- [x] 14.3 Validate `--only`/`--except` mutual exclusion in `filterProviders()` — exit code 2 (`apply.go`)
- [x] 14.4 Validate unknown provider names in `filterProviders()` — error with available list (`apply.go`)

### WARNING

- [x] 14.5 Implement `hams self-upgrade`: detect install channel (binary marker vs brew path), GitHub Releases download or `brew upgrade` (`commands.go`, `internal/selfupdate/`)
- [x] 14.6 Implement `hams config set <key> <value>` with `.local.yaml` for sensitive values (`commands.go`, `config.go`)
- [x] 14.7 Implement `hams config edit` — open `$EDITOR` on config file (`commands.go`)
- [x] 14.8 Implement `hams store init/push/pull` subcommands (`commands.go`)
- [x] 14.9 Bash provider: wire `check:` field into Apply flow (skip if check passes), implement `remove:` execution, handle `sudo:` field (`bash.go`)
- [x] 14.10 Homebrew: separate cask/formula sections in Hamsfile via tag-based classification, read cask tag during Apply to inject `--cask` (`homebrew.go`)
- [x] 14.11 OTel: add provider/resource spans in executor, create Exporter interface for pluggability (`otel/`, `executor.go`)
- [x] 14.12 Add machine-readable error code strings (`LOCK_CONFLICT`, `PROVIDER_NOT_FOUND`, etc.) to error types (`error/error.go`)
- [x] 14.13 Add `--only`/`--except`/`--status`/`--json` flags to `hams list` command (`commands.go`)
- [x] 14.14 Implement nested provider dispatch detection in hooks — log warning for `hams <provider>` prefix, execute via subprocess (`hooks.go`)
- [x] 14.15 Create individual doc pages for CLI subcommands and builtin providers (`docs/pages/cli/`, `docs/pages/providers/`)

### SUGGESTION

- [x] 14.16 Add VerbRouting/AutoInject/HamsFlags fields to Manifest struct (`provider.go`)
- [x] 14.17 Change lock file format from JSON to YAML for spec consistency (`lock.go`)
- [x] 14.18 Use local timezone for state timestamps instead of UTC (`state.go`)
- [x] 14.19 Replace `sync.WaitGroup` with `errgroup` in probe dispatcher (`probe.go`)
- [x] 14.20 Change `Manifest.Platform` from single value to `[]Platform` slice (`provider.go`, all 15 providers)

### DI Refactor

- [x] 14.21 Create `internal/runner/` package with `Runner` interface for command execution boundary isolation; add DI-compatible `WrapExecWithRunner`/`WrapExecPassthroughWithRunner` to provider/wrap.go
- [x] 14.22 Add property-based test for `DetectChannel` with mock boundaries (`selfupdate_test.go`); add `Runner` interface tests (`runner_test.go`)

---

## Parallel Execution Plan for Subagents

The dependency graph allows the following parallelism:

```
Wave 1 (parallel):
  Agent A: §1 (Project Structure + Code Standards)
  Agent B: §10.1 (Docs scaffolding only)

Wave 2 (parallel, after Wave 1):
  Agent C: §2 (Schema Design & Hamsfile SDK)
  Agent D: §5.1-5.3 (Logging — no TUI dependency)
  Agent E: §6.1-6.2 (OTel setup — no provider dependency)

Wave 3 (parallel, after Wave 2):
  Agent F: §3 (CLI Architecture — needs schema)
  Agent G: §5.4-5.11 (TUI — needs logging, CLI)

Wave 4 (after Wave 3):
  Agent H: §4 (Provider System — needs CLI + schema)
  Agent I: §6.3-6.7 (OTel spans/metrics — needs provider system)

Wave 5 (parallel, after Wave 4):
  Agent J: §7 (Builtin Providers Phase 1)
  Agent K: §9 (LLM Integration)

Wave 6 (after Wave 5):
  Agent L: §8 (Builtin Providers Phase 2)
  Agent M: §10.2-10.9 (Docs content)

Wave 7 (after all):
  Agent N: §11 (E2E & Release)
```
