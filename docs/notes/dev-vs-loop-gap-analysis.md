# `dev` vs `local/loop` — Gap Analysis Against Specs

> **Status (2026-04-19):** all gaps identified in this document have been
> remediated on `local/loop` via 17 atomic commits landing between
> `ac135d9` and `f44ea1f`. The hybrid plan at the bottom of this document
> was applied in full plus additional hardening: see
> `openspec/changes/archive/2026-04-18-storeinit-package-with-gogit-fallback/`,
> `…-i18n-builtin-provider-catalog/`, and
> `…-provider-shared-abstraction-adoption/` for the shipped spec deltas.
> Remaining open items are architectural follow-ups (`BatchPackageInstaller`
> for apt, `FlaggedPackageInstaller` for homebrew) tracked as 5.7 / 5.8
> inside the provider-shared-abstraction archived change.
>
Scope: compare `origin/dev @ f6c063d` and `local/loop @ 66266ff` (both diverged
from base `d582756`), judge each dimension against the **authoritative
specs** (`openspec/specs/**`) and `CLAUDE.md`, identify which branch
implementation is better, which is more complete, and what both branches
must still fix.
>
> Companion to `docs/notes/dev-vs-loop-branch-analysis.md`, which catalogues
> the differences. This document converts the catalogue into a prescriptive
> action list for the current branch.

## Verdict Table

Legend:

- **dev ✓** — dev's implementation matches the spec, loop's does not.
- **loop ✓** — loop's implementation matches the spec, dev's does not.
- **both ✗** — both violate the spec.
- **both ✓** — both pass the spec; only stylistic differences remain.

| # | Capability | Authoritative Spec | Winner | Where in dev | Where in loop |
|---|------------|--------------------|--------|--------------|---------------|
| 1 | go-git fallback for auto-init | `project-structure/spec.md:686-699` | **dev ✓** | `internal/storeinit/storeinit.go:94-107` | `internal/cli/scaffold.go:36-41` (missing fallback) |
| 2 | `hams git <anything>` passthrough | `builtin-providers/spec.md:69` | **loop ✓** | `internal/provider/builtin/git/unified.go:55-80` (rejects) | `internal/provider/builtin/git/unified.go:94-207` (passthrough) |
| 3 | First-run non-interactive seeding of `profile_tag` + `machine_id` | `cli-architecture/spec.md:654` | **loop ✓** | `internal/cli/autoinit.go:93-139` (separate path) | `internal/cli/scaffold.go:149-181` (seeds in scaffold) |
| 4 | DI-injected writer (no global `os.Stdout` mutation under `t.Parallel()`) | `code-standards/spec.md` (DI boundary rule) + `provider-system/spec.md` | **loop ✓** | n/a (global swap still in homebrew tests) | `internal/provider/flags.go:20-41` |
| 5 | Local/CI isomorphism (`act` parity) | `CLAUDE.md` → *Development Process Principles* | **loop ✓** | unchanged | `.github/workflows/ci.yml:69-233` |
| 6 | OpenSpec archival discipline | `CLAUDE.md` → *OpenSpec / Core Principles* | **loop ✓** | `openspec/changes/2026-04-17-onboarding-auto-init/` (in-flight) | `openspec/changes/archive/2026-04-17-*` (all archived) |
| 7 | Typed i18n key registry | `cli-architecture/spec.md:103-116` + good practice | **loop ✓** | string-literal keys in `autoinit.go` | `internal/i18n/keys.go:21-85` |
| 8 | Config-tag override pure functions | `CLAUDE.md` → *DI boundary isolation* | **loop ✓** | inlined in `cli/autoinit.go` | `internal/config/resolve.go:22-97` |
| 9 | Auto-init package boundary | `CLAUDE.md` → *package hygiene* (no explicit spec) | **dev ✓** | `internal/storeinit/` top-level package | `internal/cli/scaffold.go` co-located |
| 10 | i18n coverage across all user-facing strings | `cli-architecture/spec.md:105` | **both ✗** | 4 call sites, CLI layer only, 0 in builtin providers | 24 call sites total, 0 in builtin providers |
| 11 | Shared provider abstraction actually adopted | `CLAUDE.md` → *Current Tasks* ("design shared abstractions … extending with a new provider is a matter of filling in a well-defined template") | **both ✗** | `internal/provider/baseprovider/` — 0 adopters | `internal/provider/package_dispatcher.go` — 0 adopters |
| 12 | `code-ext` → `code` full rename (manifest, on-disk, logs) | `CLAUDE.md` → *Current Tasks* ("The code-ext provider likewise should expose only the `hams code` entry point") | **loop ✓** (partial) | wrapper only, `Manifest.Name="code-ext"` unchanged | manifest renamed; 20 sites updated; old on-disk `vscodeext.hams.yaml` not migrated |
| 13 | Unified `hams git` = ConfigProvider + CloneProvider | `builtin-providers/spec.md:1788-1907` | both deliver, **loop ✓** on passthrough (see row 2) | 81-line strict subset | 221-line passthrough dispatcher |
| 14 | Integration tests assert logs are emitted | `CLAUDE.md` → *Current Tasks* | both **✓** | apt/integration.sh + assertions.sh extended | ansible/bash/git/vscodeext/integration.sh + assertions.sh + provider_flow.sh extended |
| 15 | Documentation in sync (en + zh-CN) | `CLAUDE.md` → *Documentation i18n Sync* | both **✓** | — | — |

