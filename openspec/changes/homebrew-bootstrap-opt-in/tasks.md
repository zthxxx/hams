# Tasks

## 1. Provider framework: bootstrap primitive

- [ ] 1.1 Add `internal/provider/bootstrap.go` with `BashScriptRunner` interface (`RunScript(ctx, script string) error`) + `RunBootstrap(ctx, p, registry) error` that iterates `DependsOn`, platform-filters, and delegates non-empty `.Script` to a registered `BashScriptRunner`.
- [ ] 1.2 Add `WithBootstrapAllowed(ctx, bool)` + `BootstrapAllowed(ctx) bool` ctx helpers in `internal/provider/bootstrap.go`.
- [ ] 1.3 Add `ErrBootstrapRequired = errors.New(...)` sentinel in the same file.
- [ ] 1.4 Add `Registry` interface (if not already present in `internal/provider/registry.go`) with `Lookup(name string) (Provider, bool)` — or reuse the existing registry type. Inspect before writing.
- [ ] 1.5 Make the Bash builtin provider (`internal/provider/builtin/bash/bash.go`) implement `provider.BashScriptRunner` by calling its existing `CmdRunner.Run`.
- [ ] 1.6 Unit tests `internal/provider/bootstrap_test.go`:
  - Delegation happy path: fake `BashScriptRunner` records the script argument.
  - Platform gating: darwin-only entry skipped on linux, returns nil.
  - Missing host provider: error mentions the missing provider name.
  - Script failure: error wraps the runner's error.
  - Multiple entries: all executed in declaration order.
  - Empty-`Script` entries: skipped silently (DAG-only deps).

## 2. Homebrew provider: consent-aware Bootstrap

- [ ] 2.1 Rewrite `internal/provider/builtin/homebrew/homebrew.go` `Bootstrap(ctx)`:
  - If `exec.LookPath("brew") == nil` → return nil.
  - If `provider.BootstrapAllowed(ctx)` → delegate to `provider.RunBootstrap(ctx, p, p.registry)` and re-check LookPath; return nil on success or the (re-)checked error.
  - Otherwise → return `&UserFacingError{Summary, Detail=manifest script, Remedy, Err: provider.ErrBootstrapRequired}`.
- [ ] 2.2 Inject the provider registry into `*homebrew.Provider` (constructor change) so it can call `RunBootstrap`. Confirm the existing fx wiring in `cmd/hams` / `internal/provider/registry.go`.
- [ ] 2.3 Unit tests `internal/provider/builtin/homebrew/bootstrap_test.go`:
  - `brew` present → Bootstrap returns nil without touching ctx.
  - `brew` missing + no consent → UserFacingError wraps ErrBootstrapRequired; script text present in Detail.
  - `brew` missing + consent → RunBootstrap invoked; on success, lookup retried.
  - Bootstrap script succeeds but `brew` still missing → terminal error.
  - Stub `exec.LookPath` via a DI seam (or inject a `binaryChecker` function).

## 3. Apply CLI: flag wiring + TTY prompt

- [ ] 3.1 Add `--bootstrap` and `--no-bootstrap` flags on `hams apply` (and `hams refresh` for symmetry) in `internal/cli/apply.go` (or its command-definition file). Mutually exclusive; both-set = usage error exit 2.
- [ ] 3.2 Before `p.Bootstrap(ctx)`, set `ctx = provider.WithBootstrapAllowed(ctx, *flagBootstrap)`.
- [ ] 3.3 On `Bootstrap` returning `ErrBootstrapRequired`:
  - If `*flagNoBootstrap` OR stdin is not a TTY → surface the UserFacingError and exit.
  - Else → render the interactive prompt (script text + side-effect summary + Xcode-CLT warning + `[y/N/s]`).
  - On `y`: re-wrap ctx with consent=true, retry `p.Bootstrap(ctx)`, continue.
  - On `N`/EOF: surface UserFacingError, exit.
  - On `s`: add provider to the runtime skip list and continue.
