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
- [ ] 2.2 Implement `internal/config/` package: load global config (`~/.config/hams/hams.config.yaml`), project-level config, `.local.yaml` merge with 4-level precedence
- [ ] 2.3 Implement `internal/hamsfile/` package: YAML read/write with go-yaml v3 `yaml.Node` for comment preservation
- [ ] 2.4 Implement hamsfile `.local.yaml` merge engine with provider-registered merge strategies (append for lists, override for same-URN)
- [ ] 2.5 Implement atomic file writes (temp + fsync + rename) in hamsfile module
- [ ] 2.6 Implement `internal/state/` package: state file read/write with schema versioning
- [ ] 2.7 Implement lock manager: PID+cmd+timestamp lock file at `.state/<machine-id>/.lock`, stale lock detection via process liveness
- [ ] 2.8 Property-based tests: YAML round-trip fidelity (byte-identical for unchanged content), merge strategies, URN validation
- [ ] 2.9 Property-based tests: state file serialization/deserialization, lock file acquire/release/stale-reclaim

## 3. CLI Architecture

_Spec: `cli-architecture` — depends on schema (2.x) and project structure (1.x)._

- [ ] 3.1 Implement Cobra root command with Fx integration, global flag parsing (`--debug`, `--dry-run`, `--json`, `--no-color`, `--config`, `--store`, `--profile`, `--help`, `--version`)
- [ ] 3.2 Implement provider command routing: `hams <provider> <verb> <args>` dispatches to registered provider
- [ ] 3.3 Implement `--hams:` prefix flag extraction and `--` force-forward separator
- [ ] 3.4 Implement `--help` priority logic (position-dependent help display, highest priority)
- [ ] 3.5 Implement `internal/sudo/` package: one-time credential prompt at startup, 4-minute `sudo -v` heartbeat goroutine
- [ ] 3.6 Implement `internal/i18n/` package: `LC_ALL`/`LC_CTYPE`/`LANG` parsing, message catalog loading, `en_US` default
- [ ] 3.7 Implement exit code semantics (0/1/2/3/4/10/11-19/126/127) and AI-agent friendly error format (`code`/`message`/`suggestions`, `--json` mode)
- [ ] 3.8 Implement `hams apply` command: lock → sudo → load config → refresh → diff → execute → update state → release lock
- [ ] 3.9 Implement `hams apply --from-repo=<repo>` bootstrap flow: GitHub shorthand prefixing, go-git clone to `HAMS_DATA_HOME/repo/`, interactive profile init prompt
- [ ] 3.10 Implement `hams refresh` command: probe known resources only, `--only`/`--except` provider filtering
- [ ] 3.11 Implement `hams apply --only`/`--except` provider filtering (mutually exclusive, case-insensitive)
- [ ] 3.12 Implement `hams config` subcommands: `get`, `set`, `list`, `edit` (sensitive values → `.local.yaml`/keychain)
- [ ] 3.13 Implement `hams store` subcommand: show store directory path and status
- [ ] 3.14 Implement `hams list` subcommand: grouped by provider, status filter, JSON output
- [ ] 3.15 Implement `hams self-upgrade` command: detect install channel (marker file / binary path), route to GitHub Releases download or `brew upgrade`
- [ ] 3.16 Implement `--dry-run` mode: read-only plan display, no mutations, no lock
- [ ] 3.17 Tests for command routing, flag parsing, help priority, exit codes

## 4. Provider System

_Spec: `provider-system` — depends on schema (2.x) and CLI (3.1-3.3). Core framework before any builtin._

