# Tasks: `hams git` passthrough

- [x] **Passthrough helper**
  - [x] `passthroughExec` package-level var (DI seam) added.
  - [x] `passthrough(ctx, args, flags)` method on `*UnifiedHandler` runs real git with stdio preserved.
  - [x] `flags.DryRun` short-circuits to `[dry-run] Would run: git <args>` on `flags.Stdout()`.
  - [x] `HandleCommand`'s `default:` branch invokes `passthrough(...)` instead of returning UFE.

- [x] **Clone translation**
  - [x] `handleClone` accepts `hams git clone <url> <path>` natural form, folding positional path into `hamsFlags["path"]`.
  - [x] Legacy `--hams-path=<path>` form preserved.
  - [x] `hams git clone remove <urn>` / `hams git clone list` forward unchanged to the CloneProvider's CLI handler.
  - [x] Unforwarded git flags rejected with UFE (user told to file a follow-up or run plain git).

- [x] **Tests**
  - [x] Passthrough via exec seam (normal + error propagation + dry-run).
  - [x] Natural clone form folds path.
  - [x] Unknown git flag rejected.
  - [x] Bare `hams git clone` emits usage.

- [x] **Spec deltas**
  - [x] `specs/provider-system/spec.md` — new "Passthrough for Unhandled Subcommands" requirement.
  - [x] `specs/builtin-providers/spec.md` — `git` clone translation + `--hams-path` back-compat explicit.

- [x] **Verification**
  - [x] `go test ./internal/provider/builtin/git/...` PASS.
  - [x] `task fmt && task lint && task test:unit` green.
