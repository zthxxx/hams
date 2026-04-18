# Design: `internal/storeinit/` Package with `go-git` Fallback

## Context

`project-structure/spec.md:686-699` requires that the hams binary bundle
`go-git` AND use it as a fallback when system `git` is absent. The current
`internal/cli/scaffold.go` only uses `exec.Command("git", ...)` and has no
fallback — a fresh container that ships without `git` aborts at the very
first `hams <provider> ...` invocation. The fix is architectural
(introduce a package seam) + one-shot (add the fallback branch).

## Goals

1. Close the SHALL in `project-structure/spec.md:686-699` with a
   demonstrable DI-isolated unit test plus a Docker integration test.
2. Preserve every behavioural contract the current `scaffold.go` ships —
   dry-run short-circuit, `seedIfMissing(profile_tag)`,
   `seedIfMissing(machine_id)`, `WriteConfigKey(store_path)`, embedded
   templates.
3. Move the code to a package boundary that reflects its responsibility
   (`internal/storeinit/`), leaving `internal/cli/` for the command
   surface.

## Non-Goals

- Redesigning the bootstrap flow (that lives in `internal/cli/bootstrap.go`
  for `--from-repo` and is untouched here).
- Changing the scaffolded files' contents (the `gitignore` and
  `hams.config.yaml` templates are moved byte-for-byte).
- Adding an "update scaffold" feature (idempotent skip-if-present stays).

## Public Surface

```go
// Package storeinit scaffolds a fresh hams store directory:
// create-dir → git init (or go-git fallback) → write embedded templates
// → seed identity defaults.
package storeinit

// Bootstrap is the synchronous entry point used by CLI commands.
func Bootstrap(ctx context.Context, paths config.Paths, flags *provider.GlobalFlags) (storePath string, err error)

// Bootstrapped returns true when dir already looks like a hams-initialized
// store (has .git AND hams.config.yaml). Idempotency helper.
func Bootstrapped(dir string) bool

// DefaultLocation returns where the auto-scaffold lands when no flag or
// config value is supplied: $HAMS_STORE > $HAMS_DATA_HOME/store.
func DefaultLocation(paths config.Paths) string
```

DI seams (package-level variables, rebound in tests):

```go
// ExecGitInit shells out to the system git when available. Test fakes
// simulate the outcome without forking a real process.
var ExecGitInit = defaultExecGitInit

// GoGitInit falls back to go-git when ExecGitInit cannot find git on
// PATH. Test fakes can force-miss the exec path to cover the fallback
// branch.
var GoGitInit = defaultGoGitInit
```

The DI shape mirrors `internal/cli/scaffold.go`'s existing
`gitInitExec` pattern, so tests that rebind `gitInitExec` translate
directly.

## `Bootstrap` Algorithm

```text
1. Resolve storePath:
     if flags.Store != ""                   → flags.Store
     elif cfg.StorePath != ""                → cfg.StorePath
     elif DefaultLocation(paths) → default

2. if storePath exists AND Bootstrapped(storePath)  → persist & return
3. if flags.DryRun → print "[dry-run] Would scaffold store at <path>"
                     and return early

4. MkdirAll(storePath)

5. if no .git in storePath:
     if ExecGitInit(ctx, storePath) → ok
     else if git not on PATH → GoGitInit(ctx, storePath)
     else                    → return the exec error

6. For each embedded template file (gitignore → .gitignore,
   hams.config.yaml → hams.config.yaml):
     if file missing         → write from embed
     if file present         → skip (user edits win)

7. WriteConfigKey(paths, "", "store_path", storePath)   (best-effort)

8. SeedIdentityDefaults:
     seedIfMissing("profile_tag", config.DefaultProfileTag)
     seedIfMissing("machine_id",  config.DeriveMachineID)

9. return storePath
```

## `go-git` Fallback Semantics

The fallback fires only when `exec.LookPath("git")` returns `ErrNotFound`
(or the equivalent PathError with `ENOENT`). Any other failure —
`permission denied`, hook crash, hung global `core.hooksPath` binary —
propagates unchanged so operators can still distinguish "git missing"
from "git misconfigured".