## 1. Per-capability reasoning

### 1.1 go-git fallback — **dev ✓**

`openspec/specs/project-structure/spec.md:686-699` is an explicit SHALL:

> "The hams binary SHALL bundle go-git as a compiled-in dependency for Git
> operations. … The go-git dependency SHALL be used as a fallback when
> system `git` is not available."

- `dev`, `internal/storeinit/storeinit.go:94-107`:

  ```go
  if gitBin, err := exec.LookPath("git"); err == nil {
      cmd := exec.CommandContext(ctx, gitBin, "init", "--quiet", dir)
      …
      return nil
  }
  if _, err := gogit.PlainInit(dir, false); err != nil {
      return fmt.Errorf("go-git PlainInit failed: %w", err)
  }
  ```

- `loop`, `internal/cli/scaffold.go:36-41`:

  ```go
  var gitInitExec = func(ctx context.Context, dir string) error {
      cmd := exec.CommandContext(ctx, "git", "init", "--quiet", dir)
      …
      return cmd.Run()
  }
  ```

  There is no `go-git` branch — a machine without `git` on `PATH` fails the
  auto-scaffold step, which is precisely the scenario the spec exists to cover.

**Loop must port dev's go-git fallback.**

### 1.2 `hams git <anything>` passthrough — **loop ✓**

`openspec/specs/builtin-providers/spec.md:69`:

> "CLI wrapping: Provider recognizes `install`, `remove`, and `list`
> subcommands. **All other subcommands are passthrough to the underlying CLI.**"

- `loop`, `internal/provider/builtin/git/unified.go:94-207`:

  ```go
  switch args[0] {
  case "config": return p.handleConfig(...)
  case "clone":  return p.handleClone(...)
  default:       return p.passthrough(ctx, args, flags)   // ← shells out to real git
  }
  ```

- `dev`, `internal/provider/builtin/git/unified.go:55-80`:

  ```go
  default:
      return hamserr.NewUserError(hamserr.ExitUsageError,
          i18n.Tf("git.unknown_subcommand", …),
          …,
      )
  ```

  `hams git status`, `hams git pull`, `hams git push` **fail** on `dev`, so
  the unified `hams git` is unusable as a drop-in for the real `git`.

Loop keeps this. Dev needs the passthrough behaviour; loop does not.

### 1.3 First-run non-interactive seeding — **loop ✓**

`openspec/specs/cli-architecture/spec.md:654`:

> "… AND SHALL seed `profile_tag: macOS` + `machine_id: <hostname-or-$HAMS_MACHINE_ID>`
> into the global config on a fresh machine without prompting, making the
> apply fully non-interactive."

- `loop`, `internal/cli/scaffold.go:149-181`:

  ```go
  seedIfMissing(paths, "profile_tag", func() string { return config.DefaultProfileTag })
  seedIfMissing(paths, "machine_id", config.DeriveMachineID)
  ```

  Both identity fields are populated inside the scaffold call itself, so the
  very next provider invocation is silent.

