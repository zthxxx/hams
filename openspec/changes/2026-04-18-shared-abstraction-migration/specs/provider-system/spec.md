# Spec delta: provider-system — shared helpers + passthrough extended

## ADDED Requirement: CLI-wrapping providers SHALL use shared baseprovider + package_dispatcher helpers

Every builtin provider that wraps a CLI tool for auto-record (`hams cargo install`, `hams npm install`, `hams pnpm add`, `hams goinstall install`, `hams uv install`, `hams mas install`, `hams code install`, `hams brew install`, `hams apt install`, …) SHALL draw the boilerplate "resolve hamsfile path for the active profile, load-or-create the file, resolve effective config from per-invocation flags" from a single shared package, `internal/provider/baseprovider/`, rather than reimplementing the same 58-LOC helper trio (`loadOrCreateHamsfile` / `hamsfilePath` / `effectiveConfig`) per provider.

The `baseprovider` package exposes three call-sites:

- `LoadOrCreateHamsfile(cfg, filePrefix, hamsFlags, flags) (*hamsfile.File, error)` — returns the parsed hamsfile for the active profile, creating an empty document when missing.
- `HamsfilePath(cfg, filePrefix, hamsFlags, flags) (string, error)` — returns the absolute path honoring `--hams-local` → `.hams.local.yaml` and the `--store` / `--tag` / `--profile` overrides.
- `EffectiveConfig(cfg, flags) *config.Config` — returns a copy of cfg with per-invocation `flags.Store` / `flags.Profile` overlays applied, safe for the caller to mutate further.

Providers that auto-record via the canonical "lock → exec → append-hamsfile → save-state" sequence SHALL use the shared `provider.AutoRecordInstall` / `provider.AutoRecordRemove` helpers in `internal/provider/package_dispatcher.go` where their handler is a pure "lock → runner.Install → append → save" recipe. Providers with custom extractors (apt's `parseAptInstallToken`, brew's `--cask` tag routing, etc.) MAY keep their bespoke handleInstall flow — the shared helpers are a replacement for the boilerplate, not a mandate.

#### Scenario: a new package provider is added

- **Given** a new package provider (e.g., `pipx`, `krew`, `scoop`) needs to be added to hams
- **When** the author writes the provider
- **Then** the provider's hamsfile-path and effective-config helpers are ONE file containing only the `tagCLI` constant (~4 LOC). The `handleInstall` / `handleRemove` logic calls `baseprovider.LoadOrCreateHamsfile` and `baseprovider.EffectiveConfig` for path resolution. Where the install/remove flow is "lock → runner.Install → append → save" with no custom extraction, it calls `provider.AutoRecordInstall` / `provider.AutoRecordRemove`; otherwise the provider keeps its own bespoke flow and only uses the `baseprovider` path helpers.

#### Scenario: `AutoRecordInstall` is invoked with zero packages

- **Given** a caller invokes `AutoRecordInstall(ctx, runner, nil, cfg, flags, hfPath, statePath, opts)`
- **When** the helper runs
- **Then** it returns a `UserFacingError` with `ExitUsageError` and message `"<cliName> <installVerb> requires at least one package name"` — without calling runner.Install, without acquiring the lock, without touching the hamsfile or state file.

#### Scenario: `AutoRecordInstall` is invoked with `flags.DryRun=true`

- **Given** a caller invokes `AutoRecordInstall` with `flags.DryRun=true`
- **When** the helper runs
- **Then** it prints `[dry-run] Would install: <cliName> <installVerb> [pkg1 pkg2 …]` to `flags.Stdout()` and returns nil. No runner call, no lock acquisition, no hamsfile/state write.

#### Scenario: `AutoRecordInstall` — runner.Install fails mid-batch

- **Given** the runner rejects the second package in a 3-package install batch
- **When** the helper runs
- **Then** it returns the runner's error immediately. The hamsfile and state file are NOT touched (no partial record). The first package's install already ran on the host but the atomic-record contract means the user must rerun `hams <provider> install <pkg1>` to get it recorded.

## MODIFIED Requirement: CLI-wrapping providers SHALL passthrough unhandled subcommands via `provider.Passthrough`

Every builtin provider that wraps a CLI tool SHALL, for any subcommand it does not explicitly intercept, invoke the wrapped tool transparently via the shared `provider.Passthrough(ctx, tool, args, flags)` helper — NOT via the older `provider.WrapExecPassthrough` which does NOT honor `flags.DryRun`.

`Passthrough` preserves stdin, stdout, stderr, and exit code. When `flags.DryRun` is set, it prints `[dry-run] Would run: <tool> <args>` to `flags.Stdout()` and returns nil without exec. The package-level `provider.PassthroughExec` DI seam is shared across every CLI-wrapping provider so unit tests can assert passthrough invocations without spawning real processes.

This requirement extends the 2026-04-18 `git` passthrough requirement (which was git-specific) to every CLI-wrapping provider. The previous requirement named `provider.WrapExecPassthrough` as the default branch; that helper remains available but does NOT honor DryRun and SHOULD NOT be used in new code.

#### Scenario: user runs `hams --dry-run cargo search ripgrep`

- **Given** `flags.DryRun == true` and the cargo provider's `HandleCommand` receives `["search", "ripgrep"]`
- **When** `search` matches no intercepted verb (`install`, `i`, `remove`, `uninstall`, `rm`, `list`)
- **Then** the default branch calls `provider.Passthrough(ctx, "cargo", ["search", "ripgrep"], flags)`, which prints `[dry-run] Would run: cargo search ripgrep` to `flags.Stdout()` and returns nil. Real `cargo` is NOT invoked.

#### Scenario: user runs `hams cargo search ripgrep` (no dry-run)

- **Given** `flags.DryRun == false`
- **When** the default branch calls `provider.Passthrough`
- **Then** the package-level `PassthroughExec` seam runs `cargo search ripgrep` with stdin/stdout/stderr preserved and propagates the exit code to the hams process.

#### Scenario: a provider is migrated from `WrapExecPassthrough` to `Passthrough`

- **Given** a provider previously called `provider.WrapExecPassthrough(ctx, tool, args, nil)` in its HandleCommand default branch
- **When** the migration replaces that call with `provider.Passthrough(ctx, tool, args, flags)`
- **Then** the provider's users observe identical behavior for non-DryRun invocations AND additionally see the DryRun preview for `hams --dry-run <provider> <unintercepted-verb>`.
