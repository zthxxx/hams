# Tasks: Fix v1 Planning Gaps

## §1: `--hams-lucky` Flag Wiring

- [x] 1.1 Pass `hamsFlags["lucky"]` from `HandleCommand` through to the enrichment/tag-picker call site
- [x] 1.2 In `RunTagPicker()` (internal/tui/run.go), accept a `lucky bool` parameter; when true, skip TUI and return LLM tags directly
- [x] 1.3 In `EnrichAsync()` flow, propagate lucky flag so intro is also auto-accepted
- [x] 1.4 Add unit test: verify that when lucky=true, RunTagPicker returns LLM tags without prompting
- [x] 1.5 Add unit test: verify that hamsFlags["lucky"] is correctly propagated from CLI to provider HandleCommand

## §2: Homebrew Tap Separation

- [x] 2.1 Add "tap" as a recognized classification tag in Homebrew provider (alongside "cask" and formula)
- [x] 2.2 When `hams brew install org/repo/formula`, auto-detect tap format and classify under "tap" group in Hamsfile
- [x] 2.3 During Probe, enumerate installed taps via `brew tap` and reconcile with tap entries in Hamsfile
- [x] 2.4 During Apply, handle tap resources: `brew tap <repo>` for install, `brew untap <repo>` for remove
- [x] 2.5 Add property-based test: tap resource round-trip (Hamsfile write → read → probe → plan)
- [x] 2.6 Update Homebrew provider spec delta for tap classification

## §3: Provider `list` Showing Diff

- [x] 3.1 Define a `DiffResult` type in internal/provider that captures: desired-only, state-only, matched, and status-diverged resources
- [x] 3.2 Implement `DiffDesiredVsState()` function that compares Hamsfile resources against state resources
- [x] 3.3 Wire `DiffDesiredVsState` into the `List()` method of all providers (replace TODO in Homebrew)
- [x] 3.4 Format diff output: show additions (+), removals (-), status mismatches (~) with colors
- [x] 3.5 Support `--json` output mode for diff results
- [x] 3.6 Add property-based test: diff computation correctness for various resource sets

## §4: Update Hooks Invocation

- [x] 4.1 Implement `RunPreUpdateHooks()` and `RunPostUpdateHooks()` in internal/provider/hooks.go (parallel to existing install hook functions)
- [x] 4.2 In `executeUpdate()` (executor.go), call `RunPreUpdateHooks` before and `RunPostUpdateHooks` after the update action
- [x] 4.3 Handle pre-update hook failure: skip update, mark resource as failed
- [x] 4.4 Handle post-update hook failure: mark resource as hook-failed (update succeeded, hook didn't)
- [x] 4.5 Collect deferred update hooks and run them after all provider resources are processed
- [x] 4.6 Add property-based test: update hooks fire in correct order, failure propagation works correctly

## §5: Sensitive Config Key Detection

- [x] 5.1 Expand `sensitiveKeys` map in internal/config/config.go to include patterns: keys containing "token", "key", "secret", "password", "credential"
- [x] 5.2 Change `IsSensitiveKey()` to use substring matching in addition to exact match, so future keys are automatically caught
- [x] 5.3 When `hams config set` detects a sensitive key, log a message explaining it was routed to `.local.yaml`
- [x] 5.4 Add property-based test: various key names correctly classified as sensitive or non-sensitive

## §6: `preview-cmd` Field for KV Config Providers

- [x] 6.1 Add `PreviewCmd` field to the hamsfile resource/item schema in internal/hamsfile
- [x] 6.2 In defaults provider: populate `preview-cmd` with the original `defaults write` command string when recording
- [x] 6.3 In git-config provider: populate `preview-cmd` with the original `git config` command string when recording
- [x] 6.4 In duti provider: populate `preview-cmd` with the original `duti` command string when recording
- [x] 6.5 Ensure `preview-cmd` is preserved during YAML round-trips (comment preservation)
- [x] 6.6 Add property-based test: preview-cmd field survives YAML read/write cycle

## §7: Spec Deltas

- [x] 7.1 Write spec delta for provider-system: update hooks invocation requirement
- [x] 7.2 Write spec delta for cli-architecture: --hams-lucky flag behavior
- [x] 7.3 Write spec delta for schema-design: preview-cmd field
- [x] 7.4 Write spec delta for builtin-providers: Homebrew tap classification, provider list diff