- `dev`, `internal/cli/autoinit.go:93-139`: only `EnsureGlobalConfig`
  writes `tag: default` and `machine_id: <hostname>`; `store_path` is
  written in `EnsureStoreReady`, but `profile_tag` and `machine_id` are not
  re-seeded when the scaffold path creates a new store — the user still
  sees the legacy nudge if the global config happens to exist but lacks
  these keys.

### 1.4 DI-injected writer (no global `os.Stdout` mutation) — **loop ✓**

`openspec/specs/code-standards/spec.md` mandates DI for all boundaries;
`provider-system/spec.md` requires DI-friendly interfaces. `loop`'s fix is
`internal/provider/flags.go:20-41`:

```go
Out io.Writer
Err io.Writer
func (f *GlobalFlags) Stdout() io.Writer { … }
func (f *GlobalFlags) Stderr() io.Writer { … }
```

Homebrew's `handleList` / `handleTap` / `handleInstall` / `handleRemove`
rewrite to `fmt.Fprintf(flags.Stdout(), …)`, eliminating the
`captureStdoutForHomebrew` global-swap pattern that tripped `-race`
under `t.Parallel()`.

Dev still uses the global-stdout swap in homebrew tests. Even if `task
check` currently passes, the race is latent.

### 1.5 Local/CI isomorphism — **loop ✓**

`CLAUDE.md` explicitly lists "Local/CI isomorphism" among the non-negotiable
Development Process Principles. `loop`'s `.github/workflows/ci.yml:69-233`
adds `if: ${{ !env.ACT }}` guards around `upload-artifact@v4` /
`download-artifact@v4` steps, plus a `setup-go@v5` + `task build:linux`
ACT fallback that reproduces the artifact handoff locally.

Dev does not touch CI. `act test:e2e:one TARGET=debian-amd64` on `dev`
today diverges from GitHub Actions; on `loop` it matches.

### 1.6 OpenSpec archival discipline — **loop ✓**

- `dev`: 5 new archived changes, **1 in-flight** (`onboarding-auto-init`).
- `loop`: 9 new archived changes, **0 in-flight**.

`CLAUDE.md` says "Specs lag reality intentionally — they reflect shipped
behavior". If the behaviour is shipped, the change must be archived.
`loop` honours this; `dev` does not.

### 1.7 Typed i18n key registry — **loop ✓**

Not a hard SHALL in the spec, but `cli-architecture/spec.md:103-116`
mandates "a message catalog interface that all user-facing strings go
through". Typed constants in `internal/i18n/keys.go:21-85` make the
catalogue enumerable and compile-time-checked; string literals do not.

### 1.8 Config-tag override pure functions — **loop ✓**

`loop`'s `internal/config/resolve.go:22-97` exposes
`ResolveCLITagOverride`, `ResolveActiveTag`, `DeriveMachineID` — pure,
unit-tested, reusable across `apply`, `refresh`, `config`, etc. `dev`
hides the same logic inside `cli/autoinit.go` where only `apply` can
call it.

`CLAUDE.md` → *DI boundary isolation principle* prefers pure-function
factoring; loop matches.

### 1.9 Auto-init package boundary — **dev ✓**

No explicit spec requirement; a `CLAUDE.md` project-structure convention.
`dev` puts everything in `internal/storeinit/` (`doc.go`, `storeinit.go`,
`storeinit_test.go`, `template/`). `loop` keeps everything inside
`internal/cli/`, so the scaffolder is coupled to the CLI package.

Dev's package boundary is strictly cleaner. Loop should adopt it.

### 1.10 i18n coverage across all user-facing strings — **both ✗**

`cli-architecture/spec.md:105`:

> "The i18n module SHALL provide a message catalog interface that **all**
> user-facing strings (errors, help text, prompts) go through."

Current state on `local/loop` HEAD (66266ff):

```text
$ rg -l 'i18n\.' internal/ --type go -g '!*_test.go'
internal/cli/apply.go
internal/cli/commands.go
internal/cli/root.go
internal/config/resolve.go
internal/i18n/i18n.go
internal/i18n/keys.go

$ rg -n 'i18n\.' internal/provider/builtin/ -g '!*_test.go'
(no matches)
```

Zero builtin providers use `i18n.T`. Every `hamserr.NewUserError(...)`
invocation, every dry-run line, every `fmt.Fprintf(flags.Stdout(), ...)`
with English prose — all hardcoded. `dev` is in the same state (4 call
sites, all in `cli/`).

