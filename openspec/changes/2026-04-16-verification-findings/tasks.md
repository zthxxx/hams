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
- [x] [provider-test-coverage] — extend apt-style lifecycle tests and property-based parser tests to remaining providers: `tasks/provider-test-coverage.task.md`. **Tier 1 complete; Tier 2/3 partial:**
  - [x] Property-based parser tests for cargo, npm, pnpm, uv, mas, vscodeext (commit `3467967`). Caught + fixed real bug in `parseExtensionList` (silent corruption on `@version` and tab-containing inputs).
  - [x] Tempdir-isolated apply/probe/remove tests for git-config (commit `703f66c`). Coverage 1.8% → 14.0%.
  - [x] Probe + property + safety tests for git-clone (commit `588b86e`). Coverage 14.0% → 23.0%.
  - [x] **Tier 1 closure** — DI refactor + apt-style lifecycle tests for all 5 package-like providers:
    - cargo (commit `f3dde9a`): 28.8% → 68.8%
    - npm (commit `a972bd4`): 23.4% → 67.7%
    - pnpm (commit `5bad9cd`): 29.8% → 71.4%
    - uv (commit `682f22b`): 31.5% → 70.0%
    - goinstall (commit `682f22b`): 13.7% → 62.0%
  - [ ] **Still deferred**: apply/probe DI tests for defaults, duti, mas (Tier 2 — same DI-refactor scope as Tier 1; follow-up cycle).

## Cycles 101-109 — follow-up autonomous verification (overnight loop)

The user-workflow "CLI-first, auto-record" contract was broken on 3 KV-config providers (CP-2 class: git-config, defaults, duti) — same shape as the package-class gap addressed in commits `39f8f4c` / `4c89814` / etc., but a class the earlier audit missed because it was scoped to package-class providers only. Cycles 101-109 close the gap, add spec-required verbs, fix a real URL-encoding bug, tighten store push UX, and fill coverage holes.

- [x] **Cycle 101** — git-config CLI `hams git-config <key> <value>` now auto-records to hamsfile + state via a new `CmdRunner` DI seam. Apply/Probe/Remove also routed through the seam so unit tests never exec real git. 9 U-series tests. (commit `2f1bc6f`)
- [x] **Cycle 102** — defaults CLI `write`/`delete` auto-record (was: exec'd `defaults` directly, only updated `preview-cmd` on already-existing entries). Write records `{app: "domain.key=type:value"}` + state; delete removes entry + marks `StateRemoved`. Raw `defaults read/domains/...` passes through. 10 U-series tests. (commit `ae9fa4e`)
- [x] **Cycle 103** — duti CLI `hams duti <ext>=<bundle-id>` auto-record. Raw duti flags (`-s`, `-x`, …) still pass through for power-user escape-hatch. 9 U-series tests. (commit `9885718`)
- [x] **Cycle 104** — git-config verb routing per spec: `set <key> <value>` / `remove <key>` / `list` / bare (backward compat). 8 additional U-series tests (U10-U17). Spec said these verbs existed; impl only accepted bare form. (commit `5ba5171`)
- [x] **Cycle 105** — Dead code: `Registry.All()` had zero callers anywhere; deleted. Added direct tests for `ResourceClass.String`, `ActionType.String`, `BootstrapRequiredError.Error/Unwrap` (all previously 0% coverage). (commit `95565e0`)
- [x] **Cycle 106** — Property-based parser tests for git-config, defaults, duti, homebrew, goinstall (previously lacking, unlike other providers). No-panic, idempotency, prefix/round-trip invariants, fail-closed on malformed input. Found one bug in my own test design (shrinker caught join/split mismatch). Plus `.gitignore` fix: `testdata/rapid/` wasn't matching subdirectories. (commits `77c3c04`, `ac25b61`)
- [x] **Cycle 107** — **Real bug.** `barkChannel.Send` used `strings.ReplaceAll(..., " ", "%20")` only, leaving `#`/`?`/`/` unescaped. A notification message with `#` would truncate at the fragment separator silently. Fix: `url.PathEscape` per segment + overridable `barkBaseURL` for DI. notify coverage: 60% → 96%. (commit `561a8be`)
- [x] **Cycle 108** — **Real UX bug.** `hams store push` after `hams refresh` (clean tree) exited non-zero with git's "nothing to commit" bubbling through. Added empty-commit detection via `git status --porcelain` short-circuit, plus `-m/--message` flag so commits aren't all "hams: update store". Introduced `storePushRunner` DI seam for unit testability. 6 unit tests. (commit `6841eb7`)
- [x] **Cycle 109** — Direct tests for `IsPlatformsMatch` and `HookSet.HasAny` (both 66.7% via indirect exercise, now 100%). Locks in the "empty platform = wildcard" contract so a future default-change can't silently hide every unfiltered provider. provider coverage: 73.6% → 74.2%. (commit `f217b18`)
