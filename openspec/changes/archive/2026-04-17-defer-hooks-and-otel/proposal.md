# 2026-04-16-defer-hooks-and-otel

## Why

A cycle-2 architectural audit (see `openspec/changes/2026-04-16-verification-findings/`) flagged **two more scaffolded-but-unwired features** matching the shape of the already-deferred `--hams-lucky` (see `2026-04-16-spec-impl-reconciliation/`):

### 1. Hook execution engine is fully built but never triggered

- `internal/provider/hooks.go` defines `Hook`, `HookSet`, `RunPreInstallHooks`, `RunPostInstallHooks`, `RunPreUpdateHooks`, `RunPostUpdateHooks`, `CollectDeferredHooks`, `RunDeferredHooks`. ✓
- `internal/provider/executor.go:100-150` calls `runPhasePreHooks`/`runPhasePostHooks` around every action. ✓
- `Action.Hooks *HookSet` field exists on the Action struct (`internal/provider/provider.go:104`). ✓
- **Zero producers assign `action.Hooks`.** A grep for `\.Hooks\s*=` finds only internal references in hooks.go; no provider's `Plan()` method populates the field.
- **The `hamsfile` package has no hook-parsing logic.** No `hooks:`, `pre_install`, `post_install`, `pre_update`, or `post_update` key handling anywhere in `internal/hamsfile/`.
- **User impact**: a user who copies the spec's example `hooks:` block into their hamsfile gets a silent no-op. Worse than lucky-enrichment because hooks are a documented user-facing feature (see `schema-design/spec.md` around line 340).

### 2. OTel tracing + metrics exist but CLI never invokes them

- `internal/otel/otel.go` defines `Session`, `Span`, `LocalFileExporter`, `StartSpan`, `EndSpan`, `RecordMetric`. ✓
- `LocalFileExporter` writes JSON to `${HAMS_DATA_HOME}/otel/traces/` and `${HAMS_DATA_HOME}/otel/metrics/`. ✓
- `provider.Execute(ctx, p, actions, sf, otelSession ...*otel.Session)` accepts an optional session. ✓
- **Every caller of `provider.Execute` passes no session** — `internal/cli/apply.go:444` is the sole caller; zero OTel integration in any `internal/cli/*.go` file.
- **No root `hams.apply` / `hams.refresh` spans are ever created.** Spec promised observability for long-running operations (sampled at >200 resources); sampling logic isn't implemented either.
- **User impact**: lower than hooks — users don't directly consume OTel output. But `CLAUDE.md` advertises "OTel: trace + metrics, local file exporter at `${HAMS_DATA_HOME}/otel/`" and no user would find traces at that path on disk.

## What Changes

Both features are **deferred to v1.1**, mirroring the architect call for `--hams-lucky`. No code changes in this change (scaffolding preserved for v1.1 to plug into). Spec deltas only.

### Hooks deferral (schema-design spec)

- Mark the `hooks:` schema section as "specified but not yet parsed by the hamsfile loader; v1 will silently ignore `hooks:` blocks. The execution engine at `internal/provider/hooks.go` is ready for v1.1 to wire up."
- Add a new requirement: "In v1, the loader SHOULD emit a `slog.Warn` when it encounters a hamsfile `hooks:` block so users are not surprised by silent behavior." *(deferred to a follow-up — the warning implementation is ~20 lines + tests; spec captures it but doesn't block archival.)*

### OTel deferral (cli-architecture spec)

- Update any CLI-architecture mentions of "OTel tracing of apply/refresh" to mark the CLI-integration as v1.1-deferred.
- `CLAUDE.md` claim remains accurate *at the package level* (the OTel package can export traces; just nothing in hams calls it).

### Code changes

**None in this change.** Like lucky, both scaffoldings stay in place for v1.1 to plug into — removing them would force re-implementation of ~600 lines.

## Impact

- Affected specs: `schema-design` (hooks deferral), `cli-architecture` (OTel integration deferral).
- Affected code: none.
- Affected tests: the existing `internal/provider/hooks_test.go` stays passing — it tests the hooks engine in isolation, which is correctly scoped.
- User-visible: no behavior change in v1 (features weren't working before, aren't working after). Spec now honestly documents the gap.

## Rationale

Same as the `--hams-lucky` precedent (commit `f4c0f20`): the plumbing is end-to-end correct; only the "last mile" consumer wiring is missing. Removing the scaffolding would force re-implementation in v1.1 with identical structure. Documenting the gap is the honest architectural move.

A possible follow-up in v1.1 is one of three options for hooks (selected by the v1.1 designer):

1. Wire hamsfile parsing to populate `Action.Hooks` — unblocks the full documented workflow.
2. Remove hooks entirely if a simpler alternative (e.g., explicit `run` steps via the bash provider) covers the use case.
3. Rewrite hooks on top of the bash provider so there's only one "run a command" mechanism.
