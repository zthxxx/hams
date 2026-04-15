# Homebrew self-install bootstrap as an explicit opt-in

## Why

A critical-architect review of the just-archived 4-cycle session surfaced
a real spec/code divergence in the Homebrew provider.

**Spec** (`openspec/specs/builtin-providers/spec.md:375-378`):

> #### Scenario: Homebrew self-install bootstrap
> WHEN the Homebrew provider is needed but `brew` is not found on `$PATH`
> THEN hams SHALL resolve the `depend-on: bash` declaration, locate the
> Bash provider, and execute the Homebrew install step (defined as a Bash
> step in the Bash Hamsfile or as an inline bootstrap script in the
> Homebrew provider manifest) before proceeding with any Homebrew
> operations.

**Code** (`internal/provider/builtin/homebrew/homebrew.go:60-66`):

```go
func (p *Provider) Bootstrap(_ context.Context) error {
    if _, err := exec.LookPath("brew"); err == nil {
        return nil
    }
    slog.Info("Homebrew not found, bootstrapping via bash provider")
    return fmt.Errorf("homebrew not installed; run the bootstrap script first")
}
```

The manifest DOES declare the bootstrap payload:

```go
DependsOn: []provider.DependOn{{
    Provider: "bash",
    Script:   `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`,
}}
```

But `DependOn.Script` is **never read** — `ResolveDAG` (`internal/provider/dag.go:30`)
uses `DependsOn` only for topological ordering, and `grep -rn "\.Script"
internal/` returns zero matches. The declared script is dead data. The
spec has been lying about a capability that does not exist.

An autonomous architect/user Agent debate (both perspectives recorded
in `design.md`) converged on **Option C — opt-in hybrid**:

- Auto-running `curl | bash` silently (Option A) violates the principle
  of least astonishment. Even the integration test side-steps it by
  pre-installing linuxbrew in the Dockerfile. Corporate firewalls block
  `raw.githubusercontent.com`. Xcode CLI Tools prompt blocks stdin.
- Error-out-and-tell-user (Option B) drops a fresh-Mac user off a cliff
  on the one day it matters most — the "one-command restore" promise
  dies on the flagship platform.
- Explicit consent (Option C) preserves both: default = actionable error
  with the exact command; `--bootstrap` flag (or interactive `[y/N]`
  prompt on a TTY) = execute the manifest-declared script via the Bash
  provider. Mirrors the `--prune-orphans` pattern from the prior cycle
  (destructive/remote-executing defaults are always opt-in).

## What Changes

- **Provider framework** (`internal/provider/dag.go` + new
  `internal/provider/bootstrap.go`): add `RunBootstrap(ctx, p,
  registry)` that reads `p.Manifest().DependsOn[i].Script`, resolves the
  target bash-compatible provider, and executes the script under the
  Bash provider's exec boundary (honoring the existing DI seam). This
  makes `DependOn.Script` a live contract instead of documentation.
- **Homebrew provider** (`homebrew.go`): rewrite `Bootstrap` to emit a
  structured `UserFacingError` naming the missing binary, the exact
  script, and the `--bootstrap` flag. When `--bootstrap` was provided
  (threaded via ctx), delegate to `provider.RunBootstrap`.
- **Apply CLI** (`internal/cli/apply.go`): add `--bootstrap` flag (bool,
  default `false`). When a provider's `Bootstrap` returns
  `ErrBootstrapRequired`, either (a) run `RunBootstrap` if
  `--bootstrap` was set, (b) open an interactive TTY prompt if stdin
  is a terminal, or (c) abort with the actionable error.
- **Spec** (`builtin-providers`): replace the single aspirational
  bootstrap scenario with three honest ones — default error, explicit
  consent (`--bootstrap`), and terminal bootstrap failure.
- **Tests:** unit tests around `RunBootstrap` + the prompt branching;
  second integration-container variant
  `internal/provider/builtin/homebrew/integration/Dockerfile.bootstrap`
  that starts WITHOUT linuxbrew and exercises the opt-in path.
- **Docs** (`docs/content/{en,zh-CN}/docs/providers/homebrew.mdx` +
  `cli/apply.mdx`): describe the default error, the opt-in flag, and
  the Xcode-CLT dialog gotcha.

## Impact

- **Affected specs:** `builtin-providers` (replace 1 scenario → 3
  scenarios for bootstrap), `cli-architecture` (ADD `--bootstrap` flag).
- **Affected code:** `internal/provider/{dag,bootstrap,provider}.go`,
  `internal/provider/builtin/homebrew/homebrew.go`,
  `internal/cli/apply.go`, docs, homebrew integration layer.
- **Backwards compatibility:** preserved. Current behavior (error out)
  is what the default branch already does. The new `--bootstrap` flag
  adds capability without changing default semantics.
- **User-visible change:** clearer error message with copy-pasteable
  remedy; new opt-in flag for one-command fresh-machine restore.

## Provenance

- Holistic critical-architect review surfaced the divergence; two
  Agent-team debate rounds (architect-role + user-role) both landed on
  Option C hybrid. Decision + arguments preserved in `design.md`.
- Related: `clarify-apply-state-only-semantics` (2026-04-15) already
  established `--prune-orphans` as the opt-in precedent for
  destructive/remote-executing defaults.
- The dead `DependOn.Script` field has been in the manifest since the
  v1 design — this change finally honors what the manifest already
  declared.