`gogit.PlainInit(dir, false)` produces the same `.git/HEAD` +
`.git/config` layout the CLI would; the subsequent embedded-template
writes work regardless of which path produced the repo.

An `INFO`-level log line (`slog.Info("storeinit: used bundled go-git
fallback", "path", dir)`) fires when the fallback triggers. This is the
observable surface the existing `project-structure/spec.md:693`
scenario asserts on.

## Idempotency Proof

1. `Bootstrap(ctx, paths, flags)` runs when the store does not exist
   → creates everything.
2. Second `Bootstrap(...)` call with the same args → `Bootstrapped()`
   returns `true`, function returns the existing path without
   re-initing git, without overwriting templates, without re-seeding
   non-empty identity keys.
3. If the user hand-edited `.gitignore` between runs, run 2 does NOT
   clobber the edit — the "file missing → write embed" branch is the
   only writer.

A property-based test asserts: for any sequence of `Bootstrap` calls on
the same `t.TempDir()`, the content of `.gitignore` and
`hams.config.yaml` after the last call equals the content after the
first call, unless a call explicitly wrote in between (no writer in
this package). See `storeinit_test.go`.

## Migration Plan

1. Create `internal/storeinit/{doc.go,storeinit.go,storeinit_test.go}`
   with the package doc, implementation, and unit tests.
2. Move `internal/cli/template/store/` → `internal/storeinit/template/`.
3. Replace `internal/cli/scaffold.go`'s body with a single-line
   delegator to `storeinit.Bootstrap` (keeps caller signatures stable
   through the transition) OR delete the file and update the 3 callers
   in-place. The second option is simpler; the first is shorter to
   review.
4. Update `internal/cli/apply.go:runApply` (the "no store configured"
   auto-init branch) and `internal/cli/provider_cmd.go` (first-run
   provider wrapping) to call `storeinit.Bootstrap` directly.
5. Run `task check`; expect only test/package path updates.
6. Ship an integration test under
   `internal/provider/builtin/apt/integration/` (re-uses the apt harness)
   that installs htop in a debian-slim container **with `git` removed
   from `/usr/bin/`** and asserts:
   - the scaffolded store exists at `$HAMS_DATA_HOME/store`;
   - `.git/HEAD` exists;
   - the log stream contains `"used bundled go-git fallback"`.

## Alternatives Considered

**A. Keep code in `internal/cli/` and add the fallback inline.**
Satisfies the SHALL but leaves the package-boundary gap unaddressed.
Rejected because CLAUDE.md → Development Process Principles §2 calls
out package hygiene, and a second scaffold-shaped module
(`CloneFromRepo`) is likely within the next release.

**B. Always use `go-git`, never shell out.**
One code path, but makes hook output invisible to the user (system
`git init` prints any custom hook stdout, go-git does not replay it).
Rejected — preserving exec behaviour when `git` is present matches the
SHALL's "fallback" wording.

**C. Detect missing git via `os.Stat("/usr/bin/git")`.**
Brittle — `git` lives at different paths on different distros,
homebrew on macOS uses `/opt/homebrew/bin/git`, and users may keep it
in `~/.local/bin`. `exec.LookPath` is the portable answer.

**Chosen: A + DI-seams + log breadcrumb** (the current doc describes
this option).

## Test Plan

1. **Unit test** — `storeinit_test.go` with `t.TempDir`:
   - `Bootstrap(happy-path)` creates dir, writes templates, sets
     store_path in global config.
   - `Bootstrap(already-initialized)` is a no-op.
   - `Bootstrap(hand-edited-.gitignore)` does not clobber.
   - `Bootstrap(no-git-on-path)` falls back to go-git when the `ExecGitInit`
     seam is rebound to return `exec.ErrNotFound`.
   - `Bootstrap(dry-run)` does not touch filesystem or shell out.
   - Property-based: any suffix of `[Bootstrap, ...]` calls yields
     identical on-disk content.
2. **Integration test** — debian-slim image with `apt remove git` baked
   in; `hams apt install htop` succeeds and the log line fires.
3. **Regression** — the existing `TestEnsureStoreScaffolded_*` tests
   continue to pass after the move (renamed + path-fixed).
