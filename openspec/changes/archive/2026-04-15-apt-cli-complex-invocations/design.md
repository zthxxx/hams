# Design: apt-cli-complex-invocations

## Context

The current apt CLI handler short-circuits auto-record on three classes of
"complex invocation" (commit `e9bef58`):

- Version pinning: `nginx=1.24.0`
- Release pinning: `nginx/bookworm-backports`
- Dry-run flags: `--download-only`, `--simulate`, etc.

For dry-run flags this is correct end-state (no host change → no record).
For version + release pins it's a friction point: apt installed nginx,
the user typed the install through `hams`, but they have to manually
edit the hamsfile to keep tracking it.

This change extends the apt auto-record contract to accept structured
`version` and `source` pins. Dry-run flags stay short-circuited.

## Goals / Non-Goals

**Goals:**

- Drop the `=` and `/` trip-wires from `isComplexAptInvocation`. Parse
  pins out of args; record structured entries.
- Hamsfile per-package entry shape gains optional `version` and
  `source` fields, both omitempty.
- State per-resource entry gains optional `requested_version` and
  `requested_source` fields (the user's pin) — distinct from the
  existing `version` field which is the OBSERVED dpkg version.
- Plan compares observed against requested; mismatch → `update`
  (re-install) action.
- Bare-name entries continue to round-trip with byte-identical YAML
  (no spurious empty-string fields).
- Existing state files without the new fields load as "no pin", which
  matches today's behavior.

**Non-Goals:**

- Do NOT extend pinning support to other providers (homebrew already
  has `--head`/version syntax; apt is the prototype). Each provider
  earns the schema extension via its own openspec change.
- Do NOT support APT pin-priority files (`/etc/apt/preferences.d/`).
  Those are system-level; hams stays out of them.
- Do NOT re-introduce a dpkg-based gate. The dry-run trip-wires in
  `isComplexAptInvocation` still cover the cases where dpkg can't
  distinguish "we installed it now" from "it was already there".
- Do NOT support multi-package version pinning in one command
  (`hams apt install nginx=1.24 redis=7`). The CLI accepts it (apt-get
  does), and we record each parsed pkg+pin individually — but a
  scenario in the spec restricts this to "supported but each package
  pin is recorded independently"; we don't promise atomic transaction
  semantics across multiple pins beyond what apt-get itself provides.

## Decisions

### Decision 1: Hamsfile API surface — `AddAppWithFields` (additive helper)

**Picked**: Add a new `AddAppWithFields(tag, appName, intro string, extra map[string]string)` to `hamsfile.File`. The existing `AddApp(tag, appName, intro string)` becomes a thin wrapper that calls `AddAppWithFields(tag, appName, intro, nil)`.

- `extra` is an ordered set of additional `key: value` pairs to emit on
  the entry's mapping node. Keys with empty values are omitted (so
  callers can pass `{"version": "", "source": ""}` for the bare-name
  case without polluting the YAML).
- The existing 16+ call sites of `AddApp` keep compiling unchanged.
- The new helper keeps the YAML layer concerns (node ordering, scalar
  escaping) inside the hamsfile package.

Alternative: change `AddApp`'s signature to take `extra ...string` or
restructure to take a struct. Rejected: noisy diff at all 16 call
sites, adds friction for providers that don't need pinning.

### Decision 2: Parse apt pins via a small `parseAptInstallToken(arg string)` helper

**Picked**: a 10-line helper in `apt.go` that returns
`(pkg, version, source string)` for a single arg. Logic:

- If arg contains `=`: split on first `=`; left = `pkg`, right = `version`.
- Else if arg contains `/`: split on first `/`; left = `pkg`, right = `source`.
- Else: pkg = arg, version = "", source = "".

Apt's actual grammar permits more (e.g., `pkg/release=version`),
but those forms are uncommon and the proposal scope explicitly limits
us to the two simple cases. Edge cases beyond this fall back to the
current "complex; warn and skip" path because they don't match the
parser.

Alternative: pull in `pault.ag/go/debian` for full apt syntax. Rejected:
heavyweight dependency for a 95% case the simple parser handles.

