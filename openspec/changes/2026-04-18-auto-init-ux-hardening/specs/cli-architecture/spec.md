# Spec delta: cli-architecture — Auto-init UX hardening

## ADDED Requirement: Dry-run short-circuits auto-init side effects

When a hams invocation runs with `--dry-run` set AND the auto-init path would otherwise fire (no `--store`, no configured `store_path`, `HAMS_NO_AUTO_INIT` not set), the auto-init helpers SHALL NOT mutate the filesystem. Specifically:

- `EnsureGlobalConfig` SHALL NOT write `~/.config/hams/hams.config.yaml`.
- `EnsureStoreReady` SHALL NOT call `storeinit.Bootstrap` (no directory creation, no `git init`, no template materialization).
- `EnsureStoreReady` SHALL NOT call `config.WriteConfigKey` to persist `store_path`, `profile_tag`, or `machine_id`.

Instead, each helper SHALL print a single `[dry-run] Would ...` preview line naming the target path to `flags.Stderr()`, and return the resolved target path so downstream logic can reason about "where the store would land".

#### Scenario: dry-run provider wrap on a pristine host

- **Given** a machine with no `~/.config/hams/hams.config.yaml` and no `~/.local/share/hams/store/`
- **When** the user runs `hams --dry-run brew install htop`
- **Then** both `~/.config/hams/hams.config.yaml` and `~/.local/share/hams/store/` SHALL remain absent on disk
- **And** stderr SHALL contain two `[dry-run] Would ...` preview lines (one for the global config, one for the store)
- **And** the command SHALL NOT fail with a missing-store error — the in-memory flags.Store reflects the preview target so dispatch still proceeds

#### Scenario: dry-run apply on a pristine host

- **Given** a machine with no configured store
- **When** the user runs `hams --dry-run apply`
- **Then** the auto-init path MUST NOT materialize any file
- **And** the existing `[dry-run] Would apply configurations. No changes will be made.` header remains the only stdout output (when `--json` is not set)

## ADDED Requirement: `git init` SHALL time out after 30 seconds

The `git init` step inside `storeinit.Bootstrap` SHALL run under a `context.WithTimeout(ctx, 30*time.Second)`. If the `git` binary (resolved via `exec.LookPath("git")`) takes longer than 30 seconds to return, the bootstrap SHALL fail with a context-deadline-exceeded error rather than block indefinitely.

The go-git (`github.com/go-git/go-git/v5`) fallback path, which runs in-process when `git` is not on PATH, is NOT subject to this timeout — it cannot wedge on a global git hook because the hook chain is only triggered by the `git` binary.

#### Scenario: slow git init hook on a corporate laptop

- **Given** a machine where a global `init.templateDir` hook does a blocking network lookup
- **When** the user runs `hams brew install htop` for the first time
- **Then** the auto-init path SHALL fail after ~30 seconds with a wrapped `context deadline exceeded` error
- **And** the user's terminal SHALL return control — no indefinite hang

#### Scenario: go-git fallback is unaffected

- **Given** a machine with no `git` binary on PATH
- **When** auto-init fires
- **Then** `storeinit.Bootstrap` falls through to `gogit.PlainInit` without applying the 30s timeout

## ADDED Requirement: Scaffold seeds `profile_tag` and `machine_id` when empty

After `EnsureStoreReady` successfully completes a real (non-dry-run) `storeinit.Bootstrap`, it SHALL inspect the global config and, for each of `profile_tag` / `machine_id` that is currently empty or unset, SHALL write the canonical default (`config.DefaultProfileTag` / `config.DeriveMachineID()`) via `config.WriteConfigKey`.

If the user has already set one or both keys (e.g. ran `hams config set profile_tag macOS` before their first provider install), the helper SHALL NOT overwrite the user's value. The check is "empty, therefore seed" — never "nonempty, therefore reseed".

Failures during the seed write are logged (`slog.Warn`) but not propagated; the surrounding auto-init path is best-effort and the store is already usable even when an optional identity key cannot be persisted.

#### Scenario: fresh scaffold populates identity keys

- **Given** a machine with no `~/.config/hams/hams.config.yaml`
- **When** the user runs `hams brew install htop` for the first time
- **Then** after the scaffolder returns, `~/.config/hams/hams.config.yaml` SHALL contain `profile_tag: default` (or the hostname-derived value) AND a `machine_id:` line
- **And** subsequent `hams <provider> …` invocations SHALL NOT re-prompt for these values

#### Scenario: user pre-seeded profile_tag is preserved

- **Given** a user who has pre-created `~/.config/hams/hams.config.yaml` with `profile_tag: macOS`
- **When** the user runs `hams brew install htop` for the first time (no store yet)
- **Then** the scaffolded store is created
- **And** after scaffolding, the global config's `profile_tag` SHALL remain `macOS` (not be overwritten by `default`)
