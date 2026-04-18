# Branch Comparison: `dev` vs `local/loop`

Date: 2026-04-18
Base commit: `d582756 docs: review result and found issues`
Tip commits:

- `dev` → `f6c063d fix(itest): vscodeext --only filter must use Manifest.Name` (9 commits ahead)
- `local/loop` → `66266ff docs+specs: sync stale refs and document ralph-loop behaviour additions` (16 commits ahead)

Both branches solve the same problem set — the six "Current Tasks" captured in `CLAUDE.md`: archive the five completed 2026-04-16 OpenSpec changes; add `--tag` + auto-init scaffold; merge `git-clone` + `git-config` into a unified `hams git`; rename `code-ext` → `code`; add i18n; assert logging in integration tests; extract shared provider helpers. Both pass `task check` at HEAD. The difference is in *how they got there*.

The numbers give the first contrast:

| Metric | `dev` | `local/loop` |
|---|---|---|
| Commits since `d582756` | 9 | 16 |
| Files changed | 90 | 106 |
| Insertions | 2 822 | 3 768 |
| Deletions | 675 | 721 |
| New Go files | 10 (storeinit, baseprovider, autoinit, unified, code_handler…) | 14 (scaffold, resolve, keys, package_dispatcher, flags, unified…) |
| New OpenSpec changes | 1 bundled (`2026-04-17-onboarding-auto-init/`) | 4 focused (apply-tag, i18n-catalog, git+code, logging) |
| Integration scripts with new log assertions | 1 (apt) | 3 (bash, ansible, git) |
| Providers migrated to shared helpers | 0 — scaffolding only | 0 — scaffolding only |

---

## 1. Overall strategy

### `dev` — minimum viable shipping unit

`dev` collapses every task item into **one** OpenSpec change (`2026-04-17-onboarding-auto-init/`) whose `tasks.md` explicitly defers the long tails: §9.1 docs sync, §9.3 log-assertion fan-out, §9.4 i18n fan-out, §9.5 provider migration to `baseprovider`. Each commit is scoped tightly — *the* scaffold, *the* git+code merge, *the* i18n infra — and then `05237b3` + `df093ee` + `2b6d19e` land the feature while leaving behind a clear punch list.

Reading the commit stream in order, the intent is "ship the user-visible surface, then fan out the follow-up cleanup in later cycles":

```text
ba7a417 chore(openspec): archive 5 implemented 2026-04-16 changes
df093ee feat(onboarding): --tag flag + auto-init store on first run
fa38f49 docs(onboarding-auto-init): add spec deltas + update tasks.md
05237b3 feat(providers): unified hams git + hams code entry points
2b6d19e feat(i18n): wire user-facing strings through go-i18n + add zh-CN
83b39aa test(integration): assert hams emits logs in apt integration test
7c3a003 docs(claude.md): mark all CLAUDE.md current-task checklist items done
1832d64 docs: sync site with --tag, auto-init, and unified hams git/code
f6c063d fix(itest): vscodeext --only filter must use Manifest.Name
```

### `local/loop` — decomposed specs + fuller implementation

`local/loop` breaks the same set of tasks into **four** focused OpenSpec changes and lands the full fan-out inside the same branch:

```text
6027732 fix(homebrew): kill -race flake via Out io.Writer seam on GlobalFlags
ba7a417 chore(openspec): archive 5 implemented changes         (same as dev)
4ca1904 feat(cli): hams apply --tag + auto-init on fresh machines
3155642 feat(cli): auto-scaffold store on first provider invocation
d61b792 feat(providers): unify hams git + rename code-ext → code
7e8f32f feat(itest): verify logging in integration tests for every provider
2ccd9aa feat(i18n): catalog skeleton + 13 highest-visibility sites translated
8efbd72 feat(provider): shared package-dispatcher helpers (opt-in)
72c9403 chore(openspec): archive 4 Current Tasks changes + tick CLAUDE.md
132d241 docs(agents): update provider count + code-ext→code after git/code merge
d63edb9 fix(test): default local e2e/itest to ci:* tasks; act is now opt-in
3cafb27 fix(cli): silence profile_tag/machine_id warnings on --help/--version
4c43b5a feat(cli): scaffold seeds profile_tag + machine_id on fresh onboarding
5ea31df docs: sync provider catalogue and onboarding
bf84faa chore(openspec): trim double-date prefix from 7 archived changes
66266ff docs+specs: sync stale refs and document ralph-loop behaviour additions
```

