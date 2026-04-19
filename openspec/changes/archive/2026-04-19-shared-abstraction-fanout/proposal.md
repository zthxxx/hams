# Shared-abstraction fanout

## Why

The §9.1/§9.2 gap analysis at `docs/notes/dev-vs-loop-implementation-analysis.md`
identified three provider-layer infrastructure improvements that
`origin/dev` shipped and `local/loop` lacks:

1. A dedicated **`Passthrough`** primitive (vs `WrapExecPassthrough`
   that ignores `flags.DryRun` and has no DI seam).
2. A shared **`baseprovider`** helper package that centralises the
   ~30 lines of inlined `EffectiveConfig` / `HamsfilePath` /
   `LoadOrCreateHamsfile` boilerplate currently duplicated across
   ≈9 builtin providers.
3. **Finer-grained DI seams** in `internal/storeinit` so unit tests
   can simulate "PATH lookup fails" without replacing the entire
   exec pipeline.

These are pure infrastructure refactors. No user-visible CLI behaviour
changes; the Homebrew dispatcher closure variants and apt go-git E0
integration test are preserved unchanged.

## What changes

- Introduce `internal/provider/passthrough.go` with package-level
  `PassthroughExec` seam and a `Passthrough(ctx, tool, args, flags)`
  function that honours `flags.DryRun`. Migrate every existing
  `WrapExecPassthrough` call site to it. Delete `WrapExecPassthrough`
  (no compat shim — project rule on backwards-compat hacks).
- Introduce `internal/provider/baseprovider/` with `EffectiveConfig`,
  `HamsfilePath`, `LoadOrCreateHamsfile`. Migrate all package-class
  builtin providers to use it.
- Replace `internal/storeinit`'s coarse `ExecGitInit` /
  `GoGitInit` function-vars with three fine-grained seams:
  `LookPathGit`, `ExecCommandContext`, `GitInitTimeout`. Update
  the existing `storeinit_test.go` to use the new seams. **The apt
  E0 integration test is unchanged** — it still hides
  `/usr/bin/git` and exercises the real fallback path inside the
  container.

## Impact

- **Affected specs:** `provider-system`, `cli-architecture`.
- **Affected code:** `internal/provider/`, `internal/provider/builtin/*`,
  `internal/storeinit/`.
- **No user-visible change.** Same CLI surface, same exit codes, same
  output (modulo identical i18n messages).
- **Test surface:** new `passthrough_test.go`, new
  `baseprovider/baseprovider_test.go`, updated `storeinit_test.go`.
- **Rollback:** any single commit is revertable in isolation; the
  three sub-changes (passthrough, baseprovider, storeinit) commit
  independently.
