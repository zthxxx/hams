# Auto-record apt CLI complex invocations

## Why

The just-merged `clarify-apply-state-only-semantics` cycle established the
explicit boundary in the `apt Provider` spec: `hams apt install/remove`
auto-records ONLY the bare-name path (`hams apt install jq htop`). Three
classes of invocation are executed-but-not-recorded:

- Version pinning (`nginx=1.24.0`)
- Release pinning (`nginx/bookworm-backports`)
- Dry-run flags (`--download-only`, `--simulate`, etc.)

For dry-run flags this is correct end-state: dry-run means "don't change
the host", so recording is genuinely wrong. But for version/release
pinning the user IS making a real install — they just want hams to
remember it as something more structured than `{app: nginx}`. Today the
warning sends them to the hamsfile + apply path, which works but is
extra friction.

This proposal extends the auto-record contract to cover version- and
release-pinned installs by serialising the pin into the hamsfile schema.

## What Changes

- **Hamsfile schema (apt)**: extend the per-package entry shape so it
  can carry an optional `version: "<pin>"` and an optional
  `source: "<release>"` field. Backwards-compatible: bare-name entries
  still work, and the new fields are optional.
- **State schema (apt)**: store the requested version pin alongside the
  observed installed version so refresh/probe can detect drift if the
  host is upgraded out from under the pin.
- **CLI auto-record**: drop the version-pin / release-pin trip-wires
  from `isComplexAptInvocation`; instead, parse the pin from the
  install args and write the structured entry to hamsfile + state.
  Dry-run flags STAY in the trip-wire set — they are correctly
  unrecordable.
- **Plan (apt)**: when the recorded entry has a version pin, the plan
  for "already installed at the right version" is skip; "installed at
  a different version" is `update` (re-install with the pin); "absent"
  is `install`. Same for release pins.
- **Tests**: parametric unit tests over `(plain | version-pinned |
  release-pinned | both-pinned) × (install | re-install | update)`.
- **Apt integration test**: extend with a section that installs a
  version-pinned package (e.g., `nginx=1.24.0` if available in the
  bookworm archive at test time) and asserts the hamsfile + state row
  shape, then re-applies on a fresh hamsfile and asserts the pinned
  version is restored.
- **Docs**: document the version/release pinning syntax in
  `docs/content/{en,zh-CN}/docs/providers/apt.mdx` (file may not yet
  exist; create if needed).

## Impact

- **Affected specs**: `builtin-providers` (modify the `apt Provider`
  requirement to remove the version/release pin trip-wires + add the
  structured-entry recording behavior + new scenarios), `schema-design`
  (add the optional `version` and `source` fields to the apt resource
  schema).
- **Affected code**: `internal/provider/builtin/apt/apt.go`
  (`isComplexAptInvocation`, `handleInstall` parsing path), the
  hamsfile per-package shape if it is provider-typed,
  `internal/state/state.go` if a new field is needed.
- **Backwards compatibility**: preserved. Bare-name entries continue
  to round-trip. Older state files without version/source fields are
  read as "no pin", which matches today's behavior.
- **User-visible change**: `hams apt install nginx=1.24.0` will start
  auto-recording (it currently warns and skips). The new entry shape
  appears in the hamsfile when the user inspects it.

## Provenance

Surfaced from codex-review round 3 on dev branch (post-archive of
`clarify-apply-state-only-semantics`). Architect+user agent debate
(this session's transcript) decided to ship a narrow short-circuit
in-session (`isComplexAptInvocation` skipping auto-record on version
pin / release pin / dry-run flag) to close the immediate correctness
bug, and defer the structured-entry work to this proposal because it
requires deliberate hamsfile schema design.
