# Task: Reconcile Spec vs Implementation Naming & Priority

## Findings

Three divergences between `openspec/specs/builtin-providers/spec.md` (the authoritative shipped spec) and the compiled binary:

### 1. Provider Name: `go` (spec) vs `goinstall` (impl)

- Spec (builtin-providers): lists provider as `go` in the priority list and command examples.
- Impl: `internal/provider/builtin/goinstall/goinstall.go` declares `Manifest.Name = "goinstall"` and the CLI routes via `hams goinstall <pkg>`, not `hams go`.

### 2. Provider Name: `vscode-ext` (spec) vs `code-ext` (impl)

- Spec: lists provider as `vscode-ext`.
- Impl: `internal/provider/builtin/vscodeext/vscodeext.go` declares `Manifest.Name = "code-ext"`.

### 3. Priority Order: `bash=1` (spec) vs `bash=14` (impl)

- Spec: provider execution priority starts with `bash` at position 1.
- Impl: `internal/config/config.go:35-38` `DefaultProviderPriority = []string{"brew", "apt", "pnpm", "npm", "uv", "goinstall", "cargo", "code-ext", "mas", "git-config", "git-clone", "defaults", "duti", "bash", "ansible"}` — `bash` is 14th.

## Architect Decision

### For naming (1) and (2): Update the spec, not the code.

Reasoning (user-impact first):

- `hams goinstall <pkg>` matches the verb form `go install` that Go developers invoke — `hams go install foo` would be more natural but would collide with `hams` using `go` as a CLI word elsewhere.
- `code-ext` matches the VS Code Extension file-extension convention `*.code-ext.hams.yaml` already shipped in user stores.
- Renaming in code would:
  - Break every existing user's hamsfile (file name changes from `code-ext.hams.yaml` → `vscode-ext.hams.yaml`).
  - Break every existing state file path (`.state/<machine>/code-ext.state.yaml` → `.state/<machine>/vscode-ext.state.yaml`).
  - Require a migration in `internal/state` with backward-compatible read + forward-write.
  - Invalidate the auto-record logs and any screenshots in `docs/`.
- Cost of updating the spec: a find/replace across ~3 spec files. Zero user impact.

### For priority order (3): Update the spec, not the code — with caveat.

Investigate first:

- `internal/provider/dag.go:47-53` shows Kahn's algorithm with the zero-indegree queue sorted **alphabetically** for determinism (`sort.Strings(queue)`). This means the priority list from `config.go` has **no effect** on execution order for root-level (zero-dependency) providers — they run alphabetically.
- Only providers with `DependsOn` entries get ordered by DAG topology. Everything else is alphabetical.
- In practice: `bash` has no DependsOn; `brew` depends on `bash`. DAG correctly pulls `bash` before `brew`. Alphabetical order then interleaves other root-level providers (apt before bash before cargo before defaults …) which is indistinguishable from priority order for the typical user.

Two sub-questions a follow-up must answer:

1. **Does the priority list serve any purpose today?** If `Ordered()` in `registry.go:82-112` returns providers by priority but `ResolveDAG()` re-sorts zero-indegree nodes alphabetically, is the priority list vestigial?
   - Investigation: verify with a unit test that changes `DefaultProviderPriority` and confirms the execution order is (or is not) affected.
2. **If the priority list is vestigial, should we remove it?** Versus: should `ResolveDAG()` preserve the input order for zero-indegree nodes?
   - If users or downstream code rely on priority override (via `config.go` `provider_priority` YAML key), DAG should honor it. Break it silently and a user's `provider_priority: [bash, brew, ...]` override becomes a no-op.

## Subtasks

- [x] Write a unit test in `internal/provider/dag_test.go` verifying whether priority order is honored for zero-indegree nodes. — `TestResolveDAG_ZeroIndegreePriority` added. **Result: PRIORITY IS NOT HONORED** — zero-indegree nodes are sorted alphabetically by `sort.Strings(queue)` at `internal/provider/dag.go:53`. The input order from `registry.Ordered(priority)` is discarded at this point.
- [ ] Based on the test result, pick one of:
  - (a) Fix `internal/config/config.go` `DefaultProviderPriority` to start with `bash` — **doesn't help**; priority list is consumed by `Ordered()` but then `ResolveDAG` re-sorts. Reordering the list is cosmetic.
  - (b) Change `internal/provider/dag.go:47-53` Kahn's algorithm to preserve input order for zero-indegree queue (higher risk, may reorder other providers in user environments).
  - (c) **RECOMMENDED** — Document the alphabetical behavior in `openspec/specs/provider-system/spec.md` as the contract, and either:
    - Remove `provider_priority` from `config.go` entirely (currently vestigial for root-level providers), OR
    - Keep it documented as affecting only intra-DAG-level ordering of NON-root providers (where dependents become ready together).
- [ ] Update `openspec/specs/builtin-providers/spec.md` to use `goinstall` (not `go`) and `code-ext` (not `vscode-ext`) throughout. Include a brief note explaining the file-path and hamsfile-naming rationale.
- [ ] Grep `openspec/specs/` and `docs/` for stale references to `go` as provider name or `vscode-ext` as provider name and update all.

## Architect Recommendation (added 2026-04-16 after investigation)

Go with option (c). Reasoning:

- Changing DAG's Kahn to honor input order (option b) would silently reorder providers in every user's environment. If any user has relied on alphabetical ordering (even unknowingly), their machine state changes. Low win, high blast-radius.
- Option (a) does nothing because `ResolveDAG` discards input order.
- Option (c) is a documentation fix. The shipped behavior is deterministic and reasonable; only the spec is wrong. Cost: one spec scenario update. User impact: zero.
- The `provider_priority` YAML override in user config files currently only affects the order shown in `registry.Ordered()` output, which matters for list displays and the initial pre-DAG provider enumeration, but not for execution order. That's either (i) acceptable as-is and worth documenting, or (ii) a separate bug to fix in a follow-up where `ResolveDAG` takes an optional priority-list parameter as Kahn tiebreaker.

The follow-up change for this reconciliation is not in scope for `2026-04-16-verification-findings`; this task file now has enough information for that future change to begin.