### Decision 3: State field names — `requested_version` and `requested_source`

**Picked**: keep `version` as the observed dpkg version (no semantic
change). Add `requested_version` and `requested_source` as the user's
pinned values. Drift is `requested_version != "" && version != requested_version`.

Alternative: rename `version` to `observed_version`, add a `requested`
substruct. Rejected: breaks every existing state file's YAML key
naming, would force a v3 schema migration. The additive approach
keeps schema_version=2 viable.

### Decision 4: `isComplexAptInvocation` keeps the dry-run trip-wires; loses `=` and `/`

**Picked**: the helper now only flags dry-run flags. `=` and `/` no
longer trigger short-circuit; `parseAptInstallToken` handles them
inside the recording loop.

### Decision 5: Plan re-install on drift uses the requested pin for the install command

**Picked**: when state drift is detected (observed version differs from
requested), the Plan emits an `Update` action whose payload carries the
pinned form (`pkg=version` or `pkg/source`). The existing apt
`Provider.Apply(ctx, action)` calls `runner.Install(ctx, []string{action.ID})`
— so `action.ID` becomes the pinned token rather than the bare pkg
name. This re-uses the executor path; no new code path needed.

Alternative: have the Plan emit a special "re-pin" action type. Rejected:
new action type ripples through executor + tests. Re-using `Update`
with the pinned token in `action.ID` is the smaller change.

## Risks / Trade-offs

- **Risk**: parsing `=`/`/` ambiguity. A package named `python3=foo`
  or `bash/legacy` is theoretically valid in Debian's namespace.
  Real-world: pkgs containing `=` in their name are essentially
  nonexistent; pkgs containing `/` would not be installable via apt
  in the first place (apt would interpret `/` as release pin).
  → Mitigation: rely on apt-get's own grammar — if apt accepts the
  token as `pkg/release`, we accept the same parse. Surface ambiguity
  in a future unit test if a concrete failure case appears.
- **Risk**: state files written by this change can't be read by
  older hams binaries (they'd ignore the new fields silently).
  → Mitigation: this is fine. New fields are omitempty, so old hams
  binaries still see a valid state file shape. The `requested_*`
  fields just won't influence Plan output on older binaries.
- **Trade-off**: `AddAppWithFields` introduces a second hamsfile
  insertion API. Callers that need only the bare-name path use
  `AddApp` (unchanged); apt's pinned-install path uses the new
  helper. Two-API minor friction is preferable to a 16-site
  signature change.
- **Trade-off**: hamsfile YAML round-trip semantics for the new
  fields rely on YAML node-tree manipulation matching what
  buildAppEntry does today. Add a property test that asserts
  `(app, intro?, version?, source?)` round-trips byte-for-byte
  given the same input.

## Migration Plan

- **Schema**: state SchemaVersion stays at 2 (additive fields,
  backwards compatible). No state migration code needed.
- **Hamsfile**: no schema version (it's free-form YAML); the apt
  Hamsfile now accepts new optional fields. Existing files round-trip.
- **CLI behavior change**: invocations like `hams apt install nginx=1.24.0`
  that currently warn and skip will START auto-recording. Document
  this in the apt provider doc page.
- **Rollout**:
  1. Land hamsfile API extension + tests.
  2. Land state schema extension + tests.
  3. Land apt parsing + recording + Plan changes + tests.
  4. Extend apt integration test with a version-pin scenario (use a
     pkg+version that's stable in bookworm: `jq=1.6-2.1+deb12u1`).
  5. Update provider docs (en + zh-CN).
  6. Verify + archive.
- **Rollback**: revert the apt.go + hamsfile.go + state.go changes;
  the unused `requested_*` fields in already-written state files are
  ignored by the older apt binary (omitempty + no Plan logic).

## Open Questions

None at design time. Decisions 1-5 cover the contracted surface. Edge
cases (apt grammar forms beyond `=` and `/`) are explicitly out of
scope and continue to fall back to the warn-and-skip short-circuit.
