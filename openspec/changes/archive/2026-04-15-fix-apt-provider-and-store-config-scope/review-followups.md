# Simplify review follow-ups

`/simplify` (Group 9.3) ran three parallel review agents against the diff of this
change. Below is the aggregated triage — fixes applied in-scope, everything else
deferred to a future change with full provenance so it is not lost.

## Applied in this change (simplify-tidy commit)

1. **Removed `TestNewRealCmdRunner_ConstructsImpl`** (tautology — asserted the
   compiler would return the right type from a single-field struct constructor).
2. **Reduced `FakeCmdRunner` public surface** — `Installed`/`Calls` are now
   unexported (`installed`/`calls`); `FakeCall` → unexported `fakeCall`; added
   `Seed(pkg, version)` so tests don't reach into the fake's internal state.
   String op values are now `fakeOpInstall` / `fakeOpRemove` /
   `fakeOpIsInstalled` constants (still passed to `CallCount` as strings —
   that's the intended test API).
3. **Trimmed WHAT-comments** on `apt.Provider.{Probe,Apply,Remove}` — dropped
   "via the DI-injected runner" / "via the DI runner" restatements that added
   nothing beyond the method name.

## Deferred — tracked for follow-up change `apt-shared-provider-helpers`

The following findings are real cross-provider refactors, not regressions from
this change. Bundling them here would expand the blast radius beyond
`fix-apt-provider-and-store-config-scope`'s scope. Open a new OpenSpec change
before addressing:

1. **Extract `loadOrCreateHamsfile` + `hamsfilePath` + `effectiveConfig` to a
   shared helper.** Reviewer found copies in:
   - `internal/provider/builtin/homebrew/homebrew.go:436-501` (reference)
   - `internal/provider/builtin/apt/hamsfile.go:24-90` (new — this change)
   - `internal/provider/builtin/git/clone.go:265-318` (drift — missing `Profile`
     override in its `effectiveConfig`)
   - `internal/provider/builtin/defaults/defaults.go:169-184` (just
     `effectiveConfig`)

   Proposed target: `internal/provider/hamsfile_helper.go` with package-level
   helpers taking `manifest provider.Manifest` + `cfg *config.Config` +
   `flags *provider.GlobalFlags`. Collapsing to one copy also fixes the latent
   Profile-override drift in `git/clone.go`.

2. **Extract `packageArgs` flag filter.** Byte-for-byte identical between
   `internal/provider/builtin/apt/apt.go:213-222` (new) and
   `internal/provider/builtin/homebrew/homebrew.go:516-525`. Promote to
   `internal/provider/args.go` alongside `ParseVerb`.

3. **Push idempotency into `hamsfile.AddApp`.** Today apt guards with
   `FindApp` before `AddApp`; brew does not guard, which means re-installing
   an already-recorded package appends a duplicate entry. Option A: make
   `AddApp` an upsert. Option B: fix brew to guard. Option A is cleaner.

4. **Hoist `timestampFormat = "20060102T150405"` to a shared package.** Used
   in `internal/state/state.go:12`, `internal/state/lock.go:42`,
   `internal/logging/logging.go:64`, `internal/otel/otel.go:176` — three raw
   string literals that could reference one constant.

5. **Delegate `realCmdRunner` streaming to a shared helper.** `apt.realCmdRunner`
   inlines `cmd.Stdout = os.Stdout; cmd.Stderr = os.Stderr; cmd.Run()` per op.
   A tiny `provider.RunPassthroughCmd(*exec.Cmd) error` helper would deduplicate
   this with `provider.WrapExecPassthrough` (which can't be reused directly
   here because it builds its own `exec.Cmd`).

6. **Batch `dpkg-query` in `apt.Probe`.** Current code fires one subprocess per
   resource; `dpkg-query -W -f='${Package} ${Status} ${Version}\n' pkg1 pkg2 …`
   returns all in one spawn. Impact: ~150-300ms → ~20ms on stores with
   ≤30 apt packages. Warm path (every probe/refresh), not hot enough to block
   this change but worth doing.

## False positives from the review

The reviewers also flagged these, which were investigated and dismissed:

- **`apt.Provider{cfg, runner}` redundant state** — `cfg` and `runner` serve
  different purposes (hamsfile mutation vs command execution). Not derivable.
- **`realCmdRunner` single-field struct over-engineered** — three methods share
  it, a struct groups them cleanly. Function-per-method would be noisier.
- **`Resource.RemovedAt` as `string` vs `*time.Time`** — consistent with
  `FirstInstallAt` / `UpdatedAt` / `CheckedAt` in the same struct. Lex-sortable
  (relied on by `yaml_assert.sh:89`). Spec decision D3.
- **`legacyFile`/`legacyResource` duplication** — textbook one-way migration
  pattern; the alternative (strict-unmarshal + sidecar) is worse.
- **`SetResource(id, StateOK)` always bumps `UpdatedAt`** — intentional per
  spec D4. `updated_at` is "last reconciled", not "last changed".
- **Sequential apt install loop** — `apt-get` holds
  `/var/lib/dpkg/lock-frontend`; parallel spawns would serialize anyway and
  mangle stdout.
- **Migration runs on every `Load`** — cold path (6 CLI call sites, one per
  invocation). Allocation overhead bounded and negligible.
- **`Taskfile.yml` `test:*` (via act) vs `ci:*:run` (direct docker)
  duplication** — deliberate dev-vs-CI split, not redundant.
- **`yq` fetched from GitHub in the debian Dockerfile** — Debian's `yq` is
  a different Python tool; GitHub release is the canonical Go `yq`.
- **Error wrapping pattern** — `fmt.Errorf("apt-get install %s: %w", pkg, err)`
  matches 12 existing uses across 9 builtin providers.
- **Test names with `_U1` / `_S1` / `_C1` / `_E1` suffixes** — explicitly
  support traceability to spec scenarios. Intentional.
- **`e2e/lib/yaml_assert.sh`** — new cross-cutting helper, correctly placed.
