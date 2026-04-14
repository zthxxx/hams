## Context

hams is a greenfield Go CLI tool that wraps existing package managers to auto-record installations into declarative YAML files ("Hamsfiles"), enabling one-command environment restoration on new machines. The full set of design decisions (Q1-Q19) is documented in `CLAUDE.md` (AGENTS.md) at the project root.

This design document covers **cross-cutting architectural concerns** and the **dependency graph between the 9 orthogonal specs**. Each spec contains its own detailed design.

### Current State

The project has only scaffolding: `cmd/hams/main.go` with an Uber Fx bootstrap, `internal/` (empty), tooling configs (golangci-lint, lefthook, CI), and this openspec change. All application logic is to be built.

### Constraints

- **Single static binary**: no dynamic libs, must run on ARM64 OpenWrt, Apple Silicon macOS, x86_64 Debian/Alpine.
- **Terraform-style state model**: local state in `.state/<machine-id>/`, single-writer lock, refresh-then-diff.
- **Provider plugin architecture**: builtins compiled in (Go), externals via `hashicorp/go-plugin` (local gRPC).
- **YAML comment preservation**: all Hamsfile and state file I/O must preserve user comments.
- **Bundled go-git**: for fresh-machine bootstrap without system git.
- **i18n**: `LC_ALL`/`LANG` parsing, `en_US` default.
- **AI-agent friendly CLI output**: structured errors, suggested fix commands, `--help` on every subcommand.

## Goals / Non-Goals

**Goals:**

- Build a working `hams` CLI that can install/remove/list/apply/refresh across multiple providers.
- Establish the provider interface so that all 15 builtin providers can be implemented.
- Deliver a TUI with progress tracking, collapsible logs, and interactive popup for blocking operations.
- Provide Terraform-style state tracking with drift detection, failure recovery, and single-writer locking.
- Ship a documentation site with quickstart, CLI reference, and provider authoring guide.
- Set up OTel trace + metrics for local debugging.
- Establish strict Go code standards suitable for a world-class open-source project.

**Non-Goals:**

- **Not a Docker/CI replacement**: hams targets interactive workstations, not containerized CI pipelines.
- **Not NixOS-level isolation**: no sandboxing, no hermetic builds, no version pinning by default.
- **No remote state backend (v1)**: state is local-only; optional git backend is designed but not implemented in v1.
- **No OTLP remote export (v1)**: OTel data stays local.
- **No parallel provider execution (v1)**: providers execute sequentially (write-serial); architecture preserves future parallelism.
- **No Bun/TS SDK (v1)**: only Go SDK for provider authoring.
- **No file-level include/reuse across profiles**: dropped from scope.

## Decisions

### D1. Spec Decomposition Strategy

**Decision**: 9 orthogonal specs, each independently writable by a subagent.

**Rationale**: The system decomposes naturally along these boundaries because each spec has a well-defined interface contract with others (e.g., Provider System defines the interface, Builtin Providers implement it; Schema Design defines the file formats, CLI Architecture reads/writes them). This enables parallel development with clear integration points.

**Dependency graph:**

```
project-structure ──┐
                    ├──► cli-architecture ──┐
schema-design ──────┤                      ├──► builtin-providers
                    ├──► provider-system ───┘
                    │
tui-logging ────────┘

code-standards ─────── (independent, informs all specs)
observability ──────── (independent, integrates into cli-architecture and provider-system)
docs-site ─────────── (independent, written after all other specs are stable)
```

### D1b. CLI Framework: urfave/cli over spf13/cobra

**Decision**: Use `urfave/cli` (v3) instead of `spf13/cobra` for CLI command definitions.

**Rationale**: urfave/cli uses declarative struct-based command definitions (`*cli.Command` structs with `Commands` slices) which are easier to compose dynamically — critical for hams where provider commands are registered at runtime. Cobra requires imperative `AddCommand` calls and has a more complex flag inheritance model that conflicts with `DisableFlagParsing` needed for provider passthrough. urfave/cli's `SkipFlagParsing` is cleaner.

**Alternatives considered**: Cobra (original implementation) — worked but required manual global flag stripping and had awkward persistent flag inheritance with disabled flag parsing on child commands.

### D1c. Explicit DI over Uber Fx

**Decision**: Use explicit constructor-based dependency wiring instead of Uber Fx's container-based DI.

**Rationale**: The application's dependency graph is straightforward and static — provider registry, config, state, hamsfile SDK. Uber Fx adds indirection (reflection-based injection, lifecycle hooks, error messages referencing Go types) without proportional benefit for this codebase size. Explicit wiring in `cli.Execute()` → `NewApp(registry)` is easier to trace, debug, and refactor. The `internal/version` package retains an Fx Module for reference if Fx is adopted later.

**Alternatives considered**: Uber Fx (original spec) — provides lifecycle management and automatic wiring, but adds cognitive overhead and makes the boot sequence harder to follow in a project where the dependency graph is small and stable.

### D1d. Lightweight custom OTel over official SDK

**Decision**: Use a custom lightweight `otel.Session` model for trace/metric collection instead of the official `go.opentelemetry.io` SDK.

**Rationale**: v1 only exports to local JSON files — no OTLP network endpoints. The official OTel Go SDK pulls in significant dependencies (gRPC, protobuf, OTLP exporters) that increase binary size by ~10-15MB. The custom `Session` model provides the same trace/metric semantics (spans with parent/child, attributes, metrics with units) in ~150 lines with zero external dependencies. If OTLP export is needed in the future, the custom model can be replaced with the official SDK; the `Exporter` interface is designed to accommodate this migration.

**Alternatives considered**: Official OTel Go SDK — provides full OTLP support and ecosystem compatibility, but brings heavy dependencies inappropriate for a CLI tool that only writes local files in v1.

