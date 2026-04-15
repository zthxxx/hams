# Clarify `hams apply` semantics for state-only providers

## Why

The just-archived change `fix-apt-cli-state-write-and-htop-rename` introduced
a two-stage provider filter on `hams apply` and `hams refresh`:

- **Stage 1 (artifact presence):** include any provider that has a hamsfile
  *or* a state file for the active profile/machine.
- **Stage 2 (`--only` / `--except`):** narrow within stage-1 results.

The intent (per the spec deltas in
`openspec/specs/cli-architecture/spec.md`) was to skip providers whose
upstream tool may not even be installed on this machine — preventing
spurious bootstrap failures when, e.g., a Linux machine has no `brew`.

A codex-review pass after archive flagged a real semantic gap in the
implementation (commit `2e3b63b`, file `internal/cli/apply.go:235-238`):

```go
if !mainExists && !localExists {  // no hamsfile (regardless of state file)
    slog.Debug("no hamsfile for provider, skipping", "provider", name)
    continue
}
```

A provider whose state file exists but whose hamsfile has been deleted
(e.g., the user installed `htop` via `hams apt install htop`, then
deleted `apt.hams.yaml` to "stop tracking it") is selected by stage-1
but skipped at this point — so `apply` does **nothing** even though the
declared desired state is empty and the state file still says
`htop=ok`.

This is a destructive-default question: should `apply` interpret a
missing hamsfile + non-empty state as "remove all"? An autonomous
2-agent debate (architect + user perspectives, recorded in this
session's transcript) concluded:

- The current "skip" behavior is defensible (Terraform-style — a missing
  file is a missing intent, not "I want zero").
- BUT the current behavior is undocumented and surprising to users who
  reason about state declaratively ("I deleted the file, why is it
  still installed?").
- Flipping the default to "implicit prune" has a high blast radius: a
  partial `git checkout` or accidental file deletion would mass-uninstall.
- The right path: make the behavior **explicit**, document it, and
  introduce an opt-in flag (`--prune-orphans` or equivalent) for users
  who want destructive reconciliation.

This change formalizes that decision in the spec and adds the opt-in
flag.

## What Changes

- **CLI:** add `--prune-orphans` flag to `hams apply` (default `false`).
  When set, providers with state-only (no hamsfile, no `.local.yaml`)
  are processed using an empty desired-state — Plan computes "remove
  every resource currently in state". Without the flag, the existing
  skip behavior is preserved verbatim.
- **Spec (cli-architecture):** add the explicit "Apply skips state-only
  providers by default; `--prune-orphans` opts into destructive
  reconciliation" requirement with WHEN/THEN scenarios for both modes.
- **Spec (cli-architecture):** clarify the existing two-stage filter
  requirement to spell out the state-only case (currently silent).
- **Tests:** unit tests for both modes; integration scenario for
  apt that exercises the prune path end-to-end.
- **Docs:** mention `--prune-orphans` in `docs/content/{en,zh-CN}/docs/cli/`
  apply page.

## Impact

- **Affected specs:** `cli-architecture` (modify Apply requirement,
  add prune-orphans requirement).
- **Affected code:** `internal/cli/apply.go`, `internal/cli/commands.go`
  (flag wiring), apt integration test fixture.
- **Backwards compatibility:** preserved. Default behavior unchanged
  (skip state-only providers); the prune behavior is opt-in only.
- **User-visible change:** new `--prune-orphans` flag; updated CLI
  reference docs.

## Provenance

- Codex review on dev branch (post-archive): finding "Reconcile state-only
  providers instead of skipping them" — `internal/cli/apply.go:138-138`.
- Architect/user agent debate in conversation transcript decided to
  defer rather than patch in-session, on the basis that destructive
  default flips warrant explicit spec scenarios + an opt-in path.
- Sibling P2 from the same review (apt CLI flag passthrough + multi-pkg
  atomicity) was fixed in-session as commit `fcc3415` because it was
  local + non-destructive.
