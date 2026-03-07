## Why

Developers managing macOS/Linux workstations lack a tool that bridges "install a package via CLI" and "record it declaratively for future machine restoration." Existing solutions each cover a partial slice: Homebrew's `Brewfile` only covers brew; NixOS is too strict and isolated; Ansible requires writing playbooks before installing; chezmoi handles dotfiles but not packages; Terraform/Pulumi target cloud infrastructure, not local hosts. The gap is a tool that wraps _any_ package manager, auto-records installations into comment-preserving YAML, and replays them on a new machine with `hams apply`. Now is the time because the author's own `init-macOS-dev` shell scripts have grown unmaintainable: over 200 packages across brew/cask/mas/npm, plus `defaults write`, `git config`, `duti`, `chsh`, and arbitrary shell scripts, all in procedural bash with no state tracking, no idempotency probes, and no TUI feedback.

## What Changes

This is a greenfield v1 — the entire `hams` CLI and its ecosystem are being built from scratch.

- **New CLI binary** (`hams`): Go static binary with Uber Fx DI, Cobra command routing, multi-level subcommands (`hams brew install`, `hams pnpm add`, `hams apply`, etc.).
- **Provider architecture**: Plugin system (builtins compiled in Go, externals via `hashicorp/go-plugin`) where each provider wraps an existing CLI tool (brew, pnpm, npm, apt, uv, go, cargo, mas, code, git, defaults, duti) with auto-record and probe capabilities.
- **Hamsfile format** (`<Provider>.hams.yaml`): Comment-preserving YAML store files per provider, organized by profile directory, with `.local.yaml` merge support for machine-specific entries.
- **Terraform-style state** (`.state/<machine-id>/<Provider>.state.yaml`): Local state tracking per-resource status (`ok`/`failed`/`pending`/`removed`), drift detection via refresh, single-writer lock, and failure recovery.
- **TUI**: Alternate-screen terminal UI with collapsible logs, sticky log path, interactive popup for signin/OAuth, and notification system (terminal-notifier + Bark).
- **Observability**: OpenTelemetry trace + metrics with local file exporter for debugging.
- **Documentation site**: Nextra-based docs at `hams.zthxxx.me` on GitHub Pages.
- **Bootstrap**: One-line `curl | bash` installer, `hams apply --from-repo=<github-user/repo>` for first-time setup on a fresh machine (bundled go-git, no git dependency).
- **LLM integration**: Async tag/intro generation via `claude`/`codex` CLI subprocess for auto-categorizing installed packages.

## Capabilities

### New Capabilities

- `project-structure`: Go module layout, directory conventions (`cmd/`, `internal/`, `pkg/`), builtin vs plugin separation, build targets (OS/arch matrix), Docker e2e test infrastructure, GitHub Actions CI.
- `cli-architecture`: Cobra/Fx bootstrap, command routing (`hams <global-flags> <provider> <verb> <args> --hams:flags -- <passthrough>`), lock file (PID+cmd), sudo management (once-at-startup), i18n (`LC_ALL`/`LANG` parsing), exit code semantics, error format (AI-agent friendly), `self-upgrade` command.
- `schema-design`: `hams.config.yaml` (global + project-level + `.local.yaml` merge), `<Provider>.hams.yaml` Hamsfile format, `<Provider>.state.yaml` state format, `urn:hams:<provider>:<id>` structure, YAML comment-preserving read/write SDK (Go + future Bun).
- `provider-system`: Provider interface/lifecycle (register, probe, apply, remove, list, enrich), resource identity (natural name vs URN), depend-on DAG with platform-conditional declarations, cycle detection, hook model (pre/post-install, `defer`, nested provider calls), manifest format, `hashicorp/go-plugin` extension contract, builtin/external classification table.
- `tui-logging`: Alternate screen layout (progress bar, current operation, collapsible logs), sticky top log file path (`~/` prefix), interactive popup (tmux-popup style for signin/OAuth), notification system (terminal-notifier mandatory + Bark optional), log rotation (`${HAMS_DATA_HOME}/<YYYY-MM>/`), session log linking, `--debug` log level design.
- `observability`: OTel trace spans (apply → provider → resource), metrics (duration, failure rate, resource count), local file exporter at `${HAMS_DATA_HOME}/otel/`, future OTLP extensibility.
- `docs-site`: Nextra framework, chapter structure (Why/Quickstart/CLI Reference/Builtin Provider Catalog/Schema Reference/Provider API), GitHub Pages deploy at `hams.zthxxx.me`, English primary with Chinese i18n extensibility, hand-crafted code examples.
- `code-standards`: Go conventions (dependency inversion, IoC, interface-driven design), strict golangci-lint v2 config, error handling patterns, package naming, testing standards (property-based), PR/commit conventions for world-class open-source quality.
- `builtin-providers`: Individual design for each builtin: Bash, Homebrew, apt, pnpm, npm, uv, go, cargo, VSCode Extension, git (config + clone), defaults, duti, mas. Each covers: manifest, store schema, probe implementation, apply/remove flow, auto-inject flags, LLM enrichment command.

### Modified Capabilities

_(None — this is a greenfield project.)_

## Impact

- **New Go packages**: `cmd/hams`, `internal/cli`, `internal/provider`, `internal/state`, `internal/hamsfile`, `internal/tui`, `internal/otel`, `internal/notify`, `internal/i18n`, `pkg/sdk`.
- **New JS/TS packages**: Nextra docs site under `docs/`, future Bun SDK under `sdk/bun/`.
- **External dependencies**: Uber Fx, Cobra, `hashicorp/go-plugin`, go-git, BubbleTea (TUI), OTel Go SDK, `go-yaml` (comment-preserving YAML).
- **CI/CD**: GitHub Actions for lint/test/build matrix (darwin-arm64, linux-amd64, linux-arm64), Docker e2e tests (Debian, Alpine, OpenWrt), GitHub Pages deploy for docs.
- **Distribution**: GitHub Releases (static binaries), Homebrew tap (`zthxxx/tap/hams`), `curl | bash` installer script.
