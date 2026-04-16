# Tasks — 2026-04-16-verification-findings

## Completed in this change (verified by `task check` passing)

- [x] Fix 10 goconst lint errors across 7 provider files by extracting name/display-name literals into `const cliName` / `const displayName` (matching the `apt`/`git`/`defaults` precedent).
  - [x] `internal/provider/builtin/duti/duti.go` — add `const cliName = "duti"`
  - [x] `internal/provider/builtin/git/git.go` — add `const configDisplayName = "git config"`
  - [x] `internal/provider/builtin/goinstall/goinstall.go` — add `const cliName = "goinstall"` + `const displayName = "go install"`
  - [x] `internal/provider/builtin/homebrew/homebrew.go` — add `const cliName = "brew"` + `const brewDisplayName = "Homebrew"`
  - [x] `internal/provider/builtin/npm/npm.go` — add `const cliName = "npm"`
  - [x] `internal/provider/builtin/pnpm/pnpm.go` — add `const cliName = "pnpm"`
  - [x] `internal/provider/builtin/vscodeext/vscodeext.go` — add `const cliName = "code-ext"` + `const displayName = "VS Code Extensions"`
- [x] Fix `Taskfile.yml` `check` target: was transitively calling `task test` → `test:integration` → `act` (not installed locally, violates the task's own "no Docker required" description). Changed to call `task test:unit` directly.
- [x] Fix 3 `MD032` markdown lint violations (blank line required before lists) in `CLAUDE.md`, `AGENTS.md`, `docs/notes/gh-cli-engineering-analysis.md`.
- [x] Verify shipped specs against implementation via 4 parallel Explore agents (provider-system, CLI architecture, schema + builtin providers, test design). Findings consolidated in `tasks/*.task.md`.
- [x] [code-cleanup] Remove dead `CLIHandler` interface from `internal/provider/provider.go` — zero Go references remain (see `tasks/code-cleanup.task.md`).
- [x] [spec-reconciliation investigation] Add `TestResolveDAG_ZeroIndegreePriority` at `internal/provider/dag_test.go` — confirms priority-list is INERT for root-level providers (zero-indegree queue is sorted alphabetically in `dag.go:53`). Architect recommendation added to `tasks/spec-reconciliation.task.md`: document alphabetical behavior in spec rather than change DAG tie-breaking.
- [x] [spec-reconciliation: DAG-ordering delta] Updated `specs/provider-system/spec.md` delta with two new scenarios documenting the shipped behavior (alphabetical tie-breaking + dependency precedence). The investigation test in `dag_test.go` now enforces the spec.

## Deferred to follow-up changes (see `tasks/` for design rationale)

- [x] [spec-reconciliation: naming] — align `openspec/specs/builtin-providers/spec.md` to use `goinstall` (not `go`) and `code-ext` (not `vscode-ext`); grep specs + docs for stale references. **Done in commit `6f9e533`** + change `2026-04-16-spec-impl-reconciliation`.
- [x] [lucky-enrichment] — wire `--hams-lucky` through enrichment flow: **architect call in commit `f4c0f20` was to defer to v1.1.** Spec updated to mark scenarios as deferred; scaffolding kept (Enricher interface, RunTagPicker, EnrichAsync, Recommend, llm.Config field) so v1.1 has the foundation.
- [~] [provider-test-coverage] — extend apt-style lifecycle tests and property-based parser tests to remaining providers: `tasks/provider-test-coverage.task.md`. **Partially done:**
  - [x] Property-based parser tests for cargo, npm, pnpm, uv, mas, vscodeext (commit `3467967`). Caught + fixed real bug in `parseExtensionList` (silent corruption on `@version` and tab-containing inputs).
  - [x] Tempdir-isolated apply/probe/remove tests for git-config (commit `703f66c`). Lifts coverage from 1.8% → 14.0%.
  - [x] Probe + property + safety tests for git-clone (commit `588b86e`). Lifts coverage from 14.0% → 23.0%.
  - [ ] **Still deferred to a future change**: apt-style lifecycle tests (FakeCmdRunner-based) for cargo, npm, pnpm, uv, goinstall (Tier 1). Each requires adding a `command.go` interface + `command_fake.go` stub + refactoring `Provider.New(...)` to inject the runner — significant per-provider refactoring.
  - [ ] **Still deferred**: apply/probe DI tests for defaults, duti, mas (Tier 2 — same DI-refactor scope).
