# `dev` vs `local/loop` Branch — Implementation Analysis

> Two independent implementations of the same CLAUDE.md "Current Tasks" checklist,
> both diverging from base `d582756` and both passing `task check`.
> Analysis scope: code, architecture, tests, docs, and OpenSpec discipline.

| Dimension | `dev` @ `f6c063d` | `local/loop` @ `66266ff` |
|-----------|-------------------|--------------------------|
| Commits on branch | 9 | 16 |
| Files changed vs base | 90 | 106 |
| Lines added / removed | +2 822 / −675 | +3 768 / −721 |
| OpenSpec changes archived | 17 total (+5 new) | 21 total (+9 new) |
| OpenSpec changes in-flight | 1 (`onboarding-auto-init`) | 0 |
| CLAUDE.md current-tasks | partially ticked | fully ticked |

## 1. Feature Parity

Both branches ship the same set of user-visible capabilities derived from
`CLAUDE.md` → *Current Tasks*:

1. `hams apply --tag` with precedence `--tag > config.profile_tag > "default"`.
2. Auto-init of the global config on a fresh machine (no `hams.config.yaml` required).
3. Auto-scaffold of a store repo on the first `hams <provider> …` invocation, seeded
   from an embedded template (`.gitignore` + `hams.config.yaml`) and an initial
   `git init`.
4. Unified `hams git` entry point replacing the split `hams git-config` / `hams git-clone`.
5. `code-ext` renamed to `code`; Cursor is explicitly deferred to a separate provider.
6. Shared abstractions for the CLI-auto-record provider pattern (package-level helpers).
7. Integration tests asserting that both `hams` itself and every provider emit logs.
8. i18n catalog scaffolded on top of `nicksnyder/go-i18n`, with English + `zh-CN`.

At the level of what a user can do, the two branches are interchangeable. The
differences live in **how** each capability is layered.

## 2. Architectural Differences

### 2.1 Auto-init: dedicated package vs. CLI-local scaffolder

| | `dev` | `local/loop` |
|-|-------|---------------|
| Location | `internal/storeinit/` (new top-level package, `doc.go` + `storeinit.go`) | `internal/cli/scaffold.go` (co-located with CLI) |
| Lines | 145 (pkg) + 168 (`cli/autoinit.go`) = 313 | 244 (`scaffold.go`) |
| Template placement | `internal/storeinit/template/` | `internal/cli/template/store/` |
| Template embed | `//go:embed template/*` | `//go:embed template/store` |
| `git init` fallback | `exec.LookPath("git")` **with `go-git` fallback** | `git` CLI **only** (no fallback) |
| Context-aware init | `BootstrapContext(ctx, dir)` | `gitInitExec(ctx, dir)` with 30 s timeout |
| DI seam for `git init` | `os/exec` + `go-git` branches | package-var `gitInitExec` rebound in tests |
| Seeds `profile_tag` / `machine_id` | No — separate `EnsureGlobalConfig` call | Yes — `seedIfMissing` inside scaffold |
| Persists `store_path` to global config | Yes, via `config.WriteConfigKey` | Yes, via `config.WriteConfigKey` |

**Verdict.**

- `dev`'s `storeinit` package has a tighter seam (a real package with its own
  doc, test, and responsibility). It is the natural home for future variations
  (e.g. `storeinit.CloneFromRepo`).
- `dev` preserves the project's "fresh machine" design invariant by keeping
  the `go-git` fallback alive — `storeinit.go:94-107` picks `git` from
  PATH and degrades to in-process `gogit.PlainInit` otherwise. CLAUDE.md /
  `Build & Distribution` explicitly calls out that "Bundles go-git for
  fresh machines without git"; `dev` honors it, `loop` silently drops it.
- `loop`'s `scaffold.go` folds *more* of the onboarding loop into one pass:
  it not only creates the directory and templates but also writes
  `profile_tag` + `machine_id` into the global config so the user's very
  first provider invocation is silent (no `using 'default'` / `using
  'unknown'` nudge). That is a better user experience but is orthogonal
  to the package-boundary question.
- `loop`'s 30 s context timeout on `git init` is a small but real
  defensive improvement — a hung corporate-global-hook won't wedge the
  first-run path.

