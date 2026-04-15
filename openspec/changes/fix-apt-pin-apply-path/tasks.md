# Tasks

## 1. Hamsfile API: AppFields read + AddAppWithFields merge semantics

- [ ] 1.1 Add `(*File).AppFields(name string) map[string]string` in `internal/hamsfile/hamsfile.go`. Walk the document mapping node once; for the first entry whose `app` scalar matches `name`, return a map of every other key/value scalar pair (excluding `app` and `intro`). Return nil when no entry matches.
- [ ] 1.2 Modify `AddAppWithFields` so that when `FindApp(name)` returns a non-empty tag, MERGE non-empty extras into the existing entry's mapping node instead of skipping. Empty extras are no-ops on the existing entry. Append (current behavior) is preserved when no existing entry matches.
- [ ] 1.3 Add unit tests in `internal/hamsfile/hamsfile_test.go`:
  - `TestAppFields_ReturnsStructuredFields` — `{app: nginx, version: "1.24.0", source: "bp"}` → `{"version":"1.24.0","source":"bp"}`.
  - `TestAppFields_ReturnsNilForUnknown` — entry not present → nil.
  - `TestAppFields_ReturnsNilOrEmptyForBare` — `{app: htop}` → nil or empty (test tolerates both).
  - `TestAddAppWithFields_UpgradesBareEntryToPinned` — start with `{app: nginx}`; call `AddAppWithFields("cli", "nginx", "", {"version":"1.24.0"})`; assert YAML now reads `app: nginx` followed by `version: 1.24.0` (single entry, no duplicate).
  - `TestAddAppWithFields_EmptyExtrasOnExistingIsNoop` — start with `{app: nginx, version: "1.24.0"}`; call with empty extras; YAML round-trips byte-identical.

## 2. Apt Plan: read pins + use action.Resource

- [ ] 2.1 In `internal/provider/builtin/apt/apt.go::Plan`, walk `desired.ListApps()` and for each app call `desired.AppFields(app)` to recover the requested version + source. Build a per-app `requestedVersion` / `requestedSource` map BEFORE calling `provider.ComputePlan`.
- [ ] 2.2 After `ComputePlan` returns, walk the actions:
  - For each `Install` action whose app has a hamsfile pin: set `actions[i].Resource = pkg + "=" + version` (or `pkg + "/" + source`). LEAVE `ID` as the bare pkg name.
  - For each `Skip` action: apply the existing drift detection (observed != requested), but now also consult the HAMSFILE pin (the state pin may be empty on a fresh machine; the hamsfile is the source of truth). If drift is detected, promote to `Update` and set `Resource` to the install-token form. Keep `ID` bare.
- [ ] 2.3 Drop the existing apt.go drift logic that mutates `action.ID = a.ID + "=" + r.RequestedVersion` — it's the bug we're fixing.

## 3. Apt Apply: prefer Resource over ID

- [ ] 3.1 In `apt.go::Apply`, if `action.Resource` is a non-empty string, pass that to `runner.Install`; else pass `action.ID`. Use the comma-ok type assertion.

## 4. Apt handleInstall: drop the FindApp guard

- [ ] 4.1 In `apt.go::handleInstall`, remove the `if existingTag, _ := hf.FindApp(pkg); existingTag == ""` guard. Always call `hf.AddAppWithFields(tagCLI, pkg, "", extra)`. The new merge semantic in AddAppWithFields handles existing entries correctly.

## 5. Unit tests: apt Plan + handleInstall

- [ ] 5.1 Add `TestPlan_HamsfilePinReplaysOnFreshMachine` — hamsfile `{app: nginx, version: "1.24.0"}`, empty state. Assert Plan emits Install with `ID="nginx"`, `Resource="nginx=1.24.0"`. Then assert Apply (via the real Provider.Apply) calls `runner.Install(["nginx=1.24.0"])`.
- [ ] 5.2 Add `TestPlan_DriftKeepsBareIDAndCarriesPinInResource` — hamsfile `{app: nginx, version: "1.24.0"}`, state has `nginx.version="1.22.1"`. Assert Plan emits Update with `ID="nginx"`, `Resource="nginx=1.24.0"`. Verify state remains a single row keyed on `nginx` after a synthetic Apply.
- [ ] 5.3 Add `TestHandleCommand_U29_BareToPinnedUpgrade` — pre-write hamsfile with `{app: nginx}` (bare). Run `hams apt install nginx=1.24.0`. Assert hamsfile now has `{app: nginx, version: "1.24.0"}` (single entry, in-place upgrade), state has `requested_version: "1.24.0"`.
- [ ] 5.4 Update existing `TestPlan_VersionDriftEmitsUpdate` and `TestPlan_VersionMatchEmitsSkip` to assert the new `Resource` field shape.

## 6. Apt integration test: fresh-machine restore scenario

- [ ] 6.1 Extend `internal/provider/builtin/apt/integration/integration.sh` E7 with a sub-section "fresh-machine restore replays pin":
  1. (after the existing E7 pin install) Save the current `apt.hams.yaml` contents into a shell variable.
  2. Wipe `apt.state.yaml` and `apt.hams.yaml` (simulate fresh machine).
  3. Restore the saved hamsfile content to `apt.hams.yaml` (simulate `hams apply --from-repo=...`).
  4. `apt-get remove -y jq` so the host is unpinned.
  5. Run `hams --store="$HAMS_STORE" apply --only=apt`.
  6. Assert `dpkg -s jq` reports the pinned version.
  7. Assert `apt.state.yaml.resources.jq.requested_version` matches the pin.
  8. Assert `apt.state.yaml.resources` has exactly ONE entry for jq (no duplicate `jq=<ver>` orphan).

## 7. Verification

- [ ] 7.1 `task fmt` clean.
- [ ] 7.2 `task lint:go` clean.
- [ ] 7.3 `task test:unit` green with `-race`.
- [ ] 7.4 `task ci:itest:run PROVIDER=apt` green incl. the new E7 restore sub-section.
- [ ] 7.5 `task ci:itest` (full sweep) green — no other provider regresses from the AddAppWithFields semantic change.

## 8. Archive

- [ ] 8.1 `/opsx:verify fix-apt-pin-apply-path` — assert spec scenarios map to code, no critical/warning findings.
- [ ] 8.2 `/opsx:archive fix-apt-pin-apply-path` — sync spec deltas (will likely require `--skip-specs` + manual application).
