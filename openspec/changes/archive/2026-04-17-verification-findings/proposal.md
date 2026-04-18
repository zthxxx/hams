# 2026-04-16-verification-findings

## Why

A post-v1 verification cycle (multi-agent spec-to-implementation cross-check + user-workflow walkthrough + test-design audit) was run on 2026-04-16 against the shipped 11 capability specs. The cycle found:

- All core user workflows function correctly (install+record, bootstrap, apply, refresh, version pin, lock management, sudo heartbeat).
- `golangci-lint v2`'s `goconst` check flagged 10 issues in 7 provider files where provider name/display-name string literals repeated ≥3 times. **Already fixed in this change** (each provider now has `const cliName` / `const displayName` following the pattern already in `apt`/`git`/`defaults`).
- Three markdown files violated `MD032` (lists need blank-line separators). **Already fixed.**
- Several spec-vs-implementation divergences remain that don't block user workflows but should be reconciled (documented below as tasks).
- Provider test coverage is uneven: `apt` has 38 lifecycle tests (U1-U37 pattern), most other providers have 2 (Manifest + parse helper). This is a real gap — package-manager providers without apply/probe unit tests accept silent host mutation on contributor machines.

## What Changes

**Already applied in this change (verified by `task check` passing):**

- `Taskfile.yml` — `check` now calls `test:unit` instead of `test` (no integration/e2e chain)
- `internal/provider/builtin/{duti,git,goinstall,homebrew,npm,pnpm,vscodeext}/*.go` — extracted repeated name/display-name literals into `const cliName` / `const displayName` to satisfy `goconst`
- `CLAUDE.md`, `AGENTS.md`, `docs/notes/gh-cli-engineering-analysis.md` — added blank lines before lists (MD032)

**Documented as tasks (not yet implemented):**

- Remove dead `CLIHandler` interface in `internal/provider/provider.go:169-173` (no provider implements it; `ProviderHandler` in `internal/cli/provider_cmd.go:13-20` is the one actually used).
- Reconcile spec-vs-implementation naming divergences: spec says `go`/`vscode-ext`, impl uses `goinstall`/`code-ext`. Prefer updating the spec because changing the impl would invalidate existing users' hamsfiles and state paths.
- Reconcile `DefaultProviderPriority` ordering: spec lists `bash` first, impl has it 14th. Current behavior is functionally correct (DAG pulls bash first when any provider depends on it), but spec letter differs.
- Extend property-based parser tests across providers (`ParseNpmList`, `ParseCargoList`, `ParsePnpmList`, `ParseExtensionList`, `ParseUvToolList`, `ParseMasList`). Parsers accept arbitrary upstream CLI output; only example-based cases exist today.
- Extend `apt`-style lifecycle test pattern (install → reinstall → upgrade → remove → re-install) to other package-like providers (`cargo`, `npm`, `pnpm`, `uv`, `goinstall`, `vscodeext`, `mas`).
- Add DI-isolated apply/probe tests for KV-config providers (`defaults`, `duti`, `git-config`). Currently they have no apply tests at all.
- Wire `--hams-lucky` flag through the enrichment flow (flag is extracted by `splitHamsFlags` at `internal/cli/flags.go:5-43` but never consumed downstream).

## Impact

- Affected specs: **code-standards** (goconst constant requirement), **provider-system** (CLIHandler dead-code removal, naming reconciliation).
- Affected code (already applied): 7 provider files + Taskfile + 3 Markdown files.
- Affected tests: none broken; existing suite passes.
- User-visible: none directly. `task check` now actually runs the local-only check path it advertises.
