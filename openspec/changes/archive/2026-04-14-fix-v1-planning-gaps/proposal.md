# Proposal: Fix v1 Planning Gaps

## Why

A detailed review of the original planning document against the current implementation revealed several features that were explicitly specified in the user's design but were not fully implemented or wired up. These gaps affect real user workflows — from the `--hams-lucky` auto-accept flag to update hooks and provider list diffs.

## What Changes

This change addresses 6 gaps between the planning document and the current v1 implementation:

1. **`--hams-lucky` flag not wired to TUI** — The flag is parsed but never forwarded to the tag picker. When present, the TUI should be skipped entirely and LLM recommendations auto-accepted.

2. **Homebrew tap not separately classified** — Cask/formula separation exists, but taps are not tracked as a distinct classification in the Hamsfile. The planning doc requires "tap 也完全分开记录".

3. **Provider `list` not showing Hamsfile vs state diff** — The Homebrew provider's `List()` method has a TODO comment but only displays state contents without diffing against the desired Hamsfile. All providers should show this diff.

4. **Pre-update / post-update hooks never invoked** — `HookPreUpdate` and `HookPostUpdate` types are defined in `hooks.go` but `executeUpdate()` in `executor.go` never calls them. The planning doc explicitly requires update lifecycle hooks.

5. **Sensitive config key detection too narrow** — Only `llm_cli` is marked sensitive. Keys containing token/key/secret/password patterns should auto-route to `.local.yaml`.

6. **`preview-cmd` field missing for KV config providers** — The planning doc specifies that defaults/duti/git-config providers should store a `preview-cmd` field alongside `args` and `check` in the Hamsfile schema for human-readable review.

## What Does NOT Change

- kong parser integration (providers work with manual parsing; would be a v2 refactor)
- Discord notification channel (optional, not v1 priority)
- git state backend (explicitly described as optional/future)

## Impact

- Modified packages: `internal/cli`, `internal/tui`, `internal/provider` (hooks, executor), `internal/provider/builtin/homebrew`, `internal/config`, `internal/hamsfile`
- No new external dependencies
- All existing tests must continue to pass
- New property-based tests for each gap fix
