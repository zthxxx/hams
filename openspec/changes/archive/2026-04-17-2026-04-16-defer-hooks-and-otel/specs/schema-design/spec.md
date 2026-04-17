# schema-design — Spec Delta (hooks defer)

## MODIFIED Requirements

### Requirement: Hooks Schema — Deferred to v1.1

The `hooks:` YAML block on a hamsfile item is **specified but not yet consumed by the v1 loader**. The execution engine at `internal/provider/hooks.go` is fully built (pre/post-install, pre/post-update, `defer: true` collection, `RunDeferredHooks` late-execution) and tested in isolation at `internal/provider/hooks_test.go`. But:

- `internal/hamsfile/` has zero YAML parsing of `hooks:`, `pre_install`, `post_install`, `pre_update`, or `post_update` keys.
- No provider's `Plan()` method assigns `Action.Hooks`. A grep for `\.Hooks\s*=` across `internal/provider/builtin/` finds zero producer assignments.
- Therefore: in v1, a hamsfile containing a `hooks:` block has the block **silently ignored**. Install/remove proceeds without running any hook.

This SHALL be acknowledged honestly in the spec until v1.1 wires the hamsfile parser through to `Action.Hooks`.

#### Scenario: hooks: block is silently ignored in v1

- **WHEN** a user adds `hooks: { pre_install: [{run: "echo hello"}] }` to an item in their hamsfile
- **THEN** the v1 hams loader SHALL load the hamsfile without error
- **AND** `hams apply` SHALL NOT execute the hook
- **AND** no slog warning is emitted in v1 (a follow-up change MAY add one; it is NOT blocking).

#### Scenario: Hook execution engine is ready for v1.1 to wire up

- **WHEN** a v1.1 release adds `hooks:` parsing to `internal/hamsfile/` and populates `Action.Hooks` in provider `Plan()` methods
- **THEN** the existing `internal/provider/hooks.go` engine SHALL run the declared hooks around install/update actions
- **AND** the existing `internal/provider/executor.go:100-150` dispatch logic SHALL execute them in the documented order (pre → action → post → deferred).

#### Scenario: Deferred hooks remain as v1.1 scope

- **WHEN** v1.1 ships hook parsing
- **THEN** the `defer: true` semantics described in the schema-design spec SHALL re-activate as a user-facing requirement (currently enforceable only by unit tests via `CollectDeferredHooks`).

## Why deferred (not removed)

- The hook execution engine + tests (~200 lines) would need to be re-written in v1.1 if removed.
- The Action.Hooks field + executor dispatch glue are correctly structured; v1.1 only needs to add parser-to-Action plumbing (~50 lines).
- Documenting the gap is less disruptive than editing the spec to remove the feature then re-adding it.
