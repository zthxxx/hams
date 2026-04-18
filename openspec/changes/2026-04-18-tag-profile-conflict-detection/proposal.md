# Proposal: `--tag` / `--profile` conflict detection

## Why

`--tag` is the canonical flag (2026-04-17 rename; `--profile` is the legacy alias). The current implementation registers `--profile` as a urfave/cli *alias* of `--tag`, so `--tag=macOS --profile=linux` silently picks whichever value was written last into the shared underlying field — no error, no warning, no audit trail. A user migrating a CI script who forgets to remove the old `--profile=linux` line loses silently.

Spec implication: `openspec/specs/schema-design/spec.md` — tag/profile is a `path segment` resolved into `<store>/<tag>/` on disk. Picking the wrong one mis-identifies the profile directory. A silent wrong pick on a fresh machine leads to "profile 'linux' not found" *even though the user typed `--tag=macOS`* — a confusing failure mode.

`origin/local/loop` already solves this via a dedicated `config.ResolveCLITagOverride` helper + a separate `Tag` field on `GlobalFlags`; this proposal ports that.

## What changes

1. `internal/provider/flags.go` — new `Tag string` field on `GlobalFlags`, separate from the existing `Profile string`. Also adds `Out io.Writer` / `Err io.Writer` fields + `Stdout()` / `Stderr()` accessor methods as DI seams for dry-run printing (used by the next tasks in the queue).
2. `internal/config/resolve.go` — new file with:
   - `DefaultProfileTag = "default"` constant (exported so scaffolders seed identical values).
   - `ResolveCLITagOverride(cliTag, cliProfile string) (string, error)` — fails loud when both are non-empty and disagree, else returns whichever is non-empty.
   - `ResolveActiveTag(cfg *Config, cliTag, cliProfile string) (string, error)` — composes ResolveCLITagOverride with config precedence.
   - `HostnameLookup` DI seam + `DeriveMachineID()` helper for the auto-init path (used by the next task).
3. `internal/i18n/locales/{en,zh-CN}.yaml` — `cli.err.tag-profile-conflict` message key (translated).
4. `internal/cli/root.go` — `globalFlagDefs` registers `--tag` and `--profile` as TWO separate `StringFlag`s (not aliases). `globalFlags` fills both `Tag` and `Profile` on the returned struct.
5. `internal/cli/{apply,commands,provider_cmd,register}.go` — every call-site that previously read `flags.Profile` now:
   - Calls `config.ResolveCLITagOverride(flags.Tag, flags.Profile)` at the top of the action and fails fast on conflict, OR
   - Calls the new `flags.EffectiveTag()` convenience shim (only where conflict has already been checked or doesn't apply).
6. `internal/config/resolve_test.go` — property-based tests (using `pgregory.net/rapid`) for the resolver precedence + a deterministic assertion that the conflict branch emits a `UserFacingError` with `ExitUsageError`. Deterministic tests for `DeriveMachineID` (env, hostname, error fallback).

## Impact

- **Capability `cli-architecture` / "global flags"** — `--tag` and `--profile` are siblings, not aliases. Conflict detection is spec-required.
- **Capability `schema-design`** — a typed `DefaultProfileTag` constant is the single source of truth for the "default" fallback, replacing the legacy inline literal in `sanitizePathSegment`.
- **User-visible:**
  - `hams apply --tag=macOS --profile=linux` → fails with a clear usage error.
  - `hams apply --tag=macOS` → works identically to before.
  - `hams apply --profile=macOS` → works identically to before (legacy alias still honored, just as a separate field now).
- **Code-side:** `GlobalFlags.Profile` stays for call-sites that don't need conflict detection. New helper `EffectiveTag()` handles the common case.
- **No migration needed** — existing scripts that set only one of the two flags see no behavior change.