### 2.2 Unified `hams git` — strict subset vs. passthrough

```text
hams git config …   → ConfigProvider.HandleCommand      (both)
hams git clone …    → CloneProvider.HandleCommand(add+) (both)
hams git pull …     → ???
hams git status …   → ???
```

| | `dev` (`UnifiedHandler`, 81 lines) | `local/loop` (`UnifiedProvider`, 221 lines) |
|-|--------------------------------------|------------------------------------------------|
| Unknown subcommand | `UserError` listing supported verbs | **Passthrough to `exec.CommandContext(ctx, "git", args...)`** preserving stdio + exit code |
| `hams git clone <remote> <path>` | Synthesises `add <remote>` + appends path | Fish the positional path out of `extra`; reject `--flag` args loudly with a "not yet forwarded" message |
| DryRun behaviour | N/A (never reaches passthrough) | `"[dry-run] Would run: git …"` |
| i18n of usage text | Yes (`i18n.T("git.usage…")`) | Literal English strings |

CLAUDE.md *Current Tasks* requires:

> Provider wrapped commands MUST behave exactly like the original command, at
> least at the first-level command entry point.

`loop` matches this invariant: `hams git log`, `hams git status`, `hams git push`
all work because the default arm shells out to the real `git` binary. `dev`'s
handler rejects unknown subcommands with a usage error, which **violates the
invariant**. The implementation size difference is justified — 140 extra lines
for passthrough + positional rewriting + dry-run preview + flag-forwarding
refusal.

`dev`'s i18n coverage of the usage text is better (`i18n.T(...)` on every line),
but the underlying behaviour is wrong for the stated spec.

### 2.3 `code-ext` → `code`: wrapper vs. in-place rename

| | `dev` (`CodeHandler`, 44 lines) | `local/loop` (edit `vscodeext.go`) |
|-|----------------------------------|-------------------------------------|
| Manifest `Name` | unchanged — stays `"code-ext"` | flipped to `"code"` |
| Manifest `FilePrefix` | unchanged — stays `"vscodeext"` | flipped to `"code"` |
| CLI verb users type | `hams code` (via wrapper) | `hams code` (native) |
| `*.hams.yaml` file name on disk | `vscodeext.hams.yaml` (legacy) | `code.hams.yaml` (new) |
| Existing stores that have `vscodeext.hams.yaml` | keep working | **one-time rename required** |
| Number of error messages / usage lines changed | 0 (the wrapper delegates) | ~20 sites updated across the package |