**Both branches violate the SHALL.** The fix touches every provider and
is not a pick-one-from-the-other port; both need new code.

### 1.11 Shared provider abstraction actually adopted — **both ✗**

`CLAUDE.md` → *Current Tasks*:

> "All providers follow the same pattern: parse the original command
> structure, extract what needs to be recorded, then pass the remainder
> through to the underlying command for execution. … design shared
> abstractions — either a single generic base or a few categorical base
> types — so that extending with a new provider is a matter of filling
> in a well-defined template, not reimplementing the pattern from
> scratch."

- `dev` shipped `internal/provider/baseprovider/` (80 LoC, helpers for
  `LoadOrCreateHamsfile` / `HamsfilePath` / `EffectiveConfig`).
  **Zero builtin providers call into it.**
- `loop` shipped `internal/provider/package_dispatcher.go` (190 LoC,
  `AutoRecordInstall` / `AutoRecordRemove`). **Zero builtin providers
  call into it.**

Both branches added framework-shaped code and did not migrate a single
existing provider onto it. The task says "extending with a new provider
is a matter of filling in a well-defined template"; until at least one
existing provider uses the template, we do not know whether the template
is correct.

**Both branches fail the spirit of the task.** The fix: migrate at least
the package-like providers (`apt`, `brew`, `pnpm`, `npm`, `cargo`,
`goinstall`, `uv`, `mas`, `vscodeext`) onto `package_dispatcher` (or
`baseprovider`, but `package_dispatcher` consolidates more).

### 1.12 `code-ext` → `code` full rename — **loop ✓** (partial)

`CLAUDE.md` → *Current Tasks*:

> "The code-ext provider likewise should expose only the `hams code`
> entry point."

- `dev`, `internal/provider/builtin/vscodeext/code_handler.go:32-43`:
  a 44-line wrapper that projects the name `"code"` onto a Provider
  whose `Manifest.Name` is still `"code-ext"` and whose `FilePrefix` is
  still `"vscodeext"`. Internal error strings, state lock names, log
  labels all read `"code-ext"`.
- `loop`, `internal/provider/builtin/vscodeext/vscodeext.go`:
  `cliName = "code"`, `filePrefix = "code"`, every error and lock label
  reads `"code"`. No wrapper needed.

Since hams is pre-v1 and no existing `vscodeext.hams.yaml` files need
preserving (this is a brand-new workflow), **loop's in-place rename is
the correct interpretation of the task.** Dev's wrapper leaves a
dangling `code-ext` identity in logs and state — partially done.

One residual in loop: the Manifest auto-scaffold still wires the legacy
`BootstrapEntries` shape but this is not observable to end-users today;
tracking under the shared-abstraction task below.

## 2. Recommendations for `local/loop` — ranked by spec leverage

Rank = priority × ease. Higher = do first.

| Rank | Task | Rationale | Difficulty |
|------|------|-----------|-----------|
| 1 | Port dev's `go-git` fallback into `loop`'s auto-scaffold path | Closes a hard `SHALL` violation (project-structure spec) | S |
| 2 | Wire i18n through every builtin provider's user-facing strings | Closes the `cli-architecture` SHALL; today loop has 0 i18n.T calls in any provider | L |
| 3 | Migrate at least the 9 package-like providers onto `package_dispatcher` | Closes the CLAUDE.md "shared abstraction" task in practice; removes the dead-code smell | L |
| 4 | Extract loop's auto-scaffold into a dedicated `internal/storeinit/` package with a `doc.go`, mirroring dev | Cleaner package boundary; enables #1 without bloating `internal/cli/` | M |
| 5 | Expand `i18n/keys.go` to name every new i18n site added by #2 | Keeps loop's existing typed-key discipline honest | M |
| 6 | Write an OpenSpec change proposal per remediation (1, 2+5, 3, 4) with tasks.md + spec delta, verify, archive | CLAUDE.md OpenSpec discipline; every ship needs a spec delta | M |
| 7 | Provider-level integration tests: assert each provider's NON-english output changes when `LANG=zh_CN.UTF-8` is set | Prevents regression once #2 lands; proves user-facing strings really go through i18n.T | M |
| 8 | Verify `hams git` passthrough against real-world verbs (`status`, `pull`, `push`, `log`) in integration tests | Loop's dispatcher already passes through; the test asserts the SHALL stays true if refactored | S |
| 9 | Migrate `internal/provider/baseprovider/` into `package_dispatcher`'s package so there is only one "shared base" abstraction, not two | YAGNI; fewer APIs to keep in sync | S |
| 10 | Add cross-reference: CLAUDE.md → `docs/notes/` → `openspec/specs/` so new contributors see the chain | One-line edits; improves onboarding | XS |

