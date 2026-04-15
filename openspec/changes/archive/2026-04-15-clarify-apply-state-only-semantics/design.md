# Design: clarify-apply-state-only-semantics

## Context

The just-archived change `fix-apt-cli-state-write-and-htop-rename` shipped a
two-stage filter on `hams apply` and `hams refresh`:

- **Stage 1 (artifact-presence)**: include providers that have either a
  hamsfile or a state file for the active profile/machine.
- **Stage 2 (`--only` / `--except`)**: narrow within stage-1.

The intent was to skip providers whose upstream tool may not even be
installed on this machine. But the runApply implementation
(`internal/cli/apply.go:235-238`) still has a separate guard that
SKIPS any provider with no hamsfile, regardless of state-file
presence:

```go
if !mainExists && !localExists {
    slog.Debug("no hamsfile for provider, skipping", "provider", name)
    continue
}
```

So a provider in the "state-only" position — user installed
resources via `hams apt install htop`, then deleted `apt.hams.yaml`
— is selected by stage-1 but skipped before any reconciliation runs.
The state file says `htop=ok`; the user expects `apply` to do
something; nothing happens.

A codex-review pass after archive flagged this. An autonomous
architect+user agent debate (recorded in this session's transcript)
concluded:

- "skip" (current) is defensible — Terraform-style: missing file =
  missing intent, not "intent: zero". Implicit prune is high blast
  radius (partial git checkout / accidental delete = mass uninstall).
- BUT the current behavior is undocumented and surprising. Users who
  reason about state declaratively expect deletion of the hamsfile
  to mean "remove these resources next apply".
- Right path: keep "skip" as the explicit, documented default. Add an
  opt-in `--prune-orphans` flag for users who want destructive
  reconciliation. Do not flip the default.

This change formalizes that decision.

## Goals / Non-Goals

**Goals:**

- Preserve the current "skip state-only providers" behavior as the
  explicit, spec-documented default — no surprise behavior change for
  any existing user.
- Add `hams apply --prune-orphans` (default off) so users who want
  destructive reconciliation can opt in.
- Document both modes in cli-architecture spec with WHEN/THEN
  scenarios for state-only-skip + state-only-with-prune.
- Pass through `--prune-orphans` from CLI to the apply path with no
  global-flag pollution (it is apply-specific).
- Lock in the regression with both unit and integration tests so a
  future refactor cannot silently re-introduce mass-uninstall behavior
  by default.

**Non-Goals:**

- Do NOT change the default. Implicit prune is rejected for blast
  radius reasons.
- Do NOT add `--prune-orphans` to `refresh`. Refresh is a read-only
  state-update operation; it has nothing to remove. Keeping the flag
  apply-only avoids confusion about what "prune during refresh" would
  even mean.
- Do NOT extend the flag semantics to per-resource pruning. The
  granularity is "the entire provider goes from declared to
  undeclared" (i.e., the hamsfile vanishes). Per-resource declarative
  removal is already handled by `hams apply` against a hamsfile that
  drops the resource from its list (executor compares declared vs
  observed and produces Remove actions).
- Do NOT introduce a config file knob for "always prune" — opt-in
  must be per-invocation to keep the destructive action explicit at
  the call site.

## Decisions

### Decision 1: Flag name `--prune-orphans` (vs `--prune`, `--remove-orphaned`, `--reconcile-empty`)

**Picked**: `--prune-orphans`.

- `--prune` alone is too generic; users may wonder "prune what?"
- `--remove-orphaned` is verbose and weakly typed (orphaned what?).
- `--reconcile-empty` describes the mechanism, not the user intent.
- "orphans" is the common term for state entries with no declared
  parent (mirrors `kubectl delete --cascade=orphan`'s vocabulary
  inversion: kubectl uses orphan to MEAN "leave behind", we use it
  to mean "the orphans we want to clean up"). The collision is
  unfortunate but the gerund ("prune") disambiguates intent.

### Decision 2: Scope = whole provider, not per-resource

**Picked**: `--prune-orphans` only kicks in when the provider is
state-only (no hamsfile + no `.local.yaml`). It does NOT
retroactively remove resources from a provider whose hamsfile still
exists but no longer lists them — that case is already handled by
the normal Plan/Execute flow against the hamsfile-as-declared.

Alternative: extend `--prune-orphans` to mean "across all providers,
remove anything in state that has no declared counterpart". Rejected:
broader blast radius, blurs the "hamsfile is source of truth" model.
Per-provider scope keeps the user's mental model crisp.

### Decision 3: Implementation point — replace skip with empty-desired in runApply

**Picked**: in `internal/cli/apply.go::runApply`, change the
`if !mainExists && !localExists { continue }` block. When
`flags.PruneOrphans` is true AND the state file exists for this
provider, construct an empty `hamsfile.File` (root MappingNode, no
entries) in-memory and continue into the existing Plan/Execute path
— the executor will compute remove-actions against state and run
them. When `flags.PruneOrphans` is false, the existing skip semantics
are preserved verbatim.

Alternative: handle pruning in a brand-new top-level branch (separate
loop over state-only providers). Rejected: doubles the bootstrap /
sudo / refresh code paths and introduces a second place where
provider invocation lives. The empty-desired-hamsfile approach
re-uses the existing executor, which already knows how to compute
remove actions when desired ⊂ observed.

### Decision 4: Flag wiring — local to apply command, not a global flag

**Picked**: register `--prune-orphans` on the `apply` subcommand only
in `internal/cli/commands.go`. Plumb it through `runApply`'s
signature (or via a dedicated `applyFlags` struct).

Alternative: add a global `--prune` flag. Rejected: pollutes
non-apply commands (refresh, list, config, store) with an irrelevant
knob.

### Decision 5: Test surface

- **Unit**: `internal/cli/apply_test.go` — two new tests:
  1. `TestApply_PruneOrphans_RemovesOrphanedStateResources` — apt
     provider, state has htop=ok, no hamsfile, `--prune-orphans=true`
     → executor removes htop, state transitions to removed.
  2. `TestApply_NoPruneOrphans_PreservesOrphanedStateResources`
     (default behavior) — same setup, `--prune-orphans=false` → htop
     stays installed, state unchanged, debug log emitted.
- **Integration**: extend `internal/provider/builtin/apt/integration/integration.sh`
  with a new section `assert_apply_prune_orphans_flow` that exercises
  the real apt path: install via CLI, delete the hamsfile, run apply
  without flag (expect no-op), run apply with `--prune-orphans`
  (expect htop uninstalled + state.htop.state=removed).

## Risks / Trade-offs

- **Risk**: users discover `--prune-orphans` and reach for it on a
  partially-checked-out repo, mass-uninstalling. → Mitigation: docs
  page and `apply --help` for the flag both explicitly call out
  "destructive; verify your hamsfiles are present before using".
  Default stays off so this requires an explicit conscious choice.
- **Risk**: state-file-only providers may exist for legitimate
  reasons we haven't enumerated (e.g., a provider whose hamsfile
  is generated and the source is currently in a different worktree).
  → Mitigation: skip is the default; opt-in is per-command. Users
  in this situation simply don't pass the flag.
- **Trade-off**: we now have two "no-op" reasons for stage-1
  selection (state-only without flag, hamsfile-less but state-less
  shouldn't reach stage-1 at all). The debug log already names the
  case. Acceptable.
- **Trade-off**: flag documentation lives in two places (cli-reference
  docs + cli-architecture spec). Spec is authoritative; docs link to
  the spec scenarios.

## Migration Plan

- This is a strict additive change: existing behavior unchanged. No
  data migration. No flag-removal deprecation cycle needed.
- Rollout steps:
  1. Land the spec delta + design + tasks.
  2. Implement: flag wiring + runApply change + unit tests + apt
     integration test extension.
  3. Verify locally (`task check` + `task ci:itest:run PROVIDER=apt`).
  4. Update docs (`docs/content/{en,zh-CN}/docs/cli/apply.mdx` + the
     `--prune-orphans` mention in the global-flags reference).
  5. Archive.
- **Rollback**: drop the flag from CLI registration. The runApply
  change is purely guarded by `flags.PruneOrphans`, so removing the
  flag stack-traces back to the pre-change behavior. No data
  consequences.

## Open Questions

None at design time. The agent debate already resolved the default
direction (skip) and the opt-in mechanism (per-command flag). All
implementation choices flow from those decisions.