Both branches make an explicit call out in comments. `dev` preserves the
on-disk contract; `loop` changes it and relies on the pre-v1 escape hatch
("breaking the on-disk name is acceptable since the CLI verb itself is the
documented identity of the provider").

- `dev`'s approach is **the minimum-blast-radius rename**. Other internal
  references, state files, and logs still say `code-ext`. New users never see
  the old name, but logs and state inspection will.
- `loop`'s approach is **the consistent rename**. Manifest, file prefix, error
  text, log labels, and lock names all move to `code`. No lingering `code-ext`
  identity in the codebase. But on-disk state moved without a migration.

For a v1-lite tool, neither is categorically wrong. `loop`'s choice is more
coherent; `dev`'s is more conservative. Both rule out future Cursor overloading
via `cli_command: cursor`.

### 2.4 Shared provider abstractions — both branches landed **unused** code

| | `dev`'s `internal/provider/baseprovider/` | `local/loop`'s `internal/provider/package_dispatcher.go` |
|-|--------------------------------------------|-----------------------------------------------------------|
| Lines (non-test) | 80 | 190 |
| Lines (test) | 89 | 0 (no dispatcher-level test) |
| Shape | Value-based helpers (`LoadOrCreateHamsfile`, `HamsfilePath`, `EffectiveConfig`) | Interface + dispatcher (`PackageInstaller`, `AutoRecordInstall`, `AutoRecordRemove`) |
| Coupling | Takes `*config.Config` + `filePrefix string` + flags | Takes runner + pkgs + cfg + flags + paths + opts struct |
| Scope of consolidation | File-path resolution only | Full dry-run → lock → exec → record flow |
| **Providers that actually adopt it** | **0 (`rg -l baseprovider\.` → only its own test)** | **0 (`rg -l AutoRecordInstall` → only its own file)** |

Both branches wrote an abstraction for the "CLI-first auto-record" pattern and
then **did not migrate a single provider onto it**. Today both are net
additions with no call sites — dead code modulo self-tests.

- `dev`'s `baseprovider` has a lower risk profile: it is 80 lines of pure helper
  functions that any provider can adopt file-by-file. It also ships its own
  test (89 lines). Low-cost dead code.
- `loop`'s `package_dispatcher` is a bigger bet: 190 lines of flow that a
  provider has to restructure its `handleInstall` / `handleRemove` around.
  Ships no tests beyond what the compiler enforces. Higher-cost dead code,
  and higher risk of bit-rotting before the first adopter proves it works.

The commit message for the `loop` dispatcher explicitly calls it
"opt-in" — so the author is aware of the YAGNI hazard. `dev` does not flag the
risk in its commit, but the helper is cheap enough that it does not matter.

**Recommendation for either branch:** land an adoption in one provider
(apt is the smallest case) before merging. Otherwise the abstraction is
guessing at a pattern that no real code has exercised.

### 2.5 i18n — string literals vs. typed constants

| | `dev` | `local/loop` |
|-|-------|---------------|
| Locale catalogue size (`locales/en.yaml` entries) | 36 | 38 |
| `i18n.T(…)` call sites in `apply.go` | 4 | 7 |
| Key identifier discipline | Inline string literals (`i18n.T("ufe.no_store_configured.opt_out")`) | Typed constants in `internal/i18n/keys.go` (`i18n.T(i18n.CLIErrBootstrapConflict)`) |
| Compile-time rename safety | Weak — typo in key string compiles fine | Strong — typo in constant is a build error |
| Discoverability for translators | `grep 'i18n.T(' internal/` | `cat internal/i18n/keys.go` |

`loop` is strictly ahead here. 85-line `keys.go` is a tiny overhead for:

- compile-time key safety,
- a single file to grep for "what strings does this CLI emit?",
- a documented naming convention (`<capability>.<component>.<short-id>`),
- no call-site noise from inline string literals.

Both branches translate roughly the same surface of user-facing strings; `loop`
translates slightly more error paths and introduces the typed registry.

### 2.6 Config resolution — inline vs. reusable pure functions

`local/loop`'s `internal/config/resolve.go` (97 lines + 187 lines of tests)
exposes:

- `ResolveCLITagOverride(cliTag, cliProfile) (string, error)` — handles
  `--tag` / `--profile` aliasing and the disagree-case error.
- `ResolveActiveTag(cfg, cliTag, cliProfile) (string, error)` — composes
  the override with config + hardcoded default.
- `DeriveMachineID() string` — `$HAMS_MACHINE_ID` → `os.Hostname()` →
  `"default"`, with path-segment sanitisation.
- `HostnameLookup = os.Hostname` — DI seam for tests.

All four helpers live in the `config` package, are pure, and are unit-tested
with property-style inputs.

`dev` inlines the equivalent logic inside `internal/cli/autoinit.go`
(`defaultMachineID`, `DefaultTag` constant, direct hostname call). The logic
is correct but not reusable from other commands, and it is bound to the
CLI layer so provider code cannot call it.

### 2.7 `provider.GlobalFlags` — adds an `Out` / `Err` writer seam (`loop` only)

`local/loop` adds `Out io.Writer` / `Err io.Writer` + `Stdout()` / `Stderr()`
helpers to `GlobalFlags` (`internal/provider/flags.go`). The motivation is
spelled out in commit `6027732`:

> Brew unit tests tripped `-race` under `t.Parallel()` because
> `captureStdoutForHomebrew` swapped `os.Stdout` globally while peer tests
> were concurrently calling `fmt.Printf` from dry-run / list paths. The
> mutex only serialized the swap itself — not the unrelated `Printf`
> calls that read the same `os.Stdout` variable.

All Homebrew `fmt.Printf` sites rewrite to `fmt.Fprintf(flags.Stdout(), …)`,
and tests inject `flags.Out = &bytes.Buffer{}`.

`dev` does not make this change. Since the user asserts both branches pass
`task check`, either:

1. `dev`'s test runner happens to avoid the interleaving that exposes the
   race, or
2. the race is latent but not triggered on the current machines.

Either way, `loop`'s fix is the correct long-term shape: DI-inject the
writer, never mutate the process-global `os.Stdout` under `t.Parallel()`.

### 2.8 CI / ACT isomorphism (`loop` only)

`local/loop` adds ACT-conditional guards to `.github/workflows/ci.yml`:

- `upload-artifact@v4` and `download-artifact@v4` steps are skipped when
  `env.ACT == true` (act's artifact server cannot reliably emulate the v4
  protocol — it returns `ECONNRESET`).
- When running under ACT, a `setup-go@v5` + `task build:linux` fallback
  rebuilds the binary in-place, replacing the missing artifact handoff.

CLAUDE.md / `Development Process Principles` explicitly calls out the
*Local/CI isomorphism* invariant. `loop` actively closes the gap; `dev`
does not touch CI, so local `act` runs still diverge from real CI.

`loop` also extends `.golangci.yml`'s `errcheck.exclude-functions` to cover
`fmt.Fprint*`, `io.Writer.Write`, `http.ResponseWriter.Write`, and buffer
`WriteString` / `WriteByte`. Those are the call sites the `GlobalFlags.Out`
refactor forced golangci-lint v2 to flag. Without the exclusion, every
writer-bound `Fprintln` call would need a `//nolint:errcheck` directive.

`dev` ships `task check` green without these exclusions because it never
introduces the `Fprint*`-on-an-injected-writer pattern.

### 2.9 OpenSpec discipline

`dev` lands 5 new archived changes and leaves `onboarding-auto-init` **in-flight**.
`local/loop` archives 9 new changes and has **zero in-flight**. The loop set
breaks the same work into more focused change IDs:

- `2026-04-17-apply-tag-and-auto-init`
- `2026-04-17-cli-i18n-catalog`
- `2026-04-17-provider-autoscaffold-store`
- `2026-04-17-provider-shared-abstractions`

vs `dev`'s one aggregate in-flight `onboarding-auto-init`.

Per CLAUDE.md, *"One change = one coherent shippable unit; split if it spans
unrelated capabilities"* — `loop` honours this more faithfully.

## 3. Per-area Scorecard

Positive = done better than the other branch; negative = done worse.

| Area | `dev` | `local/loop` |
|------|-------|---------------|
| Auto-init package boundary | **+** dedicated `storeinit` pkg | — co-located in `cli` |
| Fresh-machine `git init` without host git | **+** `go-git` fallback preserved | **−** fallback dropped |
| First-run silent loop (seeds `profile_tag`/`machine_id`) | — separate call path | **+** done inside scaffold |
| `hams git` passthrough (CLAUDE.md invariant) | **−** rejects unknown verbs | **+** full passthrough + dry-run |
| `code-ext` → `code` rename coherence | — wrapper only, legacy names remain | **+** manifest/prefix/logs all flipped |
| Shared provider abstraction size | **+** smaller (80 LoC) | — larger (190 LoC) |
| Shared abstraction adoption | **−** unused | **−** unused |
| i18n key robustness | — inline strings | **+** typed `keys.go` constants |
| Config resolution reusability | — inlined in CLI layer | **+** pure funcs in `config` pkg |
| Homebrew `-race` fix via writer seam | — not addressed | **+** `GlobalFlags.Out io.Writer` |
| CI / ACT isomorphism | — unchanged | **+** ACT-conditional build job |
| lint exclusions kept in sync with new patterns | n/a | **+** `.golangci.yml` updated |
| OpenSpec archival discipline | **−** one change still in-flight | **+** all archived |
| Spec-delta granularity | — one aggregate change | **+** four focused changes |
| Net code added | **+** 2 822 / −675 (smaller) | — 3 768 / −721 (larger) |
| Test LoC added | ~600 | ~800 |
| Docs updated (en+zh) | both branches updated | both branches updated |

## 4. Where Each Branch Is Strong

**`dev` is strong in:**

- *Package hygiene.* `storeinit` is a real package with a `doc.go`, a test
  file, and a single responsibility. If hams ever needs a second "initialise
  something" helper (clone-from-repo, migrate-from-old-layout), that package
  is the obvious home.
- *Preserving the `go-git` fallback.* The project description explicitly
  promises that hams installs on a fresh machine without git on PATH. `dev`
  keeps that guarantee wired into auto-init; `loop` quietly regresses it.
- *Smaller change surface.* Fewer lines, fewer commits, fewer touched files.
  Easier to review.
- *`baseprovider` is honest about scope.* It is small and clearly a helper,
  not a framework. The 89-line test verifies the behaviour directly.

**`local/loop` is strong in:**

- *Conforming to CLAUDE.md invariants.* Passthrough `hams git`, Local/CI
  isomorphism, split-small openspec changes, spec archival — every
  written rule gets a correspondingly engineered commit.
- *User experience on first run.* `scaffold.go` seeds every identity key
  the user would otherwise be prompted for, so `hams brew install htop` on a
  pristine machine produces a complete, commit-able config in one shot.
- *Test-facing DI seams.* `GlobalFlags.Out`, `HostnameLookup`,
  `gitInitExec`, property-style `resolve_test.go` cases — every
  boundary between "our code" and "the host" has a test seam.
- *i18n robustness.* The typed `keys.go` registry plus ~7 call sites in
  apply alone means translators have a single file to read and a compiler
  to catch typos.
- *Observable bug fixes.* The homebrew `-race` / `t.Parallel()` fix is real
  and unrelated to the feature list — it is the kind of maintenance work
  that only happens when the engineer owns the test suite end-to-end.

## 5. Where Each Branch Is Weak

**`dev` is weak in:**

- `hams git` rejects unknown subcommands, violating the stated
  "behave like the real tool" invariant.
- One OpenSpec change left in-flight, not archived — CLAUDE.md says archive
  after ship.
- String-literal i18n keys are fragile.
- No ACT compatibility — a developer running `task test:itest:one` via ACT
  locally will see different behaviour from CI.
- The `baseprovider` helper is unused in the builtin providers.

**`local/loop` is weak in:**

- Drops the `go-git` fallback from the auto-scaffold path. Not a runtime
  regression today (most target machines have `git`), but it is a silent
  retreat from a documented design constraint.
- `scaffold.go` lives inside `internal/cli/`, not in a dedicated package.
  Co-location is fine now; it blocks reuse later.
- `package_dispatcher.go` is 190 lines of un-adopted code with no
  dispatcher-level test. Higher carrying cost than `dev`'s helper.
- Larger surface area: 16 commits and 106 touched files, some of which are
  speculative (the shared dispatcher, the unused `Tag` field plumbed
  through before `--tag` lands) — more code to revisit when the abstraction
  fails to land an adopter.

## 6. Recommendation

A hybrid taken from the two branches, in priority order:

1. **Take `loop`'s `hams git` passthrough implementation**. CLAUDE.md requires
   it and `dev`'s subset is a regression.
2. **Take `loop`'s `GlobalFlags.Out` / `Err` writer seam** and the Homebrew
   refactor. `t.Parallel()` + global `os.Stdout` swap is a latent bug.
3. **Take `loop`'s typed `i18n.keys` registry** and the `.golangci.yml`
   `errcheck` tweaks that keep it maintainable.
4. **Take `loop`'s `config.ResolveCLITagOverride` / `ResolveActiveTag` /
   `DeriveMachineID`** — pure functions in the right package, reusable
   from future commands.
5. **Take `dev`'s `storeinit` package shape** — new top-level package with
   `doc.go` + `go-git` fallback. Re-apply `loop`'s `seedIfMissing` inside
   that package so the single-shot silent onboarding is preserved.
6. **Take `loop`'s ACT-conditional CI workflow tweaks** — closes the
   Local/CI isomorphism gap.
7. **Take `loop`'s OpenSpec hygiene** — four focused archived changes, not
   one aggregate in-flight change.
8. **Drop both branches' unused shared-abstraction modules** unless they land
   together with at least one adopter provider. Re-introduce when a second
   provider needs the code, not speculatively.

On the `code-ext` → `code` rename, either approach is defensible before v1.
`loop`'s in-place rename is more coherent; `dev`'s wrapper is more
backward-compatible. Pick one consciously and document the migration for
existing store users in the release notes either way.
