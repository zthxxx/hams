# Tasks: `--tag` / `--profile` conflict detection

- [x] **GlobalFlags extended**
  - [x] `Tag string` field added alongside existing `Profile string`.
  - [x] `Out io.Writer` + `Err io.Writer` fields added as DI seams.
  - [x] `Stdout()` + `Stderr()` accessor methods added.
  - [x] `EffectiveTag()` convenience method added.

- [x] **config.resolve.go**
  - [x] `DefaultProfileTag = "default"` constant exported.
  - [x] `ResolveCLITagOverride(cliTag, cliProfile) (string, error)` — conflict → UFE.
  - [x] `ResolveActiveTag(cfg, cliTag, cliProfile) (string, error)` — full precedence.
  - [x] `HostnameLookup` var + `DeriveMachineID` helper.

- [x] **i18n keys**
  - [x] `cli.err.tag-profile-conflict` added to `en.yaml`.
  - [x] Chinese translation added to `zh-CN.yaml`.

- [x] **CLI flag wiring**
  - [x] `globalFlagDefs` registers `--tag` and `--profile` as separate `StringFlag`s.
  - [x] `globalFlags` fills both `Tag` and `Profile` on GlobalFlags.

- [x] **Call-site adoption**
  - [x] `internal/cli/apply.go` — conflict resolution at top of runApply; downstream consumers read `cliTagOverride`.
  - [x] `internal/cli/commands.go` — refresh, list, config-list, store-* actions call `ResolveCLITagOverride` before `config.Load`.
  - [x] `internal/cli/provider_cmd.go` — `parseProviderArgs` and `stripGlobalFlags` route `--tag` into `flags.Tag`; `autoInitForProvider` uses `flags.EffectiveTag()`.
  - [x] `internal/cli/register.go` — `loadBuiltinProviderConfig` uses `flags.EffectiveTag()`.

- [x] **Tests**
  - [x] `internal/config/resolve_test.go` — property-based precedence + deterministic conflict assertion.
  - [x] `DeriveMachineID` tests (env wins, hostname fallback, error fallback).

- [x] **Verification**
  - [x] `go build ./...` — 0 errors.
  - [x] `go test ./internal/config/...` — PASS (resolver tests new).
  - [x] `go test ./internal/cli/...` — PASS.
  - [x] `task fmt && task lint && task test:unit` green.
