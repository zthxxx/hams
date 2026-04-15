# Design: fix-apt-pin-apply-path

## Context

Cycle 3 shipped apt CLI version + release pinning but missed three
correctness requirements that the spec scenarios assert:

1. Pin authored in hamsfile (or restored from a store) → Plan strips
   it via `desired.ListApps()`.
2. Drift Update rewrites `action.ID` to the install token →
   executor saves state under the mutated key, leaving an orphan row.
3. `handleInstall` skips `AddAppWithFields` when the entry already
   exists → existing bare entries never gain pins.

All three share one architectural shape: cycle 3 used `action.ID` as
the install-token channel, and skipped a hamsfile read API.

## Goals / Non-Goals

**Goals:**

- Make `Plan` honor hamsfile-declared pins on the apply path
  (declarative + restore both work).
- Make drift Update keep state coherent (single row per package,
  keyed on the bare name).
- Make `handleInstall` upgrade existing bare entries in place when
  the user re-runs with a pin.
- Reuse the existing `provider.Action.Resource` field for the
  install-token payload (designed for this case).
- Fix all three with one new hamsfile read API + one
  AddAppWithFields semantic tweak; no schema change, no executor
  change.

**Non-Goals:**

- Do NOT introduce a new state key form (e.g.,
  `nginx@1.24.0`). Keep the bare name as the canonical state key.
- Do NOT touch the dry-run short-circuit path; cycle 3's contract
  there is correct.
- Do NOT extend pin support to other providers; this is the apt-only
  follow-up.
- Do NOT add a parallel `SetAppFields` API. `AddAppWithFields`
  becomes idempotent + merging — single helper, one mental model.

## Decisions

### Decision 1: AddAppWithFields becomes idempotent + merging

**Picked**: when `AddAppWithFields(tag, name, intro, extra)` is
called and `name` already exists under any tag, MERGE non-empty
extras into the existing entry's mapping node instead of appending
a duplicate. Empty extras are no-ops on the existing entry.

The semantic shift: "Add" now means "ensure entry exists with these
fields". This matches every existing call-site:

- apt CLI imperative recording: pin is preserved on re-run.
- Future providers using the helper: same idempotent intent.
- Tests: bare-name entries continue to round-trip; pinned entries
  continue to round-trip; bare→pinned upgrade now works.

Alternative: add a separate `SetAppFields(name, extra)` method.
Rejected: two-API confusion. The merge-on-existing semantic is
strictly more useful than the previous "skip if exists".

### Decision 2: New `AppFields(name)` read API on hamsfile

**Picked**: add `(*File).AppFields(name string) map[string]string`
that walks the YAML node tree once and returns the entry's
structured fields (everything except `app` and `intro`).

`apt.Plan` calls this for each `desired.ListApps()` entry to recover
the pin.

Alternative: have `desired.ListApps()` return a richer type. Rejected:
breaks every existing caller (dozens). The narrow read API has zero
ripple.

### Decision 3: Use `action.Resource` to carry the install-token form

**Picked**: when Plan emits a pinned Install/Update, set
`action.ID = "<bare-pkg>"` AND `action.Resource = "<bare-pkg>=<ver>"`
(or `"<bare-pkg>/<source>"` for release pins). `Provider.Apply` reads
`action.Resource` first; if the type-assert succeeds, pass it to the
runner; else fall back to `action.ID`.

This preserves the executor's contract: state keys stay bare; install
commands carry the pin.

`provider.Action.Resource` is `interface{}` (any) by design — it's
the "executor payload" channel. Cycle 3 left it nil; we now populate
it with a string. Other providers that don't care continue to leave
it nil and use `action.ID`.

Alternative: add a typed `Action.AptPin` field. Rejected: provider-
specific field on the shared Action type pollutes the abstraction.
The string-in-Resource approach is exactly what the field is for.

### Decision 4: Drop the FindApp guard in handleInstall

**Picked**: with Decision 1, the guard is now harmful — it prevents
the in-place pin upgrade. Remove the conditional; always call
`AddAppWithFields`.

The previous guard's ostensible purpose ("idempotency: don't add
duplicate entries") is now baked into AddAppWithFields itself.

### Decision 5: Apt itest E7 gains a fresh-machine restore scenario

**Picked**: extend integration.sh E7 with a step that:
1. Records a pin via the imperative path (existing).
2. Saves the resulting hamsfile content.
3. Wipes both state and hamsfile.
4. Restores the saved hamsfile.
5. Runs `hams apply --only=apt`.
6. Asserts the host is back to the pinned version AND state is
   coherent (single row, bare key, requested_version preserved).

This locks the apply-from-hamsfile path into CI.

## Risks / Trade-offs

- **Risk**: a future provider that uses `AddAppWithFields` and
  intends "skip if exists" behavior would break. → Mitigation:
  comment the API contract; the merge semantic is more useful for
  every plausible caller, so this risk is theoretical.
- **Risk**: `Provider.Apply` reading `action.Resource` requires a
  type-assert that could panic if a future planner sets it to a
  non-string. → Mitigation: use the comma-ok form
  (`if s, ok := action.Resource.(string); ok && s != ""`).
- **Trade-off**: `AppFields` returns `map[string]string` but the
  YAML node values can be any scalar. We only use it for `version`
  and `source` (both strings); generalising later would require
  changing the return type.

## Migration Plan

- Schema unchanged. State files written by cycle-3 still load. Bare
  hamsfile entries still load.
- One subtle behavior change: `hams apt install nginx=1.24.0` against
  a hamsfile with bare `{app: nginx}` now mutates the YAML in place.
  This is the intended behavior (it's what users actually want), and
  no current test asserts the old (buggy) behavior.
- Rollback: revert this change. The cycle-3 archived spec scenarios
  return to broken; user's pinned installs lose their pin on apply.
  Users on the buggy version would have hamsfile bare entries even
  after re-installing with pins; that's the same data loss they
  already had.

## Open Questions

None at design time.
