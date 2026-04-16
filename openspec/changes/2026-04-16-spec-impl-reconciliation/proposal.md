# 2026-04-16-spec-impl-reconciliation

## Why

The 2026-04-16 verification cycle (commit `10de4bd`) surfaced two classes of spec-vs-implementation divergences that the previous change explicitly deferred to follow-up. Both are pure reconciliations — code is unchanged; only specs and docs are corrected to match shipped reality.

### Divergence 1: Provider naming

- **Spec says** `go` — **impl ships** `goinstall` (file prefix, manifest name, CLI verb, state path).
- **Spec says** `vscode-ext` — **impl ships** `code-ext` (same).

Renaming the impl would invalidate every existing user's hamsfile/state/file paths and require a backward-compatible read+forward-write migration in `internal/state`. Renaming the spec is one find/replace.

50+ stale references across `openspec/specs/` (3 files), `docs/content/` (en + zh-CN), `README.md`, and `AGENTS.md`. See `tasks/naming.task.md`.

### Divergence 2: `--hams-lucky` LLM enrichment is scaffolded but unwired

- **Spec promises** `hams brew install foo --hams-lucky` skips TUI prompts and auto-accepts LLM-recommended tags + intro.
- **Reality**:
  - `splitHamsFlags()` extracts the flag ✓
  - `RunTagPicker(lucky bool)` short-circuits when lucky=true ✓
  - `Enricher` interface defined with `Enrich(ctx, resourceID) error` ✓
  - `runEnrichPhase` in apply.go iterates registered providers and type-asserts to `Enricher` ✓
  - **`llm.Recommend()` has zero production callers** ✗
  - **Zero providers implement `Enrich(ctx, resourceID)`** ✗ — the type assertion always fails; loop body never runs.
  - **Zero providers read `hamsFlags["lucky"]`** ✗ — flag is silently dropped at the provider boundary.
  - **`RunTagPicker` is only called by tests**, never from a production code path ✗.

Verdict: the entire `--hams-lucky` user-visible behavior **does not exist today**. The flag parses without error, then is dropped on the floor. The user gets no TUI prompt either way (because no provider ever shows one), and no LLM tags are written (because no provider calls the LLM).

Architect call: defer the entire feature to v1.1 in spec. **Don't remove the scaffolding** — `EnrichAsync`, `EnrichCollector`, `Recommend`, `RunTagPicker(lucky)`, `Enricher` interface stay in place because v1.1 will use them. Only the spec needs to acknowledge that providers have not yet been wired.

## What Changes

### Spec deltas (this change)

- `openspec/specs/builtin-providers/spec.md` — replace `vscode-ext`→`code-ext` and `go`→`goinstall` throughout.
- `openspec/specs/provider-system/spec.md` — same name fixes; update `DefaultProviderPriority` example to match shipped order.
- `openspec/specs/schema-design/spec.md` — same name fixes.
- `openspec/specs/cli-architecture/spec.md` — modify the `--hams-lucky` section to mark the user-facing scenarios as **deferred to v1.1**, and document the current state ("flag parses but providers do not yet consume it; LLM enrichment is scaffolded but no provider implements `Enricher`").

### Docs updates (this change)

- `docs/content/en/docs/providers/{go.mdx,vscode-ext.mdx,index.mdx}` and `_meta.ts` — rename to `goinstall.mdx` and `code-ext.mdx`; update internal references.
- `docs/content/zh-CN/docs/providers/{go.mdx,vscode-ext.mdx,index.mdx}` — same.
- `docs/content/en/docs/cli/apply.mdx` and `docs/content/zh-CN/docs/cli/apply.mdx` — fix provider name in the "skip a provider" example.
- `README.md` and `AGENTS.md` — update the "15 builtin providers" line.

### Code changes (this change)

**None for naming reconciliation.** The impl already uses `goinstall`/`code-ext`.

**For lucky enrichment**: leave all scaffolding in place. The spec change merely documents the v1.1 deferral.

## Impact

- Affected specs: `builtin-providers`, `provider-system`, `cli-architecture`, `schema-design`.
- Affected docs: `docs/content/en/`, `docs/content/zh-CN/`, `README.md`, `AGENTS.md`.
- Affected tests: none. The `dag_test.go` `TestResolveDAG_ZeroIndegreePriority` from the previous cycle still passes.
- User-visible: docs now match what `hams --help` and `hams <provider> --help` print at runtime. The `--hams-lucky` section now honestly says "deferred until provider Enricher implementations land in v1.1."
