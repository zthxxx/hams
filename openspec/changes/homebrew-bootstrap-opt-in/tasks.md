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
- [x] 4.2 Ensure unit tests cover every spec scenario — see §6.8.

## 5. Docs

- [x] 5.1 `docs/content/{en,zh-CN}/docs/providers/homebrew.mdx` — added a "First-time setup on a fresh Mac" section linking to the apply page's bootstrap-prompt section. Covers default (error), --bootstrap (non-interactive consent), and the interactive TTY prompt.
- [x] 5.2 `docs/content/{en,zh-CN}/docs/cli/apply.mdx` — added `--bootstrap` and `--no-bootstrap` rows to the flag table and a new "About the bootstrap prompt" section (+ zh-CN "关于 bootstrap 提示"). Per the spec scope-down, the flags are apply-only (refresh does not call Bootstrap), so no refresh-page change is needed.
- [x] 5.3 `README.md` + `README.zh-CN.md` updated fresh-machine restore example: `hams apply --bootstrap --from-repo=...` (with parenthetical note "if brew isn't installed yet / 如果还没装 brew，加上 --bootstrap").

## 6. Verification

- [x] 6.1 `task fmt` clean (ran inside `task check`).
- [x] 6.2 `task lint:go` clean (ran inside `task check`).
- [x] 6.3 `task test:unit` green with `-race` — 38 apt tests plus the new 16 bootstrap/consent tests (provider + homebrew + cli) all pass.
- [x] 6.4 `task ci:itest:run PROVIDER=apt` green — regression-checks that apply.go's modified bootstrap loop still handles the existing non-bootstrap-required case cleanly (apt's Bootstrap returns nil when apt-get is present). Output tail: "apt integration test passed".
- [x] 6.5 `task ci:itest:run PROVIDER=homebrew` green — main (pre-installed) path still works end-to-end: seed install → re-install → install-new → refresh → remove-via-hamsfile-delete. Confirms the modified Bootstrap (now returns BootstrapRequiredError when brew missing) still returns nil when brew IS present, and the rest of the apply pipeline is unaffected.
- [x] 6.6 Scope decision from §4: no `PROVIDER=brew-bootstrap` variant. The consent / delegation path is covered by unit tests branch-by-branch.
- [x] 6.7 `openspec validate homebrew-bootstrap-opt-in --strict` clean.
- [x] 6.8 Spec-to-test mapping (all 13 scenarios covered):
  - builtin-providers "actionable error on non-TTY" → `TestResolveBootstrapConsent_NonTTYDefaultsToDeny` + `TestRunApply_NoBootstrapFailsFastWithActionableError`
  - builtin-providers "runs with --bootstrap" → `TestResolveBootstrapConsent_AllowFlagRuns` + `TestRunApply_BootstrapFlagDelegatesThroughBashProvider`
  - builtin-providers "prompts on TTY" → `TestResolveBootstrapConsent_TTY{Yes,No,Skip,Empty,EOF}*` (5 variants)
  - builtin-providers "bootstrap failure is terminal" → `TestRunApply_BootstrapScriptFailureIsFatal`
  - builtin-providers "--no-bootstrap disables prompt" → `TestResolveBootstrapConsent_DenyFlagShortCircuits`
  - builtin-providers "RunBootstrap delegates" → `TestRunBootstrap_DelegatesRegisteredScript`
  - builtin-providers "RunBootstrap platform-gating" → `TestRunBootstrap_SkipsPlatformGated`
  - builtin-providers "RunBootstrap missing host" → `TestRunBootstrap_ErrorsOnMissingHostProvider`
  - cli-architecture "apply --bootstrap delegates" → `TestRunApply_BootstrapFlagDelegatesThroughBashProvider`
  - cli-architecture "apply --no-bootstrap fails fast" → `TestRunApply_NoBootstrapFailsFastWithActionableError`
  - cli-architecture "apply prompts on TTY" → `TestResolveBootstrapConsent_TTY*` series
  - cli-architecture "apply fails fast when not a TTY" → `TestResolveBootstrapConsent_NonTTYDefaultsToDeny`
  - cli-architecture "flags mutually exclusive" → `TestRunApply_BootstrapFlagsMutuallyExclusive`

## 7. Archive

- [x] 7.1 `/opsx:verify homebrew-bootstrap-opt-in` — spec deltas map to code + tests; 0 critical / 0 warning. 13 scenarios → 16 tests; every branch of the consent matrix exercised deterministically via injected prompt IO + TTY seam.
- [x] 7.2 `/opsx:archive homebrew-bootstrap-opt-in` — archive with `--skip-specs` given the known auto-sync header-matching bug on MODIFIED blocks (same workaround as 4 prior cycles). Apply deltas to main specs manually. See AGENTS.md for the final summary.
- [x] 7.3 Update `AGENTS.md` "Current Task" section with the cycle summary.