## 3. Things `dev` does better that `loop` should adopt unchanged

1. `internal/storeinit/` package with `//go:embed template/*` — loop's
   `internal/cli/template/store/` works but bloats `internal/cli`.
2. `storeinit.Bootstrapped()` as a separate check-function — loop inlines
   `storeDirExists()` in scaffold.
3. `doc.go` package doc — loop has no package-level doc for its scaffold
   module.

## 4. Things `loop` already does better — keep as-is

1. `hams git` passthrough (`internal/provider/builtin/git/unified.go`).
2. Typed `i18n.keys` registry (`internal/i18n/keys.go`).
3. `internal/config/resolve.go` pure helpers.
4. `provider.GlobalFlags.Out` / `Err` writer seam.
5. ACT-conditional CI workflow.
6. Homebrew `-race` fix via writer seam.
7. First-run silent `seedIfMissing` in scaffold.
8. `code-ext` → `code` in-place manifest rename.
9. All OpenSpec changes archived — no in-flight residue.
10. Extended `.golangci.yml` `errcheck.exclude-functions`.

## 5. Common failures both branches must repair

| # | Failure | Remediation |
|---|---------|-------------|
| A | i18n coverage in builtin providers is **zero** | Route every `hamserr.NewUserError` / user-facing `fmt.Fprintf` through `i18n.T` / `i18n.Tf`; add keys under `provider.<name>.*` prefix |
| B | Shared abstraction (`baseprovider` / `package_dispatcher`) is not adopted by a single existing provider | Migrate package-like providers (`apt`, `brew`, `pnpm`, `npm`, `cargo`, `goinstall`, `uv`, `mas`, `vscodeext`) onto a single shared helper; delete the unused one |
| C | `code-ext` lingering identity in places neither branch cleaned up (state/lock names for legacy stores) — not a pre-v1 blocker, but should be verified | Integration test asserting `hams code install ext.ext` produces `code.hams.yaml`, `code.state.yaml`, and logs labelled `code` (no `code-ext`) |
| D | Neither branch added a spec requirement that future providers MUST use the shared base | Add SHALL to `provider-system/spec.md`: "New Package-class providers SHALL route install/remove/list through `provider.AutoRecordInstall` / `AutoRecordRemove` unless they document why in their spec delta." |
| E | Neither branch wrote a spec delta formalising that `hams apply` auto-init is non-interactive when `--tag` is supplied | Ship as part of the `apply-tag-and-auto-init` archived change's spec or a new focused delta |

## 6. Suggested commit plan (for the follow-up remediation branch)

Each commit below should be atomic, pass `task check`, and close one
spec-citable gap.

1. `feat(storeinit): extract scaffold into dedicated package + add go-git fallback` — covers ranks 1 & 4 together.
2. `feat(i18n): route builtin-provider user-facing strings through i18n.T` — covers ranks 2 & 5 (plus per-provider keys).
3. `feat(provider): adopt package_dispatcher in apt (proof of abstraction)` — smallest migration first.
4. `feat(provider): migrate brew/pnpm/npm to package_dispatcher` — batch migrations.
5. `feat(provider): migrate cargo/goinstall/uv/mas/vscodeext to package_dispatcher` — finish the batch.
6. `refactor(provider): delete baseprovider now that package_dispatcher is the single shared base` — cleanup.
7. `test(i18n): assert builtin providers honour LANG=zh_CN.UTF-8` — prevents regression.
8. `test(integration): assert hams git passthrough for status/pull/push/log` — pins current behaviour.
9. `docs(openspec): spec delta + tasks.md + archive per remediation` — CLAUDE.md discipline.
10. `docs(notes): cross-reference specs ↔ notes ↔ CLAUDE.md` — onboarding.

Each commit must compile cleanly (`go build ./...`), lint cleanly (`task
check`), and — for provider-behaviour commits — ship an integration test
the same commit.
