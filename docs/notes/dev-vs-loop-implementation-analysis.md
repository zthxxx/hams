# `origin/dev` vs `local/loop` — Implementation Analysis

**Date:** 2026-04-19
**Base commit:** `d582756` (shared ancestor)
**Branches analyzed:**

| | Tip | Commits on top of base | Scope |
|---|---|---|---|
| `local/loop` (Branch A) | `7ea103b` | 38 | Current working branch |
| `origin/dev` (Branch B) | `924fec5` | 39 | Reference remote branch |

Both branches start from the same post-review commit, implement the same set
of V1 gap-closure features, and both pass `task check` (fmt + lint + unit +
integration + e2e, race detector on). They diverged **after the review
findings were captured** and took independent paths to the same product goals.

> Scope note: this report deliberately skips `docs/notes/` because both
> branches use it for in-session scratch space, not shipped artifacts.

## 1. Executive Summary

The two branches are functional twins at the shipping level — same providers,
same CLI surface, same YAML schemas, same test pipelines — but they make
**very different architectural bets** under the hood.

- **`origin/dev` (B)** chose **breadth, layering and stricter type safety**.
  It introduces a dedicated `baseprovider` helper package, a separate
  `Passthrough` primitive, a richer typed-i18n catalog with ~2× the
  translated strings, and a dedicated `autoinit.go` module. It has **11
  openspec changes still in-flight** — a sign of a feature-accumulating
  trajectory.