- [ ] 4.1 Define `Provider` interface: Register, Bootstrap, Probe, Plan, Apply, Remove, List, Enrich methods
- [ ] 4.2 Implement provider registry: registration, discovery, manifest validation
- [ ] 4.3 Implement depend-on DAG resolver: topological sort, platform-conditional filtering, cycle detection with error reporting
- [ ] 4.4 Implement provider execution priority: DAG-level then priority-list ordering (configurable in hams.config.yaml)
- [ ] 4.5 Implement probe dispatcher: parallel probe across providers (errgroup), 4 resource class strategies
- [ ] 4.6 Implement plan engine: desired (hamsfile) vs observed (state) diff → action list (install/update/remove/skip)
- [ ] 4.7 Implement apply executor: sequential per-provider, sequential per-resource within provider, write-serial global mutex
- [ ] 4.8 Implement hook engine: pre/post-install hooks, `defer: true` batching (within current provider), nested provider calls, hook-failed state tracking
- [ ] 4.9 Implement remove flow: delete hamsfile entry → execute remove command → state marked `Removed` (kept for audit)
- [ ] 4.10 Implement provider CLI wrapping framework: verb routing (hams-interpreted vs passthrough), auto-inject flag engine, `--hams:` prefix extraction
- [ ] 4.11 Design go-plugin extension interface (`pkg/sdk/`): gRPC service definition, plugin discovery paths, subprocess lifecycle, crash handling with backoff
- [ ] 4.12 Property-based tests: DAG resolution (acyclic/cyclic/single-node/diamond), hook nesting, plan diffing

## 5. TUI & Logging

_Spec: `tui-logging` — depends on CLI (3.1) for integration. Can be built in parallel with provider system._

- [ ] 5.1 Implement `internal/logging/` package: structured slog setup, log file rotation (`HAMS_DATA_HOME/<YYYY-MM>/hams.YYYYMM.log`), session log linking
- [ ] 5.2 Implement third-party session log manager: create `provider/<provider>.YYYYMMDDTHHmmss.session.log`, link from main log by session ID
- [ ] 5.3 Implement output path tilde prefix: replace `$HOME` with `~/` in all displayed paths
- [ ] 5.4 Implement `internal/tui/` package: BubbleTea alternate screen with sticky top (log file path), provider step progress (current/total), current operation
- [ ] 5.5 Implement collapsible log output sections in TUI
- [ ] 5.6 Implement interactive popup (tmux-popup style): provider interactive API, stdin passthrough, popup lifecycle
- [ ] 5.7 Implement `internal/notify/` package: terminal-notifier (mandatory), Bark (optional, token from `.local.yaml`/keychain)
- [ ] 5.8 Implement notification triggers: apply completion, blocking interactive action
- [ ] 5.9 Implement non-TUI fallback: detect non-TTY, plain text structured log output, no ANSI codes
- [ ] 5.10 Implement `--debug` flag: verbose output (provider traces, state diffs, DAG resolution, hook lifecycle, LLM calls)
- [ ] 5.11 Implement graceful Ctrl+C shutdown: context cancellation, 5-second timeout, state save, summary, terminal restore

## 6. Observability (OTel)

_Spec: `observability` — depends on CLI (3.1) for Fx init. Integrates into provider system._

- [ ] 6.1 Implement `internal/otel/` package: Fx module for TracerProvider + MeterProvider init, no-op for non-instrumented commands
- [ ] 6.2 Implement local file exporter: JSON-encoded OTLP to `HAMS_DATA_HOME/otel/{traces,metrics}/`
- [ ] 6.3 Implement trace spans: root (apply/refresh), child (provider), grandchild (resource operation) with attributes
- [ ] 6.4 Implement metrics: `hams.apply.duration`, `hams.provider.failures`, `hams.resources.total`, `hams.probe.duration`
- [ ] 6.5 Implement graceful shutdown flush (Fx OnStop, 5-second hard timeout)
- [ ] 6.6 Implement tail-sampling for large applies (threshold configurable, retain all failed spans)
- [ ] 6.7 Design exporter interface for future OTLP extensibility

## 7. Builtin Providers (Phase 1: Core)

_Spec: `builtin-providers` — depends on provider system (4.x). Priority order for implementation._

- [ ] 7.1 Implement `bash` provider: URN-based scripts, `check:` field, `bash.hams/` subdirectory support, step/description naming
- [ ] 7.2 Implement `Homebrew` provider: core + cask + tap in one file, `--cask` flag handling, formula `desc` fetching for LLM enrichment, depend-on bash (curl|bash installer)
- [ ] 7.3 Implement `apt` provider: auto-inject `-y`, sudo-required, Linux-only platform filter
- [ ] 7.4 Implement `pnpm` provider: auto-inject `--global`, depend-on npm for pnpm install
- [ ] 7.5 Implement `npm` provider: auto-inject `--global`
- [ ] 7.6 Implement `git config` provider: KV config class, `--global`/`--file` support, check via `git config --get`, conditional includes
- [ ] 7.7 Implement `git clone` provider: record remote→local-path→default-branch, check = path exists only
- [ ] 7.8 Implement `defaults` provider: `defaults write/read/delete`, macOS-only, killall post-hooks for Dock/Finder
- [ ] 7.9 Property-based tests for each Phase 1 provider: probe round-trip, hamsfile serialization, idempotency