Each commit is small and independent, and the spec bodies are targeted enough to be reviewed in isolation. The cost is a broader surface area — more files, more new packages, more moving pieces merging simultaneously.

### Summary

`dev` is the classic *ship the primitives, tile the edges in follow-up cycles* approach; `local/loop` is the *compose the whole story in one branch* approach. Neither is wrong. `dev` keeps the PR reviewable; `local/loop` leaves less debt at the finish line.

---

## 2. Auto-init scaffolding

Both branches add a first-run experience where invoking `hams brew install …` on a pristine machine creates a git-initialized store, writes a templated `hams.config.yaml` + `.gitignore`, and persists `store_path` back to the global config. They diverge on *where* that code lives and *how* it runs.

### `dev` — dedicated `internal/storeinit` package

```go
// internal/storeinit/storeinit.go
func Bootstrap(dir string) error { return BootstrapContext(context.Background(), dir) }
func BootstrapContext(ctx context.Context, dir string) error { /* mkdir → git init → templateFS walk */ }
func Bootstrapped(dir string) bool { /* has .git + hams.config.yaml */ }
```

- **Isolation** — bootstrapping lives in its own package (`internal/storeinit`, 145 LoC + 109 LoC test) which the CLI layer calls through `autoinit.EnsureStoreReady`. Templates are embedded under `internal/storeinit/template/` so `storeinit` is self-contained.
- **Git CLI + go-git fallback** — runs `git init` via the binary when available and falls back to `github.com/go-git/go-git/v5` `PlainInit` when git is not on PATH. This matches the "bundles go-git for fresh machines without git" line in `CLAUDE.md`.
- **Bootstraps `<store>/default/`** — materializes the default profile directory alongside the template files.
- **Tiny CLI-layer glue** — `internal/cli/autoinit.go` (168 LoC) only handles the config side (`EnsureGlobalConfig` writes `tag: default` + `machine_id: <hostname>`) and calls into `storeinit.Bootstrap`. Clear layering.
- **Opt-out via `HAMS_NO_AUTO_INIT`** — dev exposes an env escape hatch so the legacy "no store directory configured" UserFacingError stays reachable (used by tests that want the negative path without touching `$HOME`).

### `local/loop` — in-CLI `scaffold.go`

```go
// internal/cli/scaffold.go
func EnsureStoreScaffolded(ctx context.Context, paths config.Paths, flags *provider.GlobalFlags) (string, error) {
    // pick path → mkdir → git init (DI seam) → template writes → persist store_path →
    // seed profile_tag + machine_id (if missing)
}
```