- [ ] 3.4 TTY detection: use `x/term.IsTerminal(int(os.Stdin.Fd()))` (preferred) or `golang.org/x/term`. Inject via an interface for testability.
- [ ] 3.5 Prompt rendering: reuse any existing TUI/prompt helper in `internal/tui/` if present; otherwise a plain-fmt prompt is fine — this is not a BubbleTea scene.
- [ ] 3.6 Unit tests `internal/cli/apply_bootstrap_test.go`:
  - `--bootstrap` → consent=true in ctx; Bootstrap delegates.
  - `--no-bootstrap` + missing brew → exit non-zero; no prompt called.
  - Non-TTY + missing brew → exit non-zero; no prompt called.
  - TTY + `y` answer → retry with consent=true; Bootstrap delegates.
  - TTY + `N` answer → exit non-zero.
  - TTY + `s` answer → provider skipped for the run; other providers proceed.
  - `--bootstrap --no-bootstrap` → usage error exit 2.

## 4. Integration test

- [ ] 4.1 Create `internal/provider/builtin/homebrew/integration/Dockerfile.bootstrap` starting from `hams-itest-base:latest` with NO linuxbrew pre-installed. Add a non-root `brew` user + NOPASSWD sudo (same as the main Dockerfile) so the install.sh path can run.
- [ ] 4.2 Create `internal/provider/builtin/homebrew/integration/integration-bootstrap.sh` that:
  - Asserts `brew` is NOT on `$PATH`.
  - Writes a minimal hamsfile declaring one package.
  - Runs `hams apply --bootstrap --only=brew` (non-interactive consent path).
  - Asserts `brew` appears on `$PATH` after apply.
  - Asserts the declared package is installed.
- [ ] 4.3 Extend `Taskfile.yml` `ci:itest:run` or add a variant `ci:itest:run:variant` so brew's bootstrap scenario runs alongside the main brew integration test. Keep the main integration.sh (pre-installed brew) path untouched so regressions are easy to bisect.
- [ ] 4.4 Wire the new integration variant into the GitHub Actions `itest` matrix (two `brew` entries: `brew` and `brew-bootstrap`). `fail-fast: false` so one flaky variant doesn't mask the other.

## 5. Docs

- [ ] 5.1 `docs/content/en/docs/providers/homebrew.mdx` + zh-CN parity:
  - Document the default (error-out with actionable remedy).
  - Document `--bootstrap` as the non-interactive consent path.
  - Document the TTY prompt behavior with the Xcode-CLT gotcha.
- [ ] 5.2 `docs/content/en/docs/cli/apply.mdx` + zh-CN parity:
  - Document `--bootstrap` / `--no-bootstrap` in the flag table.
  - Mention that the flags also apply to `hams refresh`.
- [ ] 5.3 `README.md` + `README.zh-CN.md` fresh-machine restore example: update to `hams apply --bootstrap --from-repo=...`.

## 6. Verification

- [ ] 6.1 `task fmt` clean.
- [ ] 6.2 `task lint:go` clean.
- [ ] 6.3 `task test:unit` green with `-race` (incl. new bootstrap tests).
- [ ] 6.4 `task ci:itest:run PROVIDER=brew` — main (pre-installed) path still green.
- [ ] 6.5 `task ci:itest:run PROVIDER=brew-bootstrap` — new bootstrap path green.
- [ ] 6.6 `openspec validate homebrew-bootstrap-opt-in --strict` clean.
- [ ] 6.7 Manual doc spot-check: `docs/` renders via `pnpm dev`, homebrew + apply pages show the new content, no dead links.

## 7. Archive

- [ ] 7.1 `/opsx:verify homebrew-bootstrap-opt-in` — spec deltas map to code + tests; 0 critical / 0 warning.
- [ ] 7.2 `/opsx:archive homebrew-bootstrap-opt-in` — prefer `--skip-specs` + manual delta application given the known auto-sync header-matching bug on MODIFIED blocks (same workaround as 4 prior cycles).
- [ ] 7.3 Update `AGENTS.md` "Current Task" section with the cycle summary.
