# Tasks

## 1. Hamsfile API extension

- [x] 1.1 Add `(*hamsfile.File).AddAppWithFields(tag, appName, intro string, extra map[string]string)` in `internal/hamsfile/hamsfile.go`. Refactor existing `AddApp` to call `AddAppWithFields(tag, appName, intro, nil)` so all current call sites keep working unchanged.
- [x] 1.2 Update `buildAppEntry` (or extract a `buildAppEntryWithFields`) to accept the optional `extra` map and emit each non-empty key/value pair as a YAML scalar pair on the entry mapping node. Skip empty-string values to keep bare-name entries identical byte-for-byte.
- [x] 1.3 Add unit tests in `internal/hamsfile/hamsfile_test.go`:
  - `TestAddAppWithFields_RoundTripsExtraFields`: write `{app: nginx, version: "1.24.0", source: "bookworm-backports"}`, read back, assert all three fields present.
  - `TestAddAppWithFields_BareNameDoesNotEmitEmptyFields`: write with empty extras, assert serialized YAML has only `app`, no spurious `version: ""` or `source: ""` keys.
  - `TestAddApp_BackwardsCompatibility`: existing AddApp call sites (no extras) produce identical YAML to before this change.

## 2. State schema extension

- [x] 2.1 In `internal/state/state.go`, add two optional fields to the `Resource` struct: `RequestedVersion string \`yaml:"requested_version,omitempty"\`` and `RequestedSource string \`yaml:"requested_source,omitempty"\``. Position them adjacent to the existing `Version` field for grouping.
- [x] 2.2 Add `WithRequestedVersion(v string) ResourceOption` and `WithRequestedSource(s string) ResourceOption` helpers in the same file (mirror the existing `WithVersion` pattern).
- [x] 2.3 Update `internal/state/migration.go`'s `LegacyResource` so v1→v2 migration carries through (the new fields are v2-additive, so legacy v1 files load with empty pins — verify nothing breaks).
- [x] 2.4 Add unit tests in `internal/state/state_test.go`:
  - `TestSetResource_WithRequestedVersion_RoundTrips`: set + save + load; assert `RequestedVersion` preserved.
  - `TestSetResource_BareDoesNotEmitEmptyRequestedFields`: bare set without options produces YAML with no `requested_version` / `requested_source` keys.

## 3. Apt args parsing

- [x] 3.1 Add `parseAptInstallToken(arg string) (pkg, version, source string)` in `internal/provider/builtin/apt/apt.go`. Logic per design Decision 2: `=` split → (pkg, version); `/` split (when no `=`) → (pkg, source); else (arg, "", "").
- [x] 3.2 Add unit tests in `internal/provider/builtin/apt/apt_test.go`:
  - `TestParseAptInstallToken_BareName`: `"nginx"` → `("nginx", "", "")`.
  - `TestParseAptInstallToken_VersionPin`: `"nginx=1.24.0"` → `("nginx", "1.24.0", "")`.
  - `TestParseAptInstallToken_ReleasePin`: `"nginx/bookworm-backports"` → `("nginx", "", "bookworm-backports")`.
  - `TestParseAptInstallToken_Empty`: `""` → `("", "", "")`.

## 4. Drop `=` and `/` trip-wires from `isComplexAptInvocation`

- [x] 4.1 In `internal/provider/builtin/apt/apt.go::isComplexAptInvocation`, remove the `strings.ContainsAny(a, "=/")` check. Keep the `aptDryRunFlags` set check.
- [x] 4.2 Update the docstring to reflect that the helper now ONLY trips on dry-run flags.
- [x] 4.3 Verify existing U22 (version pin) and U23 (release pin) tests now FAIL (they assert no auto-record, but the new behavior will record) — update them to assert the NEW recording behavior with structured fields. Rename to U22 → `TestHandleCommand_U22_VersionPinRecordsStructuredEntry` and U23 → `TestHandleCommand_U23_ReleasePinRecordsStructuredEntry`.
- [x] 4.4 U20 (`--download-only`) and U21 (`-o KEY=VAL`) MUST keep failing-to-record. U21's `-o` arg has `=` in its value but the `-` prefix on the flag itself causes packageArgs to drop it; the value is then standalone but the install still includes it via apt-get's `-o` consumption — re-verify that U21's expected behavior is still "no record" (the value parses as `(KEY, VAL, "")` with `KEY` as pkg name; whether this records depends on whether the fake's IsInstalled returns true for `KEY`. Decide: either keep U21 as "complex via dry-run-style flag presence" or drop U21 in favor of simpler coverage). Document the decision in the test comment.

## 5. Apt CLI auto-record path: parse pins + record structured entries

