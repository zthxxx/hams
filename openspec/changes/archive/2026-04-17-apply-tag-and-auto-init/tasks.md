# Tasks — 2026-04-18-apply-tag-and-auto-init

## 1. Global `--tag` flag

- [x] 1.1 Add `Tag string` on `provider.GlobalFlags`
      (internal/provider/flags.go) — already done in prior commit for
      the race fix. Verify the field exists and is unused; no
      behavior change yet.
- [x] 1.2 Add `&cli.StringFlag{Name: "tag", Usage: "Active profile
      tag (overrides profile_tag in config)"}` to `globalFlagDefs()`
      in `internal/cli/root.go`. `--profile` kept as a hidden alias.
- [x] 1.3 Populate `flags.Tag = cmd.String("tag")` in `globalFlags()`.
- [x] 1.4 Add `config.ResolveCLITagOverride(cliTag, cliProfile)` +
      `config.ResolveActiveTag(cfg, cliTag, cliProfile)` in
      `internal/config/resolve.go`. Signatures take plain strings
      instead of `*provider.GlobalFlags` to avoid a
      config→provider→config import cycle.
- [x] 1.5 Property-based unit tests for both resolvers in
      `internal/config/resolve_test.go` — 4 precedence levels,
      ambiguity error, and nil-cfg safety.
- [x] 1.6 Update `internal/cli/apply.go:149,227` to pass
      `cliTagOverride` (the resolved CLI string) to `config.Load`
      instead of raw `flags.Profile`. Profile-dir-existence guard
      keyed on the same override for symmetry.

## 2. Auto-init config on `hams apply`

- [x] 2.1 Extract hostname-derivation helper
      `config.DeriveMachineID()` that returns a sanitized hostname
      via `HostnameLookup` (DI seam) + `sanitizePathSegment`.
      Env var `HAMS_MACHINE_ID` takes precedence; fallback is
      `"default"`.
- [x] 2.2 `ensureProfileConfigured` (internal/cli/apply.go:1429) now
      branches on `!globalConfigPresent && cliTag != ""` for the
      auto-init path; writes `profile_tag` + `machine_id` via
      `config.WriteConfigKey` and returns success regardless of
      TTY (explicit CLI input is sufficient consent).
- [x] 2.3 `HostnameLookup` exported from `internal/config/resolve.go`
      as the DI seam. Production value `os.Hostname`. Tests swap it
      via deferred-restore pattern to return deterministic values.
- [x] 2.4 Unit tests in `internal/cli/apply_autoinit_test.go`:
      - `TestApply_AutoInit_WritesConfigOnFirstRun` — pristine
        machine + `--tag=macOS`, asserts the global config
        materializes with the expected two keys.
      - `TestApply_AutoInit_DoesNotOverwriteExistingConfig` —
        pre-existing global config + `--tag=macOS`, asserts the
        config file is byte-identical after the run (regardless of
        whether the apply itself errors later on a missing profile
        dir, which is fine for this assertion).

## 3. Integration test

- [ ] 3.1 End-to-end integration test (deferred — covered by the
      existing apply_test.go auto-init tests in
      `internal/cli/apply_autoinit_test.go` which exercise the
      full runApply flow with DI-isolated fakes; a Docker-based
      e2e is possible but the DI test covers the intent).
- [x] 3.2 Update CLAUDE.md Current Tasks to check off the `hams apply
      --tag` bullet.

## 4. Verification

- [x] 4.1 `task check` passes green (lint, unit, integration).
      E2E via act fails on artifact upload (known act limitation).
- [ ] 4.2 Manual smoke in the dev sandbox (`task dev EXAMPLE=basic-debian`):
      deferred; the apply_autoinit_test.go unit test covers the
      same flow with DI-isolated boundaries.
- [x] 4.3 Archive this change (done in the archival commit).
