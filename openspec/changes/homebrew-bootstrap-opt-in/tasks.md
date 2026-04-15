# Tasks

## 1. Provider framework: bootstrap primitive

- [x] 1.1 Add `internal/provider/bootstrap.go` with `BashScriptRunner` interface (`RunScript(ctx, script string) error`) + `RunBootstrap(ctx, p, registry) error` that iterates `DependsOn`, platform-filters, and delegates non-empty `.Script` to a registered `BashScriptRunner`.
- [x] 1.2 Add `WithBootstrapAllowed(ctx, bool)` + `BootstrapAllowed(ctx) bool` ctx helpers in `internal/provider/bootstrap.go`.
- [x] 1.3 Add `ErrBootstrapRequired = errors.New(...)` sentinel + `BootstrapRequiredError` typed error (Provider / Binary / Script fields; `Unwrap() error` → sentinel) in the same file.
- [x] 1.4 Reuse existing `*provider.Registry` via `Get(name) Provider`. No Lookup alias needed.
- [x] 1.5 Make the Bash builtin provider (`internal/provider/builtin/bash/bash.go`) implement `provider.BashScriptRunner`. Implementation shells out via `/bin/bash -c <script>` with stdin/stdout/stderr passthrough and a package-level `bootstrapExecCommand` DI seam.
- [x] 1.6 Unit tests `internal/provider/bootstrap_test.go` + `internal/provider/builtin/bash/bash_test.go`: delegation, platform gating, missing host, non-BashScriptRunner host, script failure propagation, empty-script skip, declaration-order multi-entry, ctx helpers, bash RunScript via injected exec seam.

## 2. Homebrew provider: consent-aware Bootstrap

- [x] 2.1 Rewrite `Bootstrap(ctx)` to return `*provider.BootstrapRequiredError` (which wraps `ErrBootstrapRequired`) when `brew` is missing. Provider stays pure — no registry dependency, no ctx reads; orchestration lives in the apply CLI layer.
- [x] 2.2 Swap `exec.LookPath("brew")` for a package-level `brewBinaryLookup` var (DI seam) so tests can simulate brew-present / brew-missing deterministically.
- [x] 2.3 Unit tests `internal/provider/builtin/homebrew/bootstrap_test.go`: brew-present returns nil, brew-missing returns BootstrapRequiredError wrapping the sentinel + carrying the manifest script verbatim, Script-matches-manifest invariant.

## 3. Apply CLI: flag wiring + TTY prompt

- [x] 3.1 Added `--bootstrap` and `--no-bootstrap` flags on `hams apply`. Mutually exclusive; both-set = exit 2 usage error. (Refresh intentionally skipped — spec amended to reflect that refresh does not call Bootstrap; Probe errors are logged per-provider without aborting the run.)
- [x] 3.2 Threaded `bootstrapMode{Allow, Deny}` through `runApply` signature. Tests updated to pass `bootstrapMode{}` on all existing call sites.
- [x] 3.3 On `Bootstrap` returning `ErrBootstrapRequired` AND a hamsfile is present:
  - `--no-bootstrap` OR non-TTY → treat as regular bootstrap failure (UserFacingError with provider list).
  - `--bootstrap` → `provider.RunBootstrap(ctx, p, registry)` + retry `p.Bootstrap(ctx)`.
  - TTY + neither flag → `[y/N/s]` prompt. `y` = run+retry; `N`/EOF/empty = deny; `s` = remove provider from `sorted` for this run.
- [x] 3.4 TTY detection via `golang.org/x/term.IsTerminal(int(os.Stdin.Fd()))` behind a `bootstrapPromptIsTTY` var for testability.
- [x] 3.5 Prompt rendering: plain-fmt prompt (not BubbleTea — this is a yes/no consent, not a multi-step scene). Input/output seams are `bootstrapPromptIn` / `bootstrapPromptOut` vars for deterministic testing.
- [x] 3.6 Unit tests `internal/cli/bootstrap_consent_test.go`: `resolveBootstrapConsent` decision matrix (7 cases) + end-to-end `runApply` with `--no-bootstrap` fail-fast, `--bootstrap` delegation through bash fake, bootstrap script failure is fatal, mutual-exclusion usage error.

## 4. Integration test — scope decision

After drafting the new Dockerfile.bootstrap, surfaced architectural
concern: hams can't easily substitute the manifest-declared install.sh
URL, so any "honest" integration test of the --bootstrap path would
re-execute the real linuxbrew `install.sh`, which (a) adds ~5 min to
every `brew-bootstrap` matrix run, (b) exercises network fragility
against `raw.githubusercontent.com`, (c) redundantly exercises the
same byte path that the main brew integration Dockerfile's build-time
RUN already covers.

**Decision:** skip the full end-to-end variant. The orchestration is
fully covered by unit tests (see tasks 1.6 / 2.3 / 3.6 — 16 tests
touching every branch in the consent decision matrix + the delegation
happy/sad paths). The main `brew` integration test retains its
value (pre-installed brew → hams operations end-to-end). Fresh-brew
bootstrap is manually verifiable via the unit tests' recorded script
argument matching the manifest.

- [x] 4.1 Document the scope decision above.
- [x] 4.2 Ensure unit tests cover every spec scenario. Map: Scenario "actionable error on non-TTY + no flag" → `TestResolveBootstrapConsent_NonTTYDefaultsToDeny` + `TestRunApply_NoBootstrapFailsFastWithActionableError`. "Runs with --bootstrap" → `TestRunApply_BootstrapFlagDelegatesThroughBashProvider`. "Prompts on TTY" → `TestResolveBootstrapConsent_TTY*` (5 variants). "Bootstrap failure terminal" → `TestRunApply_BootstrapScriptFailureIsFatal`. "--no-bootstrap suppresses prompt" → `TestResolveBootstrapConsent_DenyFlagShortCircuits`. All scenarios → test-mapped.

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
