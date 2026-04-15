# Fix apt pin replay through `hams apply` and the in-place hamsfile upgrade path

## Why

Cycle 3 (`apt-cli-complex-invocations`, archived 2026-04-15) shipped apt
CLI version + release pinning for the imperative path
(`hams apt install nginx=1.24.0`). Codex review on the archived branch
found three functional bugs that break the cycle's spec scenarios on
the canonical declarative + restore paths:

1. **Plan ignores hamsfile-declared pins.** `apt.Plan` calls
   `desired.ListApps()` which returns just app names; the structured
   `version` and `source` fields are stripped before Plan ever sees
   them. So a hamsfile authored as `{app: nginx, version: "1.24.0"}`
   — or restored from a store via `hams apply --from-repo=...` —
   produces a plain `apt-get install nginx` (no pin). Hams' two
   headline workflows (hand-edit + apply, fresh-machine restore)
   silently lose the pin.
2. **Drift Plan corrupts the state key.** The cycle-3 Plan rewrites
   `action.ID` from `"nginx"` to `"nginx=1.24.0"` to carry the
   install token. The executor saves state under that mutated key
   (`sf.SetResource(action.ID, ...)`), leaving the original
   `resources["nginx"]` row stale. Subsequent applies see two rows:
   one drifted (still planned for update), one orphaned.
3. **Existing bare entries don't pin-upgrade.** `handleInstall`'s
   loop guards with `if FindApp(pkg) == ""` before
   `AddAppWithFields`. So `hams apt install nginx=1.24.0` against a
   hamsfile that already lists `{app: nginx}` (bare) records the pin
   in state but leaves the hamsfile entry unchanged. On another
   machine, restoring from this hamsfile installs nginx without the
   pin.

Together these break the spec scenarios that cycle 3 itself committed
to:

- "Version-pinned install records structured entry" — works for the
  empty-state imperative path only.
- "Plan re-installs when host version differs from pin" — runs apt-get
  correctly but leaves state corrupted.

The architectural root cause is the same in all three: cycle 3 smuggled
the "pin form" through `action.ID` and skipped a hamsfile API
extension that would let Plan and the imperative recorder both work
against structured fields. `provider.Action` already carries a
`Resource any` field designed for exactly this kind of executor
payload — cycle 3 bypassed it.

## What Changes

- **Hamsfile API**: add `(*File).AppFields(name string) map[string]string`
  returning the structured per-app fields (mirror of `FindApp`'s walk).
- **Hamsfile API**: change `AddAppWithFields` so that when the named
  app already exists (under any tag), it MERGES the new structured
  fields into the existing entry instead of skipping. Bare-name entries
  upgraded to pinned by passing `extra` with non-empty values; passing
  empty extras is a no-op (preserves backwards compatibility).
- **Apt Plan**: read each app's structured fields via the new
  `AppFields` API. Combine with the existing observed-state drift
  detection. Emit Install/Update actions whose `ID` is the bare
  package name AND whose `Resource` field carries the install-token
  form (`pkg=version` / `pkg/source`). Apply uses `Resource` if
  present; falls back to `ID`.
- **Apt handleInstall**: drop the `if existingTag == ""` guard. The
  bookkeeping loop now ALWAYS calls `AddAppWithFields` (which is now
  idempotent + merging).
- **Tests**: cover all three regression paths via unit tests + extend
  apt itest E7 with a fresh-machine restore scenario (delete state,
  keep pinned hamsfile, run apply, assert the pin reaches apt-get
  AND state ends up with the right `requested_version`).

## Impact

- **Affected specs**: `builtin-providers` (modify three apt scenarios
  to assert the new contract).
- **Affected code**: `internal/hamsfile/hamsfile.go`,
  `internal/provider/builtin/apt/apt.go`. No state schema change (the
  new state fields landed correctly in cycle 3).
- **Backwards compatibility**: preserved. Bare-name entries stay
  bare. Existing pinned entries continue to round-trip. Old state
  files keep working (the new fields are still omitempty + already in
  the schema since cycle 3).
- **User-visible change**: `hams apply` now actually honors hamsfile
  pins; drift apply leaves state coherent (one row per package, not
  two). Restoring a pinned store on a fresh machine installs the
  pinned versions.

## Provenance

Surfaced from codex-review round 4 on dev branch (post-archive of
cycle 3). Architect+user agent debate (this session's transcript)
recommended in-session fix because the cycle-3 spec scenarios are
currently broken, the fix is local (~200 LoC), and the architecture
already provides the right channel (`Action.Resource`) — cycle 3 just
didn't use it.
