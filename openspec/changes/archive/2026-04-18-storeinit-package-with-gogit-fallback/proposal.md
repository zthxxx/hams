# Proposal: Extract Store Scaffold Into `internal/storeinit/` with `go-git` Fallback

## Why

`openspec/specs/project-structure/spec.md:686-699` mandates that hams bundle
`go-git` as a compiled-in dependency **and** use it as a fallback when the
system `git` binary is not on `PATH`. The current auto-scaffold implementation
at `internal/cli/scaffold.go:36-41` only shells out to `git init`:

```go
var gitInitExec = func(ctx context.Context, dir string) error {
    cmd := exec.CommandContext(ctx, "git", "init", "--quiet", dir)
    …
    return cmd.Run()
}
```

On a machine with no `git` on `PATH` — the exact "fresh machine" scenario the
SHALL exists to cover — `hams <provider> …` fails at the very first step
instead of auto-scaffolding a store. This is a direct spec violation.

A second (cosmetic) gap: the scaffold code lives inside `internal/cli/`, which
violates the `CLAUDE.md` package-hygiene rule (package boundaries mirror
responsibility; `internal/cli/` is the command surface, not the
store-lifecycle package). Relocating the code also clears the path for a
future `storeinit.CloneFromRepo` helper under the same package.

## What Changes

- Introduce `internal/storeinit/` as a new top-level internal package with:
  - `doc.go` — package-level documentation.
  - `storeinit.go` — `Bootstrap(dir)` / `BootstrapContext(ctx, dir)` /
    `Bootstrapped(dir)` / `SeedIdentityDefaults(paths, cfg)`.
  - `storeinit_test.go` — DI-isolated unit tests (`t.TempDir`, no exec of
    real `git`).
  - `template/` — embedded template files (`gitignore`, `hams.config.yaml`),
    moved from `internal/cli/template/store/`.
- Add a `go-git` fallback in `Bootstrap`: `exec.LookPath("git")` first; if not
  found, call `gogit.PlainInit(dir, false)`.
- Preserve the current branch's behaviour for `seedIfMissing(profile_tag)` +
  `seedIfMissing(machine_id)` so first-run stays non-interactive
  (`cli-architecture/spec.md:654`).
- Rewire `internal/cli/scaffold.go` (and callers in `apply.go`,
  `provider_cmd.go`, `commands.go`) to delegate to the new package.
- Delete the embedded template at `internal/cli/template/store/` once the
  new location is the single source of truth (BREAKING only to any in-tree
  code that referenced the old path directly — no external users).

## Capabilities

### New Capabilities

None — this change relocates existing capability without inventing new
user-visible behaviour.

### Modified Capabilities

- `project-structure` — extends the existing "bundled go-git fallback" SHALL
  with a scenario covering the auto-scaffold code path (not just
  `hams apply --from-repo`). Delta under
  `specs/project-structure/spec.md`.
- `cli-architecture` — documents that the CLI package delegates store
  scaffolding to `internal/storeinit/` (DI-friendly boundary). Delta under
  `specs/cli-architecture/spec.md`.

## Impact

- Affected code:
  - NEW: `internal/storeinit/` package.
  - DELETE: `internal/cli/scaffold.go` (delegate-and-delete; the
    `EnsureStoreScaffolded` entry point moves to `storeinit.EnsureReady`
    or similar).
  - DELETE: `internal/cli/template/store/` (moved to
    `internal/storeinit/template/`).
  - UPDATED: `internal/cli/apply.go`, `internal/cli/provider_cmd.go`,
    `internal/cli/commands.go` call the new package.
  - UPDATED: `internal/cli/scaffold_test.go` → `internal/storeinit/storeinit_test.go`.
- Affected tests:
  - NEW: integration test in a `git`-less container that exercises
    `hams apt install htop` end-to-end.
- Dependencies:
  - `github.com/go-git/go-git/v5` is already in `go.mod` (used by
    `internal/cli/bootstrap.go` for the clone path); no new modules.
- No user-facing CLI flag changes.
- No hamsfile / state schema changes.