### D2. Module Architecture (Go packages)

**Decision**: Clean architecture with dependency inversion.

```
cmd/hams/              → main.go (calls cli.Execute(), explicit wiring)
internal/
  cli/                 → urfave/cli command definitions, global flag parsing, routing to providers
  config/              → hams.config.yaml loading, merge logic (.local.yaml), profile resolution
  state/               → State file read/write, lock manager (PID+cmd), baseline tracking
  hamsfile/             → Hamsfile read/write with comment preservation, SDK for providers
  provider/
    registry.go        → Provider registration, DAG resolution, lifecycle management
    interface.go       → Provider interface (Probe, Apply, Remove, List, Enrich)
    hook.go            → Hook execution engine (pre/post, defer, nested calls)
    builtin/           → All builtin provider implementations
      bash/
      homebrew/
      apt/
      pnpm/
      npm/
      uv/
      goinstall/
      cargo/
      vscodeext/
      git/
      defaults/
      duti/
      mas/
  tui/                 → BubbleTea alternate screen, progress, collapsible logs, popup
  notify/              → Notification channels (terminal-notifier, Bark)
  otel/                → OTel setup, span helpers, local file exporter
  i18n/                → Locale detection, message catalog, translation loading
  logging/             → Structured logging, log rotation, session log management
  urn/                 → URN parsing and validation (urn:hams:<provider>:<id>)
  sudo/                → Sudo credential caching, elevation helpers
  selfupdate/          → Self-upgrade logic (GitHub Releases / brew detection)
pkg/
  sdk/                 → Public SDK for external provider authors (Go)
docs/                  → Nextra documentation site
```

### D3. Provider Lifecycle

**Decision**: Unified lifecycle with 6 phases.

1. **Register**: Provider declares metadata (name, display name, platform support, depend-on, priority, manifest).
2. **Bootstrap**: If provider's runtime is not installed, execute its `depend-on` chain (recursive, cycle-detected).
3. **Probe** (refresh): Query environment for current state of all resources in this provider's state file.
4. **Plan**: Diff desired (Hamsfile) vs observed (state) → generate action list (install/update/remove/skip).
5. **Apply**: Execute actions in Hamsfile order within the provider, respecting hooks (pre/post/defer).
6. **Enrich**: Async LLM-driven tag/intro generation for newly installed resources.

### D4. Write-Serial Execution Model

**Decision**: "Read parallel, write serial" with global lock.

- All providers can run `Probe` in parallel (read-only).
- `Apply` phase: providers execute sequentially in priority order (D5). Within a provider, resources execute sequentially in Hamsfile order.
- All Hamsfile and state file writes go through the `hamsfile` module which holds a global mutex.
- Architecture uses interfaces and channels to preserve future parallel Apply extensibility (open-closed principle).

### D5. Provider Priority Resolution

**Decision**: Two-level ordering.

1. **DAG level**: Provider self-install dependencies form a DAG. Execute in topological order.
2. **Priority level**: Within the same DAG level, execute in configured priority order:
   - Default: `Homebrew, apt, pnpm, npm, uv, go, cargo, vscode-ext, mas, git, defaults, duti, bash` (bash last because its scripts typically depend on packages being installed first)
   - Overridable in `hams.config.yaml` field `provider-priority: [...]`.
   - Unlisted providers: appended alphabetically after the priority list.

### D6. Hamsfile SDK as Single Gateway

**Decision**: All Hamsfile read/write MUST go through the `hamsfile` package.

No provider directly reads or writes YAML files. The `hamsfile` package:
- Parses YAML with comment preservation (using `go-yaml` v3 with `yaml.Node`).
- Provides typed accessors for provider-specific sections.
- Handles `.local.yaml` merge (per-provider merge strategy registered by provider).
- Handles atomic writes (write to temp, rename).
- Holds the write mutex.

### D7. State as Database

**Decision**: State files are treated as an application database, not user-editable config.

- Schema-versioned (`schema_version: 1`).
- Machine-generated, never hand-edited.
- Written atomically (temp + rename).
- Lock file at `.state/<machine-id>/.lock` contains PID + command + timestamp.

## Risks / Trade-offs

- **[Risk] go-yaml v3 comment preservation is imperfect** → Mitigation: Extensive property-based tests for round-trip fidelity. Fall back to `yaml.Node` tree manipulation rather than marshal/unmarshal cycles.
- **[Risk] hashicorp/go-plugin adds binary size and complexity** → Mitigation: External plugins are v1-deferred in practice; the interface is designed but only builtin Go providers ship initially. go-plugin is a well-maintained library with minimal overhead.
- **[Risk] Sequential provider execution is slow for large configs** → Mitigation: Design for future parallelism (interfaces, channels). Probe phase is already parallel. Most time is spent in network I/O (package downloads) which the wrapped CLI handles.
- **[Risk] LLM subprocess dependency (claude/codex) may not be installed** → Mitigation: LLM enrichment is always optional. Graceful degradation: tags default to `uncategorized`, intro stays empty. Clear error message with install instructions.
- **[Risk] Sudo credential timeout during long apply** → Mitigation: Periodic `sudo -v` heartbeat in background goroutine to extend the sudo ticket.
- **[Risk] BubbleTea TUI and interactive provider stdin conflict** → Mitigation: Interactive popup suspends TUI rendering in that region, passes raw stdin to subprocess. Notification fires to alert user.
- **[Trade-off] Single binary bundles go-git (adds ~5-10MB)** → Acceptable: bootstrap UX on fresh machines outweighs binary size. Users installing via Homebrew can use system git; go-git is the fallback.
- **[Trade-off] No parallel apply in v1** → Acceptable: simplifies state management, logging, TUI. Architecture preserves extensibility.