## 8. Builtin Providers (Phase 2: Extended)

_Spec: `builtin-providers` — can start after Phase 1 establishes the pattern._

- [ ] 8.1 Implement `uv` provider: `uv tool install/uninstall`
- [ ] 8.2 Implement `go` provider: `go install`, auto-inject `@latest` if no version specified
- [ ] 8.3 Implement `cargo` provider: `cargo install/uninstall`
- [ ] 8.4 Implement `vscode-ext` provider: `code --install-extension/--uninstall-extension`, depend-on Homebrew (visual-studio-code cask)
- [ ] 8.5 Implement `mas` provider: `mas install/uninstall`, numeric app IDs, macOS-only, signin handling via interactive popup
- [ ] 8.6 Implement `duti` provider: default app associations, `duti -x` check, macOS-only
- [ ] 8.7 Implement `Ansible` provider: playbook paths + categories, `ansible-playbook` wrapping, depend-on for ansible CLI
- [ ] 8.8 Property-based tests for each Phase 2 provider

## 9. LLM Integration

_Cross-cutting — depends on provider system (4.x) and hamsfile SDK (2.x)._

- [ ] 9.1 Implement LLM subprocess caller: invoke configured CLI (claude/codex) from `hams.config.yaml`, timeout handling, graceful degradation
- [ ] 9.2 Implement tag recommendation: pass package name + desc + existing tags to LLM, parse response
- [ ] 9.3 Implement intro generation: pass package name + desc to LLM, parse response
- [ ] 9.4 Implement async enrichment flow: parallel goroutine during install, write back to hamsfile via SDK, error reporting at apply end
- [ ] 9.5 Implement `--hams:lucky` flag: auto-accept all LLM recommendations without TUI picker
- [ ] 9.6 Implement per-provider `enrich` standalone command (e.g., `hams brew enrich <app>`)
- [ ] 9.7 Implement tag TUI multi-select picker: LLM-recommended (pre-selected), existing tags, free-text input

## 10. Documentation Site

_Spec: `docs-site` — independent, can start after specs stabilize._

- [ ] 10.1 Scaffold Nextra project in `docs/` with dark-mode theme, sidebar navigation, search (Flexsearch)
- [ ] 10.2 Write "Why / Motivation" page: comparison table, "what hams is NOT" section
- [ ] 10.3 Write "Quickstart / Install" page: curl|bash, brew tap, binary download, first `hams apply --from-repo=` walkthrough
- [ ] 10.4 Write "CLI Reference" pages: every subcommand with syntax, flags, examples
- [ ] 10.5 Write "Builtin Provider Catalog" pages: per-provider page with store schema, commands, examples
- [ ] 10.6 Write "Schema Reference" page: annotated YAML examples for hams.config, hamsfile, state
- [ ] 10.7 Write "Provider API" page: Go SDK guide, go-plugin extension, resource classes, minimal example
- [ ] 10.8 Configure GitHub Pages deployment with CNAME `hams.zthxxx.me`
- [ ] 10.9 Set up i18n structure for Chinese translation (extensible)

## 11. E2E & Release

_Depends on all above being substantially complete._

- [ ] 11.1 Write e2e test: fresh Debian container → `install.sh` → `hams apply --from-repo=` fixture repo → verify all providers installed
- [ ] 11.2 Write e2e test: fresh Alpine container → same flow
- [ ] 11.3 Write e2e test: OpenWrt-like container → bash + apt providers only
- [ ] 11.4 Create Homebrew tap formula (`zthxxx/tap/hams`)
- [ ] 11.5 Set up GitHub Actions release workflow: goreleaser / manual cross-compile → GitHub Releases with checksums
- [ ] 11.6 Final integration test: macOS → `hams apply` with a real hams-store repo (manual, not CI)

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
