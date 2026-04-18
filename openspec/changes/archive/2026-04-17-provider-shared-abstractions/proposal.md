# 2026-04-18-provider-shared-abstractions

## Why

CLAUDE.md's Current Tasks list says:

> All providers follow the same pattern: parse the original command
> structure, extract what needs to be recorded, then pass the
> remainder through to the underlying command for execution. Since
> providers are structurally similar at the code level, design
> shared abstractions — either a single generic base or a few
> categorical base types — so that extending with a new provider is
> a matter of filling in a well-defined template, not
> reimplementing the pattern from scratch.

Today's reality (measured at 2026-04-18 after the git unify +
code-ext rename):

- 10 package-like providers (apt, brew, pnpm, npm, cargo, goinstall,
  uv, mas, vscodeext/code, duti) each own ~200 lines of install /
  remove / list handlers that all do the same five-step dance:
  (1) parse args, (2) dry-run shortcut, (3) acquire mutation lock,
  (4) call runner.Install/Uninstall per pkg, (5) write hamsfile +
  state.
- 2 kv-config providers (git-config, defaults) share a different
  but equally repetitive shape around set/unset/list of key=value
  records.
- 2 filesystem-class providers (git-clone, plus whatever comes
  next) wrap clone-style mutations.
- 2 script providers (bash, ansible) invoke user-declared check /
  run / remove commands.

Copy-pasting the install/remove dance when adding provider N+1
means bug fixes have to land N+1 times (see cycles 96 / 202-208
which each fixed "hams list returned empty after imperative
install" on a different provider). The user's ask lines up with
this pain: make adding a new provider template-driven.

## What Changes

### 1. Introduce `internal/provider/package_dispatcher.go`

New file exposes two helpers that capture the canonical
package-like install/remove flow:

```go
func AutoRecordInstall(ctx, runner, pkgs, cfg, flags,
    hfPath, statePath, opts) error

func AutoRecordRemove(ctx, runner, pkgs, cfg, flags,
    hfPath, statePath, opts) error
```

Both helpers:

- Validate `len(pkgs) > 0`, returning a UsageError naming
  `opts.CLIName` + `opts.InstallVerb` / `opts.RemoveVerb` so the
  error message reads naturally to the user.
- Short-circuit `flags.DryRun` with a `[dry-run] Would install:
  <cli> <verb> <pkgs>` line on `flags.Stdout()` (picks up the
  io.Writer seam added in the race-fix commit).
- Acquire the shared mutation lock via
  `AcquireMutationLockFromCfg` — the same seam every provider
  already uses.
- Call `runner.Install` / `runner.Uninstall` per pkg, failing
  fast on first error so partial work isn't recorded.
- Emit a `slog.Info("<cli> <verb>", "package", pkg)` line per
  package, matching the convention the integration-test log
  assertions rely on.
- LoadOrCreate the hamsfile + state, update both, write both.

Callers thread `PackageInstaller` (interface of
`Install(ctx, pkg) / Uninstall(ctx, pkg)`) + a
`PackageDispatchOpts` value naming the CLI verb, install verb,
remove verb, and hamsfile tag. That is enough context for the
helper to reconstruct every string it needs to produce. No
provider-specific struct inheritance; no generics; no reflection.

### 2. Why NOT a base struct / abstract provider

A few approaches were considered and rejected:

- **Embedded base struct.** Would force every package provider to
  name its CmdRunner field the same, and would leak through tests
  which currently inject fakes on the concrete provider. Also
  makes provider-specific customizations (apt's
  `isComplexAptInvocation`, brew's tap detection) awkward.
- **Registry-driven declarative config.** "Declare your verbs and
  let hams generate the handler" is appealing but falls over on
  the first provider with quirks (apt's --simulate, brew's --cask,
  vscodeext's @version pins). Five providers' worth of special
  cases become if-ladders inside the generator.
- **Code generation.** Overkill for ~10 providers and breaks the
  "one line to check the implementation" review experience.

Plain helper functions leave each provider's HandleCommand readable
top-to-bottom AND let providers with real divergence (apt) opt out
of the helper entirely.

### 3. Follow-up migrations (out of scope for this change)

Existing 10 package-like providers KEEP their current HandleCommand
implementations. Migration happens incrementally in follow-up
cycles:

1. **cargo** pilot — simplest surface, no --simulate / --cask
   quirks. Swap handleInstall / handleRemove to call the helpers;
   existing U1-U7 tests keep passing (pattern-preserving). ~60-line
   diff.
2. **npm, pnpm, uv, goinstall, mas** — same shape as cargo, one
   commit each.
3. **vscodeext** — needs the version-pin-strip hook but otherwise
   matches.
4. **brew** — keep inline; the tap-detection + cask-flag branches
   don't fit the helper and re-implementing them inside it would
   be worse than the current inline code.
5. **apt** — keep inline; --simulate detection + post-install
   version probe are legitimately different.

### 4. What this change ships

- The `provider.AutoRecordInstall` + `AutoRecordRemove` helpers
  above, with their `PackageDispatchOpts` and `PackageInstaller`
  types.
- A documentation comment block in
  `internal/provider/package_dispatcher.go` pointing future
  providers at the helper.
- Zero provider migrations in this change — staged as incremental
  follow-ups to keep each migration reviewable.

## Verification

- `go build ./...` + `go test -race ./...` green (helper compiles,
  existing providers untouched, zero behavior change).
- Future cycle proves the abstraction by swapping one provider
  (cargo) onto the helpers; that diff will be the "does this
  helper pay off?" check.

## Out of Scope

- Extracting shared abstractions for kv-config providers
  (git-config, defaults). They have a smaller N (2) and more
  variance in what "key" means. Worth a separate design cycle.
- Extracting shared abstractions for filesystem providers
  (git-clone) — N=1 today, premature to generalize.
- Providing a `provider.Base` embed. Explicit rejection in the
  "Why not a base struct" section above; revisit if we ever hit
  N>20 providers all following the same verb set.