- **Single-file implementation** — 244 LoC in `internal/cli/scaffold.go` + 257 LoC test.
- **DI seam for git** — `var gitInitExec = func(ctx, dir) error { … }` lets tests swap the git invocation without a separate package. No go-git fallback; relies on the system `git`.
- **Context timeout** — wraps `git init` in a 30-second `context.WithTimeout` so a hung corporate hook cannot wedge the first-run path. (`dev` does not time-box the exec.)
- **Dry-run aware** — `EnsureStoreScaffolded` honours `flags.DryRun`: prints a preview and skips all side effects. (`dev`'s `EnsureStoreReady` runs git init unconditionally.)
- **Identity seeding** — `seedIfMissing(paths, "profile_tag", …)` + `seedIfMissing(paths, "machine_id", config.DeriveMachineID)` writes both fields to the global config on the first provider invocation, so subsequent commands do not emit the "profile_tag is empty" nudge. (Commit `4c43b5a` explicitly addresses the "post-first-install experience feels broken" problem.)
- **No opt-out env var** — auto-init fires whenever no store is resolved. Unit tests inject a seam via test doubles rather than an env switch.

### Assessment

- **`dev` wins on package hygiene.** Putting bootstrap in its own package — with its own embedded FS — isolates a concern that has nothing to do with the CLI parser. Reusable from `hams store init` later without an import cycle.
- **`local/loop` wins on UX completeness.** The dry-run short-circuit, the context timeout on `git init`, and the identity-seeding step are genuine improvements over `dev`'s version. A user typing `hams --dry-run brew install htop` on a pristine host sees a preview instead of a half-created store.
- **`dev`'s go-git fallback is a hams-specific constraint** (the binary is shipped to fresh machines without git). `local/loop` drops it — not a regression for CI / Docker environments where git is always present, but a regression for the bundle-for-airgap user story.
- **Opt-out** — `HAMS_NO_AUTO_INIT` is not obviously better or worse than no opt-out. In a "silent fresh onboarding" world you arguably *want* the auto-init to always fire; `dev` kept the switch for negative-path testing, `local/loop` solves the same test need with a seam. Both valid.

---

## 3. Unified `hams git` dispatcher

Both branches merge `git-config` + `git-clone` into one CLI verb while keeping the two providers as separate apply/refresh resources. The dispatcher code is where the philosophies diverge most visibly.

### `dev` — thin router

`internal/provider/builtin/git/unified.go` (81 LoC):

```go
func (u *UnifiedHandler) HandleCommand(ctx, args, hamsFlags, flags) error {
    if len(args) == 0 { return hamserr.NewUserError(…, i18n.T("git.usage.header"), …) }
    subcommand, rest := args[0], args[1:]
    switch subcommand {
    case "config":
        return u.cfgProvider.HandleCommand(ctx, rest, hamsFlags, flags)
    case "clone":
        return u.cloneProvider.HandleCommand(ctx, append([]string{"add"}, rest...), hamsFlags, flags)
    default:
        return hamserr.NewUserError(…, i18n.Tf("git.unknown_subcommand", …), …)
    }
}
```

Two branches plus a usage error. Unknown subcommands surface a help-style UFE that *names* the supported verbs.

### `local/loop` — transparent passthrough

`internal/provider/builtin/git/unified.go` (221 LoC) does the same routing for `config` / `clone`, *plus*:

- `hams git <anything-else>` shells out to the real `git` binary with stdio + exit code preserved. `hams git pull`, `hams git status`, `hams git log` all work identically to unwrapped git.
- `hams git clone <remote> <path>` without `--hams-path=` translates into the CloneProvider's internal `add <remote> --hams-path=<path>` DSL. This makes the natural `git clone` invocation work end-to-end.
- `--depth`, `--branch`, and other git flags that hams does not yet forward are rejected with an explicit UFE directing the user to file a follow-up — no silent drop.
- `flags.DryRun` on the passthrough prints `[dry-run] Would run: git <args>` and returns.

### Assessment

The project's own rule (`CLAUDE.md`):

> Provider wrapped commands MUST behave exactly like the original command, at least at the first-level command entry point.

`local/loop`'s passthrough is a straightforward read of that rule — `hams git log` is identical to `git log`. `dev`'s version fails closed: `hams git log` errors with "unknown subcommand". A user who aliases `git=hams git` to get auto-record for free will find `dev`'s version breaks half their muscle memory, while `local/loop`'s just works.

Countervailing: `dev`'s version is i18n-clean (every message path goes through `i18n.T`/`i18n.Tf`), while `local/loop`'s UFEs are hard-coded English strings. In a codebase that committed to i18n this cycle, that is a real regression — both branches need fan-out, but `local/loop` added new English-only strings on the same branch that landed the i18n infra.

Net: `local/loop`'s surface is closer to the spec; `dev`'s is more internally consistent.

---

## 4. `--tag` flag wiring

Both branches expose `--tag=<tag>` globally so `hams apply --tag=macOS --from-repo=…` works as the one-command restore entry point.

### `dev` — alias

`--tag` is registered as a second `cli.StringFlag` whose value is read alongside `--profile`, both of which feed the same `flags.Profile` field:

```go
// internal/cli/root.go
Profile: cmd.String("tag"),  // `--tag` is the canonical name; `--profile` is the legacy alias
```

There is no distinct `flags.Tag`. `--tag=macOS --profile=linux` silently lands one of them in `flags.Profile` depending on declaration order — no conflict detection.

### `local/loop` — separate flag + explicit resolver

`provider.GlobalFlags` grows a new `Tag string` field alongside the existing `Profile string`. Both flags are parsed independently, and a dedicated helper unifies them:

```go
// internal/config/resolve.go
func ResolveCLITagOverride(cliTag, cliProfile string) (string, error) {
    if cliTag != "" && cliProfile != "" && cliTag != cliProfile {
        return "", hamserr.NewUserError(hamserr.ExitUsageError,
            i18n.T(i18n.CLIErrTagProfileConflict),
            "Remove either --tag="+cliTag+" or --profile="+cliProfile)
    }
    if cliTag != "" { return cliTag, nil }
    return cliProfile, nil
}
```

`apply.go`, `provider_cmd.go`, and the auto-init path all call `ResolveCLITagOverride` before touching config.

### Assessment

`local/loop`'s explicit resolver is safer — a user who has `--profile=linux` baked into an old CI script and adds `--tag=macOS` at the terminal gets a loud error, not a silent pick. `dev` just lets the second flag win. Given that `--profile` is *deprecated* (the canonical name is now `--tag`), some users will be running both for a transitional period; the conflict detection is genuinely useful there.

`dev`'s alias approach wins on implementation cost (one line in `root.go`). `local/loop` pays with a new package-level function, a new `GlobalFlags` field, and three call-sites that need to call `ResolveCLITagOverride`, but the behaviour is what users would want.

---

## 5. i18n rollout

Both branches:

- Import `github.com/nicksnyder/go-i18n/v2/i18n` + `golang.org/x/text/language`.
- Ship `internal/i18n/locales/{en,zh-CN}.yaml`.
- Add a `sync.Once`-guarded lazy init so library packages (providers, helpers) can call `T()` / `Tf()` without forcing every test entry-point to bootstrap the localizer explicitly.

Two differences stand out.

### Typed message-ID catalog

`local/loop` adds `internal/i18n/keys.go` — a dedicated file of `const CLIErrTagProfileConflict = "cli.err.tag-profile-conflict"`-style declarations with doc comments explaining each key's context and who emits it. Every call site reads `i18n.T(i18n.CLIErrTagProfileConflict)` instead of a bare string literal.

Benefits:

- Compile-time catches for typos — `i18n.T("cli.err.tag-profile-conflic")` compiles; `i18n.T(i18n.CLIErrTagProfileConflic)` does not.
- Grep-friendly: `rg i18n\.CLIErr` instantly locates every error-message call-site.
- Single source of truth for translators — the YAML files must declare every key in `keys.go`.

`dev` uses string literals throughout (`i18n.T("ufe.no_store_configured.opt_out")`), trading compile-time safety for file-size parity with the call-sites on `main`.

### Call-site density

`rg -c 'i18n\.T[f]?\('` over `internal/`:

| File | `dev` | `local/loop` |
|---|---:|---:|
| `internal/cli/apply.go` | 4 | 6 |
| `internal/cli/commands.go` | 0 | 8 |
| `internal/cli/autoinit.go` / `scaffold.go` | 2 | 0 |
| `internal/config/resolve.go` | 0 | 1 |
| `internal/provider/builtin/git/unified.go` | 7 | 0 |

Total `T`/`Tf` call-sites: `dev` ≈ 13, `local/loop` ≈ 15. Similar surface area, but concentrated in different places:

- `dev` pushes i18n into the unified git dispatcher (all user-visible strings there are localized) but leaves `commands.go` untouched.
- `local/loop` i18n-izes the top-level CLI errors (`commands.go` + `apply.go` mutex messages) but ships the git dispatcher with raw English.

Neither has full i18n coverage; both branches' `tasks.md` explicitly defer ~50 `hamserr.NewUserError` + ~100 `fmt.Print` call-sites to follow-up cycles.

### Assessment

The typed-key catalog is a meaningful design improvement and independent of the call-site fan-out. It should be cherry-picked regardless of which implementation wins on the other dimensions. `dev`'s fan-out is better in the git dispatcher; `local/loop`'s is better in the top-level CLI errors. Either direction is acceptable.

---

## 6. Shared provider helpers

Both branches scaffold reusable helpers so that future providers can drop into a template instead of re-implementing the same boilerplate. The ambitions differ by an order of magnitude.

### `dev` — `baseprovider` package (narrow)

`internal/provider/baseprovider/baseprovider.go` (80 LoC) exposes three functions that every builtin auto-record provider duplicates today:

```go
func LoadOrCreateHamsfile(cfg, filePrefix, hamsFlags, flags) (*hamsfile.File, error)
func HamsfilePath(cfg, filePrefix, hamsFlags, flags) (string, error)
func EffectiveConfig(cfg, flags) *config.Config
```

This covers the hamsfile-path resolution + "effective config overlay" that every provider's `hamsfile.go` currently re-implements with 20–40 LoC of the same conditional chain. Migrating a provider is a two-line swap. The helper is deliberately out-of-scope for install/uninstall flow.

### `local/loop` — `package_dispatcher.go` (broad)

`internal/provider/package_dispatcher.go` (190 LoC) goes much further:

```go
type PackageInstaller interface {
    Install(ctx context.Context, pkg string) error
    Uninstall(ctx context.Context, pkg string) error
}

func AutoRecordInstall(ctx, runner PackageInstaller, pkgs, cfg, flags, hfPath, statePath, opts PackageDispatchOpts) error
func AutoRecordRemove(ctx, runner PackageInstaller, pkgs, cfg, flags, hfPath, statePath, opts PackageDispatchOpts) error
```

Every step of the install / remove flow is captured: dry-run short-circuit → single-writer lock → iterate + `runner.Install` → load hamsfile + state → append records → write both. A new package-like provider becomes "write an extractor + wire the dispatcher + declare the verb strings".

### Adoption status

Both branches shipped the scaffolding without migrating a single existing provider — callers in the builtin directory still use the pre-helper code. Both `tasks.md` files track the migration as a follow-up cycle.

### Assessment

- **`dev`'s helper is safer to adopt.** It has one responsibility (hamsfile path resolution) and works for every provider shape — CLI-first *and* hooks-based, package *and* non-package. The 80 LoC is a crisp contract that future-you can migrate in a single atomic commit per provider.
- **`local/loop`'s helper has bigger upside but a narrower audience.** The dispatcher fits apt / brew / cargo / goinstall / npm / pnpm / uv / mas / vscodeext (9 providers) but not `bash` (script runner), `git-config` (key/value), `git-clone` (URN-based), `defaults` (key/value), `duti` (UTI mapping), or `ansible` (playbook). Roughly half the current builtins would not use it.
- **Neither is wrong, and they are compatible.** A follow-up cycle could ship both: use `baseprovider` for path/config helpers that every provider wants, and use `package_dispatcher` only in the package-manager-shaped providers. `local/loop`'s implementation is the right shape for that layer — just not a replacement for `dev`'s narrower helper.

---

## 7. CI / Taskfile ergonomics

This is the starkest difference and it is `local/loop`-only.

### `local/loop` changes

- `.github/workflows/ci.yml`: three `actions/upload-artifact@v4` steps gain `if: ${{ !env.ACT }}`; two downstream jobs (`integration`, `e2e`, `itest`) get a new "act fallback" step that rebuilds the binary via `task build:linux` when running under act.
- `Taskfile.yml`:
  - `test:e2e` / `test:e2e:one` / `test:integration` / `test:sudo` / `test:itest` now call the `ci:*` tasks directly via docker, *not* through act.
  - `:one-via-act` variants are registered as the opt-in simulation.
  - Long comment blocks document *why*: act's builtin artifact-server emits `ECONNRESET` on `upload-artifact@v4`'s final PUT.

### Why this matters

Before the change, `task test:e2e` required act and failed unreliably on any developer who had a slightly different act / runner-image combination. After the change, `task test:e2e` is docker-only and runs identically on a dev laptop and in real CI. The act simulation is still there for anyone who needs it — just not on the critical path.

`dev` does not touch the CI configuration. On the `dev` branch, `task test:e2e` / `task test:integration` still route through act with the same artifact-server pain. In practice this means `local/loop`'s developer experience for verifying changes is *materially better*.

### Assessment

This is the single biggest quality win on `local/loop` that is invisible from reading the feature commits alone. It should be cherry-picked to `dev` regardless of which implementation wins on the feature work, because it affects every subsequent developer running the test suite locally.

---

## 8. Integration test log assertions

Both branches add helpers to verify "hams emits logs in integration tests". The helper signatures and the fan-out diverge.

### `dev` — file-based assertions (1 provider wired)

Helpers in `e2e/base/lib/assertions.sh`:

```bash
assert_log_contains "<description>" "<expected-substring>"      # reads $HAMS_DATA_HOME/hams.*.log
assert_log_records_session "<description>"                       # verifies "hams session started" line fires
```

Wired into: `internal/provider/builtin/apt/integration/integration.sh` (one provider, as canonical example). `tasks.md` §9.3 tracks fan-out to the other providers as follow-up.

The assertion reads from the rolling log *file*, which means the test transitively verifies that the slog handler is actually persisting records (not just emitting to stderr). Stronger signal, but couples the assertion to the file layout.

### `local/loop` — stderr-based assertions (3 providers wired)

Helpers:

```bash
assert_stderr_contains "<description>" "<expected-substring>" cmd arg1…  # captures stderr from a command
assert_log_line "<provider>" "<expected-substring>" cmd arg1…            # thin wrapper
```

Wired into: `bash`, `ansible`, `git` integration scripts (three providers). Each checks that both `"hams session started"` + the provider-tagged slog line appear on stderr.

The assertion runs the command and captures stderr directly — no file-path coupling, faster feedback on failure. Does not verify file persistence.

### Assessment

- `dev`'s assertion is a *stronger* test (verifies slog → file handoff) but is only wired to one provider.
- `local/loop`'s assertion is a *weaker* test (only verifies stderr) but is wired to three providers.

Strong + widespread > strong + narrow > weak + widespread. The ideal is `dev`'s helper wired to `local/loop`'s fan-out. Either branch can reach that by absorbing the other's work — no incompatibility between the two approaches.

---

## 9. Provider `--only` filter bug (dev-only fix)

`f6c063d` on `dev` fixes a real CI failure from the provider rename: `standard_cli_flow code install …` works because the CLI verb is `code`, but the same helper then calls `hams apply --only=code`, which fails because `--only` filters by `Manifest().Name` — which is still `code-ext`. The fix:

```bash
# e2e/base/lib/provider_flow.sh
local manifest_name="${MANIFEST_NAME:-$provider}"
# …
hams --store="$HAMS_STORE" apply --only="$manifest_name"
```

Plus `internal/provider/builtin/vscodeext/integration/integration.sh` now exports `MANIFEST_NAME=code-ext`. Surgical, well-reasoned fix with a clear commit message.

`local/loop` doesn't have this commit because its unified handler wires the CLI-verb rename differently, but the underlying issue — CLI verb vs. manifest name divergence — is still there. Worth verifying post-merge that `local/loop`'s integration tests actually exercise the `apply --only=code-ext` path.

---

## 10. Quality dimensions summary

| Dimension | Winner | Why |
|---|---|---|
| Code hygiene / package layering | `dev` | Separate `storeinit` and `baseprovider` packages; bootstrap concerns do not leak into CLI |
| UX completeness (auto-init) | `local/loop` | Dry-run short-circuit, context timeout, identity seeding |
| Conformance to spec ("wrapped cmds behave like originals") | `local/loop` | `hams git <anything>` passes through; `dev` errors |
| `--tag` / `--profile` safety | `local/loop` | Conflict detection via `ResolveCLITagOverride` |
| i18n design (typed keys) | `local/loop` | Compile-time-checked keys catalog |
| i18n coverage in git dispatcher | `dev` | `local/loop` ships raw English in the new git code |
| Shared provider helpers (breadth) | `local/loop` | `AutoRecordInstall` captures the full flow |
| Shared provider helpers (universality) | `dev` | `baseprovider` works for every provider shape |
| CI / Taskfile ergonomics | `local/loop` | Act is opt-in; docker-only path works on every dev box |
| Integration log assertions (strength) | `dev` | Tests file persistence, not just stderr |
| Integration log assertions (fan-out) | `local/loop` | 3 providers vs. 1 |
| Commit granularity | `local/loop` | 16 small commits vs. 9 larger commits; easier to review |
| OpenSpec decomposition | `local/loop` | 4 focused changes vs. 1 bundled |
| Deferred scope recorded in tasks.md | `dev` | More explicit punch list for follow-up fan-out |
| Go-git fallback (airgap support) | `dev` | Matches the CLAUDE.md design constraint |
| Commit authorship style | Tie | Both Co-Authored-By Claude with detailed why-bodies |

---

## 11. Merge recommendation

Neither branch is strictly better. A pragmatic path forward:

1. Take `local/loop` as the base and cherry-pick:
   - `dev`'s `internal/storeinit` package (in place of `scaffold.go`) — cleaner layering.
   - `dev`'s go-git fallback.
   - `dev`'s i18n rollout inside the unified git dispatcher (replace the raw English UFEs in `local/loop`'s `unified.go` with `i18n.T` calls using `local/loop`'s typed keys).
   - `dev`'s `f6c063d` `--only` filter fix.
   - `dev`'s `baseprovider` package (orthogonal to `local/loop`'s `package_dispatcher`, both should ship).

