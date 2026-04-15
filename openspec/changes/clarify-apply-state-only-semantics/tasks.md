# Tasks

## 1. CLI flag wiring

- [ ] 1.1 Locate the `apply` command registration in `internal/cli/commands.go` (or wherever subcommand `apply` is constructed) and add the `--prune-orphans` boolean flag (default `false`) with help text "Process providers that have a state file but no hamsfile by removing every state-tracked resource. Destructive; default off."
- [ ] 1.2 Plumb the flag value into `runApply` (either via a new parameter or a small `applyOpts` struct so the function signature stays readable).

## 2. runApply logic change

- [ ] 2.1 In `internal/cli/apply.go::runApply`, replace the `if !mainExists && !localExists { continue }` block with a branch that consults `flags.PruneOrphans`:
  - If false, preserve current skip + debug log behavior.
  - If true AND a state file exists for this provider, construct an in-memory empty `*hamsfile.File` (DocumentNode wrapping an empty MappingNode) anchored at the would-be hamsfile path so `Plan` sees an empty desired-state, then fall through to the existing Plan/Execute path. Emit an info log naming the provider and the state-file path being pruned.
  - If true AND no state file exists, skip with a different debug log ("nothing to prune").
- [ ] 2.2 Verify the new branch does NOT bypass the bootstrap-policy logic immediately above (state-only providers still observe the "bootstrap failure with hamsfile present is fatal, without is debug" rule).

## 3. Unit tests

- [ ] 3.1 Add `TestApply_PruneOrphans_RemovesOrphanedStateResources` in `internal/cli/apply_test.go` (or the equivalent existing apply test file): seed a state file with apt resource `htop` in `state=ok`, no hamsfile present, run `runApply` with `PruneOrphans=true`, assert that the apt provider's CmdRunner.Remove was called with `["htop"]` AND state transitions to `state=removed` with `removed_at` populated.
- [ ] 3.2 Add `TestApply_NoPruneOrphans_PreservesOrphanedStateResources`: same fixture, `PruneOrphans=false` (default), assert no Remove call, state file unchanged, debug log emitted.
- [ ] 3.3 Add `TestApply_PruneOrphans_NoStateFile_IsNoOp`: stage-1 normally would not select a provider with neither hamsfile nor state, but defensive — `PruneOrphans=true` with neither present must not panic and must not call any provider method.
- [ ] 3.4 Add `TestApply_PruneOrphans_HamsfilePresent_DoesNotPrune`: provider has both hamsfile (declaring htop) and state (`htop=ok`), `--prune-orphans=true` → htop stays installed (the flag only affects state-only providers).

## 4. Apt integration test extension

- [ ] 4.1 Add a new section `assert_apply_prune_orphans_flow` to `internal/provider/builtin/apt/integration/integration.sh` that:
  1. Runs `hams apt install jq` to seed a state row.
  2. Asserts `apt.state.yaml` has `jq.state=ok`.
  3. Deletes `apt.hams.yaml`.
  4. Runs `hams apply` (no flag) → asserts state file unchanged AND `command -v jq` still succeeds (confirms default skip).
  5. Runs `hams apply --prune-orphans` → asserts `apt.state.yaml.jq.state=removed`, `removed_at` present, AND `command -v jq` fails.
  6. Cleans up so the section is idempotent across CI re-runs.

## 5. Docs sync

- [ ] 5.1 `docs/content/en/docs/cli/index.mdx` — add `--prune-orphans` mention in the `hams apply` subsection.
- [ ] 5.2 `docs/content/zh-CN/docs/cli/index.mdx` — same change in Chinese.
- [ ] 5.3 Search for any other `docs/content/{en,zh-CN}/docs/cli/*` page that documents `hams apply` flags and update if found.

## 6. Verification

- [ ] 6.1 `task fmt` clean.
- [ ] 6.2 `task lint:go` clean (no errcheck / goconst / godot regressions).
- [ ] 6.3 `task test:unit` green with `-race` (incl. the four new apply unit tests).
- [ ] 6.4 `task ci:itest:run PROVIDER=apt` green end-to-end (incl. the new `assert_apply_prune_orphans_flow` section).
- [ ] 6.5 Manual smoke via `task dev EXAMPLE=basic-debian` — install via CLI, delete hamsfile, apply with + without `--prune-orphans`, observe expected behavior (skip without flag, remove with flag).

## 7. Archive

- [ ] 7.1 `/opsx:verify clarify-apply-state-only-semantics` — assert spec scenarios map to code, no critical/warning findings.
- [ ] 7.2 `/opsx:archive clarify-apply-state-only-semantics` — sync spec deltas (will likely require `--skip-specs` + manual application due to the auto-sync header bug noted in the prior cycle).
