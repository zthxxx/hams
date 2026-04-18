# Proposal: auto-init UX hardening

## Why

Three first-run-UX gaps exist in dev's auto-init path that are already fixed on the reference branch `origin/local/loop` (see `/tmp/hams-loop/internal/cli/scaffold.go`). Left unfixed they each break the "one command restores the whole environment" onboarding promise in CLAUDE.md:

1. **Dry-run is lying.** `hams --dry-run brew install htop` on a pristine host currently still materializes `~/.local/share/hams/store/` (directory, `git init`, template files) and writes `~/.config/hams/hams.config.yaml`. The documented guarantee of `--dry-run` ("No changes will be made") is broken for any user whose first invocation is a provider wrap — the most common first-run path.

2. **`git init` can wedge first-run.** `initGitRepo` calls `exec.CommandContext(ctx, gitBin, "init", …)` with no timeout. On corporate laptops where a global `init.templateDir` hook does a blocking lookup (intranet LDAP, certificate pinning), the hams binary hangs forever at its very first command. The go-git fallback is fast, but only runs when `git` is missing from PATH; the user's machine always has `git` here, so the slow path is hit every time.

3. **Post-scaffold identity is empty.** After auto-init, every subsequent `hams <provider> …` call that reads `cfg.ProfileTag` / `cfg.MachineID` sees the runtime fallbacks (`"default"` / `"unknown"`) but the persisted config is silent. Users staring at a freshly-generated config can't tell whether the silence is intentional — the config looks half-broken. Worse, the `ensureProfileConfigured` path (for apply) re-prompts for the same values every time, because the global config never learned what apply already had to derive.

## What changes

1. **`internal/storeinit/storeinit.go::initGitRepo`** — wrap the `exec.CommandContext` invocation with `context.WithTimeout(ctx, 30*time.Second)` so a hung global git hook cannot wedge first-time setup. The go-git fallback is in-process and already fast, so no timeout is applied there.

2. **`internal/cli/autoinit.go::EnsureStoreReady`** + **`EnsureGlobalConfig`** — extend both helpers to accept a `flags *provider.GlobalFlags` parameter and short-circuit on `flags.DryRun`. When DryRun is set the helpers MUST NOT:
   - call `storeinit.Bootstrap` (no directory creation, no `git init`, no template materialization).
   - write the global config file.
   - call `config.WriteConfigKey` to persist `store_path` / `profile_tag` / `machine_id`.
   Instead they print a single `[dry-run] Would ...` preview line to `flags.Stderr()` and return the target path (so downstream logic can still reason about "where the store would land").

3. **`internal/cli/autoinit.go::seedIdentityIfMissing`** (new helper) — after a real (non-dry-run) `storeinit.Bootstrap`, writes `profile_tag` and `machine_id` to the global config IFF those keys are currently empty. Uses `config.DefaultProfileTag` + `config.DeriveMachineID()` which already exist in `internal/config/resolve.go`. Respects user-set values: if the user pre-seeded `profile_tag: macOS`, the scaffolder does NOT overwrite it.

4. **All call-sites of `EnsureStoreReady` + `EnsureGlobalConfig`** — update to pass `flags`:
   - `internal/cli/apply.go` (the `storePath == ""` branch that fires auto-init)
   - `internal/cli/commands.go` (refresh's mirror of the same branch)
   - `internal/cli/provider_cmd.go::autoInitForProvider`

## Impact

- **Capability `cli-architecture` / section "Auto-init"** — adds three new SHALL requirements for dry-run short-circuit, git-init timeout, and identity seeding.
- **Developer experience** — `hams --dry-run <anything>` stays honestly side-effect-free. First-run on a corporate laptop cannot wedge at `git init`. Post-scaffold config is self-describing.
- **Backward compatibility** — additive. The `flags` parameter is a new arg; internal package, no exported-API break. Existing tests that call `EnsureStoreReady(paths, cfg, "")` get a `nil` flags path (no dry-run, no change in behavior).
- **No user-visible product change** for the non-dry-run, non-corporate-hook common path.

## Out of scope

- Progress UX for the timeout case (no spinner, no "still waiting on git init" message). If 30s elapses the error bubbles up; future UX iteration can layer a BubbleTea spinner on top.
- Changing which keys `seedIdentityIfMissing` seeds. Only `profile_tag` + `machine_id` are seeded today; `store_path` is already persisted by the existing `EnsureStoreReady` flow.
