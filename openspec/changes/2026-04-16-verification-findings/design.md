# Design — 2026-04-16-verification-findings

## Context

This change arose from an explicit verification cycle request: "verify all implemented specs from the user-workflow perspective, check test-design reasonableness, check architecture extensibility; record findings as tasks for what needs fixing."

Four parallel Explore agents cross-referenced specs against code:

1. `openspec/specs/provider-system/spec.md` ↔ `internal/provider/`
2. `openspec/specs/cli-architecture/spec.md` ↔ `cmd/hams/`, `internal/cli/`, `internal/apply/`, `internal/config/`
3. `openspec/specs/schema-design/spec.md` + `builtin-providers/spec.md` ↔ `internal/hamsfile/`, `internal/state/`, `internal/config/`, all 15 provider implementations
4. All `*_test.go` files, integration test directories, DI boundaries

## Design Decisions

### Decision 1: Two classes of finding require two different remedies

**Real bugs:** `task check` was broken (expanded beyond its described scope); 10 goconst lint failures; 3 markdown lint failures. These have fixes with no design trade-off — applied in this change, verified by `task check` passing.

**Spec-vs-impl divergences with architectural implications:** provider naming (`go`/`goinstall`, `vscode-ext`/`code-ext`), priority ordering, dead `CLIHandler` interface, unwired `--hams-lucky`. These require decisions about "which side is correct" and those decisions have user impact. They go into `tasks/*.task.md` as follow-up work with the reasoning captured.

### Decision 2: When spec and code diverge, default to "code is right, spec is wrong"

For the two naming divergences (`go` vs `goinstall`, `vscode-ext` vs `code-ext`), the code has users: existing hamsfile + state paths + file prefixes all use the impl names. Renaming in code would invalidate those files. Renaming in spec costs one find/replace.

This is OpenSpec's core principle inverted for a reason: **the spec documents shipped behavior**. When shipped behavior works correctly but the spec drafted it with a different name, the spec — not the shipped behavior — is the artifact with the mismatch.

### Decision 3: Defer provider-test-coverage extension; don't rush it

Tempting to copy apt's 38-test pattern across all 11 undercovered providers in one sweep. Resisted because:

- Each provider needs a DI boundary (`FakeCmdRunner`-style) designed for its specific external tool. A template solution would over-abstract.
- Adding 11 × 38 = 418 tests in one PR risks introducing fakes that drift from real behavior. One provider at a time, with each one validated against its existing integration test, is safer.
- The current test suite passes. Undercovered providers aren't broken; they're just under-tested. Taking one iteration per provider in follow-up changes is better hygiene.

### Decision 4: Leave the coarse ActionUpdate-on-config-hash-change mechanism alone

An Explore agent flagged that `plan.go:ComputePlan` never emits `ActionUpdate`, claiming this breaks update hooks. Inspection of `apply.go:428-434` shows the real mechanism: when the hamsfile content hash differs from the previously-applied hash, all `ActionSkip` entries get promoted to `ActionUpdate`. This is a coarse but functional mechanism — it updates all resources when any resource changes.

Per-resource change detection would be better (update only the specific resource whose declaration changed), but it's an optimization, not a correctness issue. Adding it is a feature, not a bug fix. Not in scope for this verification change.

### Decision 5: Use context propagation for `--hams-lucky`, not parameter explosion

See `tasks/lucky-enrichment.task.md` — matches the existing `WithBootstrapAllowed` pattern in `bootstrap.go`, avoids changing `Enricher.Enrich` signature, works both for `hams <provider>` and `hams apply` invocations.

## Risks & Non-Goals

**Not addressed by this change:**

- The `ResolveDAG` alphabetical-vs-priority behavior needs an investigation before it can be fixed (see `tasks/spec-reconciliation.task.md`). Changing the DAG's tie-breaking rule could silently reorder other providers in user environments.
- `CheckedAt` timestamp not always set by `ProbeAll` — minor, low user impact.
- `OpenWrt` platform support — explicitly deferred from v1.
- `go-plugin` external-provider infrastructure — explicitly deferred from v1 per provider-system spec.
- Bun/TypeScript SDK — explicitly deferred from v1.

**Explicitly verified as CORRECTLY implemented (agent reports were wrong):**

- `internal/urn/` module exists with 96.3% test coverage. Provides `urn.Parse`, `urn.New`, `urn.String`, `urn.IsValid`. Agent-1 incorrectly claimed it was missing.
- `ComputePlan` → `ActionUpdate` path exists via config-hash comparison in `apply.go:428-434`. Agent-1 incorrectly claimed updates never fire.

## Dependencies on Other Work

None. This change captures findings about already-shipped work.