- [x] 5.1 In `handleInstall`, after the `runner.Install(ctx, args)` succeeds AND after the (now narrower) `isComplexAptInvocation` short-circuit, refactor the bookkeeping loop:
  - For each `arg` in `args` (skipping flag-prefixed args), call `parseAptInstallToken(arg)` to get `(pkg, version, source)`.
  - If `pkg == ""`, continue (defensive).
  - Build the hamsfile `extra` map: `{"version": version, "source": source}` (the helper drops empty values).
  - Call `hf.AddAppWithFields(tagCLI, pkg, "", extra)` — only when `FindApp(pkg)` returns no existing tag (idempotency guard).
  - Probe `runner.IsInstalled(ctx, pkg)` for the observed version.
  - Build state options: `state.WithVersion(observed)`, plus `state.WithRequestedVersion(version)` and `state.WithRequestedSource(source)` when non-empty.
  - `sf.SetResource(pkg, state.StateOK, opts...)`.
- [x] 5.2 `handleRemove` does NOT need a parser change — apt-get accepts `pkg=version` for remove (means "remove this version"), but hams's auto-record removes the bare entry from the hamsfile. Keep the existing flow; add a small comment explaining the asymmetry.

## 6. Plan: drift-driven Update on version mismatch

- [x] 6.1 In `internal/provider/builtin/apt/apt.go::Plan`, before calling `provider.ComputePlan`, walk `desired.ListApps()` and `observed.Resources` to detect drift: a resource where `requested_version != ""` AND `requested_version != observed.version`. For each drift, append a `provider.Action{ID: pkg+"="+requested_version, Type: provider.ActionUpdate}` to the plan.
- [x] 6.2 Symmetric: if `requested_source != ""` AND the entry isn't observed-installed (rare; release pins don't drift the same way), emit `Action{ID: pkg+"/"+requested_source, Type: provider.ActionUpdate}`.
- [x] 6.3 The existing `Provider.Apply` path passes `action.ID` to `runner.Install(ctx, []string{action.ID})` — so the pinned token reaches apt-get verbatim. Verify with a unit test.
- [x] 6.4 Add unit tests in `internal/provider/builtin/apt/apt_test.go`:
  - `TestPlan_VersionDriftEmitsUpdate`: hamsfile declares `{app: nginx, version: "1.24.0"}`, state has `nginx.version=1.22.1`, `nginx.requested_version=1.24.0`; assert Plan emits `Update` action with `ID="nginx=1.24.0"`.
  - `TestPlan_VersionMatchEmitsSkip`: same hamsfile, state has `nginx.version=1.24.0`, requested matches → assert Plan emits `Skip`.

## 7. Apt integration test extension

- [x] 7.1 Add a new section `assert_version_pin_flow` to `internal/provider/builtin/apt/integration/integration.sh`:
  - Run `hams apt install jq=1.6-2.1+deb12u1` (a version that's stable in bookworm at integration-test time).
  - Assert `apt.hams.yaml` contains `{app: jq, version: "1.6-2.1+deb12u1"}`.
  - Assert `apt.state.yaml.resources.jq.requested_version == "1.6-2.1+deb12u1"`.
  - Assert `apt.state.yaml.resources.jq.version` matches what `dpkg -s jq` reports.
  - Run `hams apply --only=apt` and assert it's a no-op (state matches request).
  - (Optional) test drift: manually `apt-get install -y --allow-downgrades jq=<other-version>` if a second version is in the index, then `hams apply` should re-pin. Skip if no second version available — the unit tests cover the drift logic.

## 8. Docs sync

- [x] 8.1 Find or create `docs/content/en/docs/providers/apt.mdx`. Document the version-pin (`hams apt install nginx=1.24.0`) and release-pin (`hams apt install nginx/bookworm-backports`) syntax.
- [x] 8.2 Mirror in `docs/content/zh-CN/docs/providers/apt.mdx`.
- [x] 8.3 Update `docs/content/en/docs/cli/apply.mdx` if any flag descriptions need adjusting (likely not — `--prune-orphans` semantics are unchanged).

## 9. Verification

- [x] 9.1 `task fmt` clean.
- [x] 9.2 `task lint:go` clean.
- [x] 9.3 `task test:unit` green with `-race` (incl. all new unit tests across hamsfile, state, apt).
- [x] 9.4 `task ci:itest:run PROVIDER=apt` green end-to-end (incl. the new `assert_version_pin_flow` section).
- [x] 9.5 `task ci:itest` (full sweep across all 11 providers) green — confirms hamsfile schema additive change didn't break any other provider.

## 10. Archive

- [x] 10.1 `/opsx:verify apt-cli-complex-invocations` — 0 critical / 0 warning. All 5 scenarios mapped to code or tests.
- [x] 10.2 `/opsx:archive apt-cli-complex-invocations` — archived with `--skip-specs` (auto-sync header-matching bug); builtin-providers delta applied to main spec manually (1 MODIFIED + 1 ADDED, requirement count 24→25).