- **`local/loop` (A)** chose **closure, spec discipline and compact code**.
  It exposes `AutoRecordInstall` + a closure variant `AutoRecordInstallFn`
  so the last exempt provider (Homebrew) joins the shared dispatcher; it
  ships a standalone `storeinit` package with an E0 integration test that
  actually exercises the go-git fallback on a git-less container; it fully
  archives every change it touched (0 in-flight vs B's 11).

Neither branch is strictly better. If we were shipping one repository and
had to pick, `origin/dev` has the **stronger long-range infrastructure**
(baseprovider, passthrough, full i18n fanout, autoinit separation) while
`local/loop` has the **stronger finishing touches** (dispatcher closure
variants, E0 fallback test, WarnIfDefaultsUsed extraction, full archival).

The rest of this report maps these tradeoffs concretely, on a per-capability
basis, so `local/loop` can absorb the parts where `origin/dev` is ahead
without giving up what it already did better.

## 2. Diff Magnitude Overview

Excluding `docs/notes/`, against base `d582756`:

| Metric | `local/loop` | `origin/dev` |
|---|---|---|
| Commits | 38 | 39 |
| Files touched | ~140 | ~180 |
| Insertions | 7 161 | 8 345 |
| Deletions | 1 860 | 1 670 |
| In-flight openspec changes | 0 | 11 |
| Archived openspec changes added | 12 | 7 |
| i18n keys (en.yaml) | 119 | 176 |
| `i18n.T`/`i18n.Tf` call sites in `internal/` | 152 | 305 |

The most notable structural fact: **`origin/dev` adds more new files,
including net-new packages** (`baseprovider/`, `passthrough.go`, dedicated
`autoinit.go`), while **`local/loop` is denser inside existing files** but
adds the single new `storeinit/` package and two closure-variant dispatcher
functions.

## 3. Shared Provider Abstraction

Both branches solve the same problem: install/remove flows across the
package-class providers (apt, brew, cargo, goinstall, mas, npm, pnpm, uv,
vscodeext) share identical lock → run → reload → record → save logic and
should not be copy-pasted per provider.

### 3.1 API surface

**`local/loop`** — `internal/provider/package_dispatcher.go`

```go
type PackageInstaller interface {
    Install(ctx context.Context, pkg string) error
    Uninstall(ctx context.Context, pkg string) error
}

func AutoRecordInstall  (ctx, runner PackageInstaller, pkgs, cfg, flags, paths, opts) error   // line 74
func AutoRecordInstallFn(ctx, installFn func(ctx, pkg) error, pkgs, cfg, flags, paths, opts)  // line 144
func AutoRecordRemoveFn (ctx, uninstallFn func(ctx, pkg) error, pkgs, cfg, flags, paths, opts)// line 200
func AutoRecordRemove   (ctx, runner PackageInstaller, pkgs, cfg, flags, paths, opts) error   // line 259
```

Four entry points: two interface-based and two closure-based. The closure
variants were added specifically to lift the last exemption (Homebrew, whose
runner carries an `isCask bool` that the plain interface can't express).

**`origin/dev`** — same file path, simpler surface

```go
func AutoRecordInstall(ctx, runner PackageInstaller, pkgs, ...) error  // line 77
func AutoRecordRemove (ctx, runner PackageInstaller, pkgs, ...) error  // line 148
```

Only two entry points. Any provider whose install signature doesn't match
the two-method `PackageInstaller` interface hand-rolls the whole lock →
record dance.

### 3.2 Actual adoption (count of real call sites)

| Provider | `local/loop` | `origin/dev` |
|---|---|---|
| cargo | `AutoRecordInstall` / `Remove` | hand-rolled inline |
| npm | `AutoRecordInstall` / `Remove` | hand-rolled inline |
| pnpm | `AutoRecordInstall` / `Remove` | hand-rolled inline |
| uv | `AutoRecordInstall` / `Remove` | hand-rolled inline |
| mas | `AutoRecordInstall` / `Remove` | hand-rolled inline |
| goinstall | `AutoRecordInstall` / `Remove` (with no-op `Uninstall`) | hand-rolled inline |
| vscodeext | `AutoRecordInstall` / `Remove` | hand-rolled inline |
| homebrew | `AutoRecordInstallFn` / `RemoveFn` (closure) | hand-rolled inline |
| apt | **documented exemption** (batch install shape) | not documented |

Effective adoption count: **8 of 9 package-class providers on
`local/loop`; 0 of 9 on `origin/dev`**. On `origin/dev`, the dispatcher
functions exist but no provider calls them — each provider inlines the
same 40–60 line lock/load/record/save sequence. That is a textbook DRY
regression masquerading as an abstraction.

To make this concrete: `cargo.go` is **235 lines on `local/loop` vs 317
lines on `origin/dev`** (+82 lines for one provider), entirely because
`origin/dev` inlines the flow that `local/loop` delegates.

### 3.3 The `baseprovider` package — `origin/dev` only

`origin/dev` adds `internal/provider/baseprovider/baseprovider.go` with:

- `EffectiveConfig(cfg, flags)` — collapses `--store` / `--profile`
  overrides onto the in-memory config
- `HamsfilePath(cfg, filePrefix, hamsFlags, flags)` — single place
  that computes Hamsfile path (regular + local)
- `LoadOrCreateHamsfile(...)` — same but loads or creates the YAML

These helpers are real and useful. `local/loop` keeps this logic inlined
in each provider (so each provider knows `--hams-tag`, `--hams-local`
resolution for itself), which is more duplication but zero coupling.

### 3.4 The `Passthrough` primitive — `origin/dev` only

`origin/dev`'s `internal/provider/passthrough.go`:

```go
var PassthroughExec = func(ctx, tool, args []string) error { ... }   // swappable for tests

func Passthrough(ctx, tool, args []string, flags *GlobalFlags) error {
    if flags != nil && flags.DryRun {
        fmt.Fprintln(flags.Stdout(), i18n.Tf(ProviderStatusDryRunRun, ...))
        return nil
    }
    return PassthroughExec(ctx, tool, args)
}
```

This is genuinely better than what `local/loop` has. On `local/loop`, the
equivalent is `provider.WrapExecPassthrough` in `wrap.go`, which **does
not honour `flags.DryRun`** and has no module-level DI seam.
`origin/dev` has a dedicated `passthrough_test.go` that swaps the seam
and asserts the recorded invocation — clean unit testing without
exec'ing real binaries.

### 3.5 Scoring

| Axis | Winner | Why |
|---|---|---|
| API surface clarity | dev | One path per direction (Install/Remove), no Fn-variant confusion |
| Practical adoption | **loop** | 8/9 adopters vs 0/9; Homebrew's closure variant actually lifts the last exemption |
| Boilerplate in providers | **loop** | cargo 235L vs 317L; same ratio across the other 7 providers |
| Exemption documentation | **loop** | Block comment at dispatcher explains why apt is exempt; dev's non-adoption is silent |
| Passthrough testability | dev | Module-level `PassthroughExec` seam, dedicated tests |
| Dry-run support on passthrough | dev | `Passthrough` honours `flags.DryRun`; `WrapExecPassthrough` does not |
| Reusable hamsfile/config helpers | dev | `baseprovider` package exists; loop inlines per-provider |

Net call: **`local/loop` wins the dispatcher story** (the one that was
specified as a SHALL in the provider-system spec), **`origin/dev` wins
the Passthrough and `baseprovider` sub-stories** (which `local/loop`
did not promise but would benefit from).

## 4. i18n Catalog

Both branches use `nicksnyder/go-i18n` with a `keys.go` that defines
typed string constants, `locales/en.yaml` and `locales/zh-CN.yaml`, and
`T()` / `Tf()` lookup helpers. The convention is the same:
`<capability>.<component>.<short-id>`.

### 4.1 Coverage

|  | `local/loop` | `origin/dev` |
|---|---|---|
| Keys in en.yaml | **119** | **176** (+48%) |
| Keys in zh-CN.yaml | 119 (locked by test) | 176 (locked by test) |
| `i18n.*` references in `internal/**.go` (non-test) | **152** | **305** (≈2×) |

Category breakdown — `origin/dev` is strictly a superset:

| Category | loop | dev |
|---|:-:|:-:|
| CLI framework errors (`cli.err.*`) | ~5 | ~28 |
| Auto-init lifecycle (`autoinit.*`) | 0 | 4 |
| User-facing errors family (`ufe.*`) | 0 | 6 |
| Apply / refresh status | 1 | ~30 |
| Store commands (`store.init/pull/commit`) | 7 (labels only) | 20 |
| Config set/unset/open | 0 | 15 |
| List command | 0 | 6 |
| Self-upgrade | 0 | 7 |
| Sudo prompt, TUI fallbacks | 0 | ~2 |
| Provider errors (`provider.err.*`) | ~60 | ~55 (consolidated) |
| Git dispatcher / passthrough | 8 | 10 + new dispatcher family |

`local/loop` focuses tightly on **provider errors and apply/refresh
status lines**. `origin/dev` fans out across the **entire CLI lifecycle**
including top-level commands, first-run onboarding, and self-upgrade.

### 4.2 Locale parity enforcement

**`local/loop`** — two dedicated tests:

1. `internal/i18n/locale_parity_test.go` — `TestLocalesAreInParity` dynamically
   walks `locales/*.yaml`, compares each non-English file to en.yaml, and
   fails with specific missing/extra key names.
2. `internal/i18n/i18n_providers_test.go` — `TestProviderKeysResolve{English,Chinese}`
   iterates every `Provider*` constant in `keys.go`, calls `Tf` with a
   realistic template-data map, and fails if the result is the raw key
   or empty.

**`origin/dev`** — one test:

- `internal/i18n/i18n_test.go` — `TestCatalogCoherence_EveryTypedKeyResolves`
  hand-maintains a slice of **every** exported constant (~176) and asserts
  `id: <key>` appears in both locale files.

Tradeoffs:

| | loop | dev |
|---|---|---|
| Catches "YAML key with no Go constant" (orphan) | ✅ (dynamic) | ❌ |
| Catches "Go constant with no YAML key" (missing) | ✅ for `Provider*` only | ✅ for every key (exhaustive list is maintained) |
| Asserts template interpolation actually produces a non-key string | ✅ | ❌ (only checks `id:` presence) |
| Requires manual upkeep of a test-side constant list | ❌ | ✅ |

The ideal is **both**: dev's exhaustive coverage plus loop's dynamic-parity
plus loop's template-interpolation check. Neither branch has all three,
but `local/loop` has the two sharper ones (parity and interpolation).

### 4.3 Lookup API and fallback

Both branches:

```go
func T(msgID string) string              // returns key on miss, never panics
func Tf(msgID string, data map[string]any) string
```

Both use `sync.Once` for bundle initialisation. `origin/dev` drops the
`if localizer == nil` early return (line 175 in its `i18n.go`) because
its `ensureLocalizer()` is guaranteed to initialise; `local/loop` keeps
a defensive nil check. Negligible difference in practice.

### 4.4 Scoring

| Axis | Winner | Note |
|---|---|---|
| Breadth / coverage | **dev** | Covers the full CLI lifecycle; loop stops at providers + apply |
| Locale parity | loop | Dynamic parity + orphan-key detection |
| Template-string regression catch | loop | `Tf` with realistic data vs string-marker match |
| Type-safety for non-provider keys | **dev** | Tests all 176 keys; loop only tests `Provider*` |
| Test maintenance cost | loop | Dynamic tests; dev requires adding new keys to the test slice |

The right move for `local/loop` is to **backport `origin/dev`'s
expanded key catalog** (48% more keys across CLI lifecycle) while
**keeping its own dynamic parity and interpolation tests**.

## 5. CLI UX: Auto-init, --tag, git passthrough, code rename

### 5.1 File layout in `internal/cli/`

|  | `local/loop` | `origin/dev` |
|---|---|---|
| `apply.go` | 64 697 B (1 559 lines) | 62 658 B (1 544 lines) |
| `commands.go` | 69 858 B | 71 590 B |
| `bootstrap.go` | 15 127 B | similar |
| `bootstrap_consent.go` + tests | present (both) | present (both) |
| Dedicated `autoinit.go` | ❌ (logic lives in apply.go / provider_cmd.go / commands.go) | ✅ (10 975 B + 11 dedicated tests) |
| Dedicated `errors.go` | present | present |

Conclusion: `origin/dev` does **not** shrink `apply.go`; it adds
`autoinit.go` as additional material. It earns **separation of
concerns** (auto-init lives in one file, is easily testable in
isolation) at a small cost in total lines. `local/loop` keeps the
logic closer to its callers — apply.go triggers init because that's
where it happens.

### 5.2 Auto-init test coverage

|  | `local/loop` | `origin/dev` |
|---|---|---|
| Dedicated auto-init test file | `apply_autoinit_test.go` (~2 tests) | `autoinit_test.go` (**11 tests**) |
| Tested scenarios | Basic idempotence | Idempotence, dry-run-no-side-effects, identity seeding, pre-set identity respected, global config created, store auto-init at default location, edge parametric cases |

`origin/dev` has genuinely better coverage of the auto-init path. This
is one of the clearest gaps in `local/loop`.

### 5.3 `storeinit` package — differences inside a shared shape

Both branches ship `internal/storeinit/storeinit.go` with go-git fallback
when `exec.LookPath("git")` fails, but the DI seams differ:

**`local/loop`** (commit `7efbd43`):

```go
var ExecGitInit = defaultExecGitInit   // func(ctx, dir) error
var GoGitInit  = defaultGoGitInit      // func(ctx, dir) error
const gitInitTimeout = 30 * time.Second
```

Two function-level seams; timeout is a fixed const.

**`origin/dev`**:

```go
var GitInitTimeout    = 30 * time.Second
var LookPathGit       = func() (string, error) { return exec.LookPath("git") }
var ExecCommandContext = exec.CommandContext
```

Three finer-grained seams (lookup, exec, timeout). Slightly more
test-swap options (you can simulate "PATH lookup fails" without
swapping the whole exec path).

**Integration-test coverage of the fallback path**:

- `local/loop`: apt integration.sh has a full **E0 scenario** (lines
  49–110) that `mv /usr/bin/git /tmp`, asserts `git` is gone, runs
  `hams apt install htop --debug`, and asserts the *"bundled go-git"*
  log line fires on stderr, then restores `/usr/bin/git` before the
  rest of the flow.
- `origin/dev`: **no E0 scenario**. The fallback code exists but is
  never exercised inside a container.

That E0 scenario is listed in `project-structure/spec.md:686–699` as a
SHALL. `local/loop` has both the code and the test; `origin/dev` has
only the code.

### 5.4 `--tag` / `--profile` conflict detection

Both branches share the same validator, which lives in the shared
`config` package:

```go
// internal/config/resolve.go
func ResolveCLITagOverride(cliTag, cliProfile string) (string, error) {
    if cliTag != "" && cliProfile != "" && cliTag != cliProfile {
        return "", hamserr.NewUserError(hamserr.ExitUsageError,
            i18n.T(i18n.CLIErrTagProfileConflict),
            "Remove either --tag=... or --profile=...")
    }
    ...
}
```

Difference is only **call-site density**:

- `local/loop`: 2 call sites (apply.go, provider_cmd.go)
- `origin/dev`: 5 call sites (apply.go, commands.go:×3, provider_cmd.go)

Both are correct; `origin/dev` is more thorough about invoking the
validator at every entry point (upgrade, config, refresh, apply,
provider shortcuts). A minor hardening win for `origin/dev`.

### 5.5 `hams git` passthrough for non-managed subcommands

Both branches implement the unified git provider (`internal/provider/
builtin/git/unified.go`) that routes `hams git config` and
`hams git clone` to managed handlers and everything else (status, log,
branch, rev-parse…) straight through to the system `git`.

Implementation differences:

**`local/loop`** — type named `UnifiedProvider`, `passthrough` is a
method with dry-run handling via `provider.DryRunRun` helper. No
module-level DI seam; `exec.CommandContext` is called inline.

**`origin/dev`** — type named `UnifiedHandler`, with a module-level
`var passthroughExec = func(ctx, args) error { ... }` that tests can
swap; dry-run handling writes to `flags.Stdout()` via a localised
`ProviderStatusDryRunRun` template; safely handles `flags == nil`.

Both pass their respective integration tests (`git/integration/
integration.sh` runs `hams git status`, `rev-parse`, `log`, `branch`
against a real repo). `origin/dev`'s is **cleaner for unit testing**
(module-level seam) but functionally equivalent.

### 5.6 `code-ext` → `code` full rename

Both branches have completed the rename at the user-facing level
(`cliName = "code"`, `filePrefix = "code"`, docs reference `hams
code install …`). The Go package stays as `vscodeext` in both. There
is **no remaining drift on either side**.

### 5.7 `--help` / `--version` warning suppression

Background: the pre-divergence `config.Validate()` emitted
`profile_tag is empty` / `machine_id is empty` warnings on every run
that loaded the config, including `hams --help` and `hams --version`.

**`local/loop`** (commit `3cafb27`) **extracts the warnings into a
separate helper**:

```go
// config.go: Validate() no longer warns.
func WarnIfDefaultsUsed(c *Config) {
    // slog.Warn only here; called explicitly by action sites
    // (apply, refresh, list) AFTER the structured logger is wired.
}
```

Dedicated test: `TestWarnIfDefaultsUsed_OncePerProcess`.

**`origin/dev`** takes a different path: it **wraps the warnings in
`sync.Once` guards inside `Validate()` itself**:

```go
func (c *Config) Validate() error {
    if c.StorePath != "" {
        if c.ProfileTag == "" {
            warnOnceProfileTag.Do(func() { slog.Warn("profile_tag is empty, ...") })
        }
        if c.MachineID == "" {
            warnOnceMachineID.Do(func() { slog.Warn("machine_id is empty, ...") })
        }
    }
    return nil
}
```

Both approaches prevent repeated-warn spam. `local/loop`'s lifts the
warning out of `Validate` entirely, so **metadata commands (--help,
--version) that still call Validate() produce no warning at all**;
`origin/dev`'s fires the warning once and silences subsequent calls
in the same process. If the very first warn-emitting call in a process
is `hams --help`, `origin/dev` will still emit one spurious warning.

### 5.8 CLI scoring

| Axis | Winner |
|---|---|
| Auto-init file separation | dev (`autoinit.go`) |
| Auto-init test coverage | dev (11 tests vs 2) |
| storeinit DI granularity | dev (3 seams vs 2) |
| storeinit fallback integration test | **loop (E0)** |
| `--tag`/`--profile` conflict — call-site density | dev (5 vs 2) |
| Git passthrough testability | dev (module-level seam) |
| Git passthrough behavioural parity | tie |
| `code-ext` → `code` rename | tie |
| `--help`/`--version` warning hygiene | **loop (extraction vs once-guard)** |
| Bootstrap consent flow | tie (identical) |

## 6. Integration Tests and CI

### 6.1 Shared helpers — `e2e/base/lib/`

| Helper | loop | dev |
|---|:-:|:-:|
| `assert_success`, `assert_output_contains`, `assert_stderr_contains` | ✅ | ✅ |
| `assert_log_line` (alias) | ✅ | ✅ |
| `assert_log_contains` (**file-based** — greps rolling log under `HAMS_DATA_HOME/<YYYY-MM>/hams.*.log`) | ❌ | ✅ |
| `assert_log_records_session` | ❌ | ✅ |
| `create_store_repo`, `run_smoke_tests`, verify_* helpers | ✅ | ✅ |

`origin/dev` (commit `a1cda2f`) adds a file-based log assertion family
(~40 new lines in `assertions.sh`). This complements the existing
stderr-based assertions: stderr captures what the user sees in one
invocation; file-based captures what landed in the persistent log that
`hams logs` can read later. That is a real product-correctness gain.

### 6.2 `provider_flow.sh` → `standard_cli_flow`

**`local/loop`** embeds a log-emission gate inside `standard_cli_flow`
(lines 220–243) so **every** provider that calls the helper automatically
gets two assertions: "hams itself emits session-start log" and
"provider emits slog line". Net: 11 providers × 2 asserts for free.

**`origin/dev`** removes those lines from the shared helper and
**fans the assertions out to each provider's own `integration.sh`**
(commit `a1cda2f`). This adds ~11 lines per provider × 11 providers.

Tradeoffs:

| | loop (shared) | dev (fanned-out) |
|---|---|---|
| Lines of script code | Shorter (helpers do the work) | Longer (~120 extra lines across providers) |
| Flexibility per provider | Lower (all providers share the gate) | Higher (each provider can tailor its log expectations) |
| Chance of forgetting the gate on a new provider | Zero (inherited from helper) | High (must remember to add it manually) |
| Debuggability when it fails | Harder (you see "provider foo failed the shared gate") | Easier (failure pins to the specific provider file) |

Both are legitimate; neither strictly dominates. `origin/dev` gains the
per-file log-file assertions (a genuine win). `local/loop` gains
"you can't forget it" inheritance.

### 6.3 apt integration — the E0 case

This is the single biggest behavioural test gap between the branches.

**`local/loop` apt/integration/integration.sh, lines 49–110:**

```bash
echo "--- E0: storeinit go-git fallback on a git-less machine ---"
E0_CONFIG_HOME=/tmp/e0-config
E0_DATA_HOME=/tmp/e0-data
...
# precondition: remove /usr/bin/git so LookPath("git") fails
sudo mv /usr/bin/git /tmp/git.hidden
assert_stderr_contains "E0: go-git fallback log line fires" "bundled go-git" \
    env HAMS_CONFIG_HOME="$E0_CONFIG_HOME" \
        HAMS_DATA_HOME="$E0_DATA_HOME" \
        hams apt install htop --debug
# restore git before the canonical flow
sudo mv /tmp/git.hidden /usr/bin/git
```

**`origin/dev` apt/integration/integration.sh:** no E0 section; the
script jumps straight to `standard_cli_flow`. The go-git fallback in
`storeinit.go` is never exercised inside a container.

This matters because the SHALL at `project-structure/spec.md:686–699`
reads *"A fresh machine without system git SHALL auto-scaffold a
store using the bundled go-git library"*. `local/loop`'s E0 is the
only real proof. Without it, a regression that breaks the go-git path
ships silently.

### 6.4 Taskfile and CI workflow

Both branches:

- Have a matching `ci:itest:base` + `ci:itest:run PROVIDER=<name>` +
  `ci:itest` task shape.
- Gate GitHub artefact upload on `if: !env.ACT` so `act` runs don't
  need artefact servers.
- Redirect `test:e2e`, `test:integration`, `test:itest` to invoke
  `ci:*` tasks directly (act is opt-in via `*-via-act` variants).

The act-opt-in story is captured on each branch by a dedicated commit
(`d63edb9` on loop, `770f3ab` on dev) and the resulting Taskfile.yml
and ci.yml are functionally identical. No gap to close here.

### 6.5 `.golangci.yml`

Both branches ship the same linter set (govet, staticcheck, errcheck,
revive, gosec, etc.) and the same `errcheck.exclude-functions`
allow-list for in-memory write sinks. No drift.

### 6.6 Integration-test scoring

| Axis | Winner |
|---|---|
| apt E0 (go-git fallback) | **loop** (only branch with container proof) |
| Per-provider stderr log gate | tie (both have it, loop via helper, dev per-file) |
| Per-provider **file-based** log assertion | dev (only branch with it) |
| `HAMS_DATA_HOME` isolation per integration script | dev |
| Resilience to "forgot to add the log gate on a new provider" | loop (inheritance) |
| Debuggability on failure | slight edge to dev (failures pin to provider script) |
| CI workflow / Taskfile parity | tie |

## 7. Provider-internal Details Worth Noting

### 7.1 `goinstall`

Both branches had to square `goinstall`'s lack of a real `uninstall`
with the shared dispatcher expectations.

- `local/loop`: adds a **documented no-op `Uninstall`** on the `goinstall`
  runner so it satisfies the `PackageInstaller` interface. The provider
  then uses `AutoRecordInstall` / `AutoRecordRemove` directly. The
  no-op is intentional and commented (`go install` packages are
  removed by deleting binaries from `$GOBIN`; the hamsfile is still
  updated so `apply` stops reinstalling them).
- `origin/dev`: inlines the whole flow; no-op uninstall is implicit
  in the inline loop.

### 7.2 `homebrew`

- `local/loop`: uses `AutoRecordInstallFn` / `AutoRecordRemoveFn` to
  curry the `isCask bool` into a closure, plus carries the
  `tap-vs-uninstall` routing in the same closure. Lifts the exemption
  that was flagged in the provider-system spec.
- `origin/dev`: hand-rolls the same dance inline. Homebrew-specific
  `baseprovider.EffectiveConfig` / `baseprovider.LoadOrCreateHamsfile`
  calls replace what `local/loop` inlines per provider.

### 7.3 `apt`

Both branches accept apt as an exemption from the shared dispatcher
because its runner signature is `Install(ctx, args []string)` (batch
install), incompatible with the per-package `PackageInstaller`.

- `local/loop`: **explicitly documents the exemption** in a block comment
  at the top of `package_dispatcher.go`, and pins a follow-up change
  (`BatchPackageInstaller` dispatcher variant) for future work.
- `origin/dev`: the exemption is silent — apt simply doesn't call the
  dispatcher. No comment, no follow-up pointer.

### 7.4 `git` and `vscodeext`

Both branches land the unified `hams git` entry point (config + clone
sub-providers, plus passthrough) and the `code-ext` → `code` rename.
The implementations differ only in the DI seams discussed in §5.5.

## 8. OpenSpec Workflow

| | `local/loop` | `origin/dev` |
|---|---|---|
| Total openspec changes touched since base | 12 | 18 |
| Archived (completed) | **12 / 12** | 7 / 18 |
| In-flight proposals at tip | **0** | **11** |
| Main specs updated | 7 | 3 (`provider-system`, `builtin-providers`, `schema-design`) |
| SHALL requirements added to main specs | More (dispatcher SHALL, storeinit boundary, auto-scaffold, i18n-for-providers) | Fewer (implicit; still in in-flight delta specs) |

### 8.1 What each branch archived that the other did not

Archived only on `local/loop` (the 7 changes that loop closed but dev
still treats as in-flight):

- `2026-04-17-apply-tag-and-auto-init` — the `--tag` flag and first-run
  auto-init, implemented.
- `2026-04-17-cli-i18n-catalog` — the initial CLI-side i18n wiring.
- `2026-04-17-package-provider-auto-record-gap` — the auto-record
  contract enforcement that motivated the dispatcher.
- `2026-04-17-provider-autoscaffold-store` — provider invocations
  scaffold a store on first run.
- `2026-04-17-provider-shared-abstractions` — the dispatcher itself.
- `2026-04-17-spec-impl-reconciliation` — one pass of spec ↔ code
  alignment.
- `2026-04-17-verification-findings` — follow-up to the prior review.

`local/loop` additionally adds three 2026-04-18 archival packages
directly tied to this session:

- `2026-04-18-storeinit-package-with-gogit-fallback` — the
  `internal/storeinit/` package + E0 test.
- `2026-04-18-i18n-builtin-provider-catalog` — the 60-key
  `provider.*` i18n fanout with locale-parity tests.
- `2026-04-18-provider-shared-abstraction-adoption` — the 8-of-9
  dispatcher adoption with closure variants.

In-flight only on `origin/dev` (the 11 proposals dev is still carrying):

- `2026-04-17-onboarding-auto-init` (new name for auto-init work)
- `2026-04-18-auto-init-ux-hardening` (dry-run + timeout + identity seed)
- `2026-04-18-ci-act-opt-in`
- `2026-04-18-code-provider-full-rename`
- `2026-04-18-docs-sync`
- `2026-04-18-git-passthrough-and-spec`
- `2026-04-18-i18n-fanout-all-userfacing` (the 176-key expansion)
- `2026-04-18-integration-log-assertion-fanout`
- `2026-04-18-shared-abstraction-migration` (the baseprovider migration)
- `2026-04-18-tag-profile-conflict-detection`
- `2026-04-18-typed-i18n-keys`

### 8.2 Scoring

| Axis | Winner |
|---|---|
| Closure discipline (0 dangling proposals) | **loop** |
| Main specs updated with new SHALLs | **loop** |
| In-flight visibility into upcoming work | dev |
| Traceability of what shipped this session | loop (archives named by feature) |

`local/loop`'s workflow is strictly tidier. `origin/dev` is equally
valid but defers the closure step, leaving a reader unable to tell
from the tree alone which proposals are done vs queued.

## 9. Gap Analysis — What `local/loop` Should Absorb from `origin/dev`

The following items are real implementation wins on `origin/dev` that
`local/loop` would benefit from adopting. Each has a clear,
bounded scope.

### 9.1 Priority — do these soon

1. **Port the `Passthrough` primitive** to `internal/provider/
   passthrough.go` with a module-level `PassthroughExec` seam and
   `flags.DryRun` handling. Migrate `WrapExecPassthrough` call sites
   (homebrew, cargo, apt default case) to it. Ship a
   `passthrough_test.go` that swaps the seam.

2. **Port the file-based log assertions** (`assert_log_contains`,
   `assert_log_records_session`) to `e2e/base/lib/assertions.sh`.
   Add an apt-side assertion that the session-start record lands in
   `${HAMS_DATA_HOME}/<YYYY-MM>/hams.*.log`. The existing stderr
   gate in `standard_cli_flow` stays — these are additive.

3. **Add dedicated auto-init tests** (matching dev's `autoinit_test.go`
   coverage — dry-run-no-side-effects, identity seeding, pre-set
   identity respected, global config creation, default-location
   store auto-init). `local/loop` currently has only 2 tests for
   this surface.

4. **Expand the i18n catalog** to cover the CLI lifecycle that
   `origin/dev` already translated: `autoinit.*`, `ufe.*` family,
   `store.init/pull/commit`, `config.set/unset/open`, `list.*`,
   `upgrade.*`. Keep the existing locale-parity and
   template-interpolation tests; add a `dev`-style "every typed
   constant appears in both locales" coherence test on top.

### 9.2 Nice-to-have

1. **Extract a `baseprovider` helper package** with
   `EffectiveConfig`, `HamsfilePath`, `LoadOrCreateHamsfile` and
   migrate providers to it. This replaces ~30 lines of per-provider
   boilerplate. Lower priority because the closure-variant
   dispatcher already lifts the highest-value duplication.

2. **Extract `internal/cli/autoinit.go`** out of `apply.go` /
   `commands.go` / `provider_cmd.go`. Functional parity with current
   behaviour; improves SRP and makes the auto-init feature
   grep-locatable by name.

3. **Finer-grained storeinit DI seams** — switch from
   `ExecGitInit`/`GoGitInit` functions to
   `LookPathGit`/`ExecCommandContext`/`GitInitTimeout` vars. Lets
   tests simulate "PATH lookup fails" without replacing the whole
   exec path. Minor win; the E0 integration test already covers the
   real path.

4. **Broaden `--tag`/`--profile` conflict validation** to the three
   additional call sites in `commands.go` (upgrade, config, refresh
   short-paths). The validator and its error message already exist
   — just invoke it earlier.

## 10. Gap Analysis — What `origin/dev` Would Absorb from `local/loop`

If the inverse merge direction were ever needed, these are the real
wins on `local/loop`:

1. **`AutoRecordInstallFn` / `AutoRecordRemoveFn` closure variants**
   — eliminate the hand-rolled inline flow in every package-class
   provider. 8 of 9 providers shrink by ~40–80 lines each.
2. **Homebrew adopts the dispatcher** via the closure variant — lifts
   the last exemption.
3. **apt E0 integration scenario** — hide `/usr/bin/git`, assert
   go-git fallback log line fires. This covers the
   `project-structure/spec.md:686–699` SHALL that is untested on dev.
4. **`WarnIfDefaultsUsed` extraction** out of `config.Validate()` —
   cleaner separation than `sync.Once`-gated warnings inside Validate,
   and guarantees `--help` / `--version` are fully silent.
5. **Locale dynamic parity test** + **template-interpolation
   regression test** — catches orphan YAML keys and unresolved `{{.X}}`
   placeholders that dev's "key appears in file" test misses.
6. **Spec formalisation**: the dispatcher SHALL, storeinit-boundary
   SHALL, first-run auto-scaffold SHALL, builtin-provider-i18n SHALL
   are all written on loop's `openspec/specs/**` but not on dev's.
   dev's in-flight proposals cover the same ground but haven't
   merged into main specs yet.
7. **Documented exemption for apt** in `package_dispatcher.go` with
   a named follow-up (BatchPackageInstaller). dev's apt is silently
   non-adopted.
8. **Closure-discipline**: 0 in-flight proposals at tip vs dev's 11.

## 11. Overall Verdict

On the **architectural foundations** that enable future scaling
(baseprovider helpers, dedicated Passthrough, isolated autoinit
module, comprehensive i18n fanout), **`origin/dev` is ahead**.

On the **finishing touches that make V1 actually feel done**
(every package provider adopts the dispatcher, the go-git fallback
has a real container test, warnings are cleanly suppressed,
every proposal is archived, main specs carry the new SHALLs),
**`local/loop` is ahead**.

Both branches pass `task check`; neither is broken. The gap
analysis in §9 and §10 lays out the concrete mutual borrowings.
For `local/loop` specifically, the four priority ports in §9.1
(Passthrough, file-based log assertions, auto-init tests, i18n
breadth) together capture ≥80% of the infrastructure gains that
`origin/dev` has without giving up the dispatcher, E0, and archival
wins that `local/loop` already owns.