2. From `local/loop`, keep:
   - Typed `internal/i18n/keys.go` catalog.
   - `config.ResolveCLITagOverride` + separate `flags.Tag`.
   - `hams git` passthrough + dry-run preview.
   - Scaffold's dry-run short-circuit, context timeout, and identity-seeding.
   - All CI / Taskfile changes (act → opt-in, artifact upload guards).
   - `package_dispatcher` alongside `baseprovider`.
   - The three already-fanned-out log assertions + `dev`'s file-based assertion helper.

3. Keep the OpenSpec change decomposition from `local/loop` — the four focused changes are easier to archive and reference than `dev`'s bundled one.

If cherry-picking is too expensive, merge `local/loop` as-is and file follow-up tasks for the items above where `dev` is better. The gap is not large; `dev`'s deferred scope already captures most of the same follow-up list.

---

## Appendix: file-level reference

| Area | `dev` file(s) | `local/loop` file(s) |
|---|---|---|
| Auto-init scaffold | `internal/storeinit/{storeinit,doc,template/*}` + `internal/cli/autoinit.go` | `internal/cli/scaffold.go` + `internal/cli/template/store/*` |
| Unified `hams git` | `internal/provider/builtin/git/unified.go` (81 LoC) | `internal/provider/builtin/git/unified.go` (221 LoC) |
| `hams code` | `internal/provider/builtin/vscodeext/code_handler.go` (44 LoC, new file) | In-place edits to `internal/provider/builtin/vscodeext/vscodeext.go` + renamed tests |
| `--tag` wiring | `internal/cli/root.go` alias | `internal/config/resolve.go` + `internal/cli/apply.go` + `internal/cli/provider_cmd.go` |
| i18n infra | `internal/i18n/i18n.go` lazy-init + `locales/{en,zh-CN}.yaml` | Same + `internal/i18n/keys.go` typed constants |
| Shared provider helpers | `internal/provider/baseprovider/baseprovider.go` (hamsfile only) | `internal/provider/package_dispatcher.go` (install/remove flow) + `internal/provider/flags.go` (split out) |
| Integration log assertions | `e2e/base/lib/assertions.sh` + `apt/integration/integration.sh` | `e2e/base/lib/assertions.sh` + `{bash,ansible,git}/integration/integration.sh` |
| CI / Taskfile | unchanged from base | `.github/workflows/ci.yml` + `Taskfile.yml` + `.golangci.yml` |
| OpenSpec | `2026-04-17-onboarding-auto-init/` + 5 archived | `2026-04-17-{apply-tag-and-auto-init, cli-i18n-catalog, hams-git-and-code-consolidation, integration-logging-assertions}/` + 5 archived |

Last verified 2026-04-18 against `dev@f6c063d` and `origin/local/loop@66266ff`.
