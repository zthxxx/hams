# Design: Homebrew self-install bootstrap as an explicit opt-in

## Context

The Homebrew provider spec (`builtin-providers`) declares that hams
SHALL auto-bootstrap `brew` via `depend-on: bash` when it is missing.
The code has never done this. The `DependOn.Script` field on the
provider manifest has been dead data since v1 — only `DependOn.Provider`
is read (for topological ordering in `dag.go`).

This change resolves the divergence by making the bootstrap an **explicit
opt-in** rather than either (a) silently executing `curl | bash` or
(b) dropping the fresh-machine user off a cliff.

## Decision: Option C — Hybrid opt-in

Both the architect-role and user-role Agent debates converged on the
same option:

- **Architect position paper** (preserved verbatim below): "Option C —
  Opt-in `--bootstrap` with actionable default error." Strongest
  argument: "the integration test already proves Option A is a
  fantasy." The team pre-installs linuxbrew in the Dockerfile
  precisely because the bootstrap path is interactive and
  platform-gated; the spec is aspirational prose, not shipped behavior.
- **User position paper** (preserved verbatim below): "Option C —
  Interactive prompt with receipts." Strongest argument: "Option A is
  a trust violation. The moment hams silently pipes `curl | bash`
  from a third-party domain on my behalf, it's no longer a config
  tool — it's a bootstrap agent running arbitrary network code with
  my sudo password queued up."

Both agents independently landed on the same structure:

1. **Default** (no flag, not a TTY, or running in CI): emit an
   actionable `UserFacingError` naming the missing binary, the full
   script, and the exact `hams apply --bootstrap ...` re-run command.
   Exit non-zero. No network traffic, no side effects.
2. **Explicit flag** (`--bootstrap` or `--yes`): resolve
   `DependOn.Script` through the Bash provider and execute. Stream
   stdout/stderr; forward interactive prompts to the TTY.
3. **Interactive TTY** (stdin is a terminal, `--bootstrap` not set):
   display the script about to run + the Xcode-CLT gotcha, prompt
   `[y/N/s]`. On `y`, delegate to the Bash provider as in branch 2.
4. **Bootstrap failure is terminal**: exit code + last 50 lines of
   output surfaced; no retry; abort the apply.

## Why Not Option A (silent auto-bootstrap)

- Auto-executing `curl | bash` from `raw.githubusercontent.com` is a
  security posture change that should require explicit user consent.
- Corporate firewalls (common in enterprise developer setups) block
  raw.githubusercontent.com without explicit auth headers. Silent
  failure is indistinguishable from "brew is broken" for the user.
- Homebrew's `install.sh` on macOS can trigger the Xcode CLI Tools GUI
  dialog, which **blocks stdin** — the installer appears hung for
  minutes while the user stares at a terminal not realizing a modal
  dialog is waiting. hams MUST warn users about this before running
  the script.
- The integration test already pre-installs linuxbrew. The team
  building hams didn't trust the auto-bootstrap path enough to put it
  on the critical path.

## Why Not Option B (error-only, no bootstrap ever)

- Core Philosophy #4 ("one-command restore") is load-bearing for the
  project's fresh-machine pitch. On macOS, brew is the first thing
  anyone installs; if hams fails before it starts, the whole tool's
  value prop collapses on day one.
- "Run this command yourself, then re-run hams" is a scavenger hunt
  that defeats the one-command promise.

## Why Option C Works

Mirrors the `--prune-orphans` precedent (cycle
`clarify-apply-state-only-semantics`, 2026-04-15): destructive or
remote-executing defaults are **always opt-in**. Users who want the
one-command path get it explicitly (`hams apply --bootstrap
--from-repo=...`). Users who want auditability get the actionable
error with the script text to review.

The interactive `[y/N]` prompt on a TTY is a graceful middle ground:
fresh-Mac users running hams interactively see the script, consent,
proceed. CI/batch users pass `--bootstrap`. Users who want fail-fast
can pass `--no-bootstrap`.

## Implementation Sketch

### New: `provider.RunBootstrap(ctx, p, registry)`

```go
// internal/provider/bootstrap.go
func RunBootstrap(ctx context.Context, p Provider, registry Registry) error {
    for _, dep := range p.Manifest().DependsOn {
        if !matchesPlatform(dep.Platform) {
            continue
        }
        if dep.Script == "" {
            continue
        }
        host, ok := registry.Lookup(dep.Provider)
        if !ok {
            return fmt.Errorf("bootstrap host provider %q not registered", dep.Provider)
        }
        bashRunner, ok := host.(BashScriptRunner)
        if !ok {
            return fmt.Errorf("bootstrap host %q does not implement BashScriptRunner", dep.Provider)
        }
        if err := bashRunner.RunScript(ctx, dep.Script); err != nil {
            return fmt.Errorf("bootstrap script for %q failed: %w", p.Manifest().Name, err)
        }
    }
    return nil
}
```

`BashScriptRunner` is a new tiny interface that the Bash provider
already fulfills (it already knows how to exec arbitrary bash). This
keeps the cross-provider coupling minimal and DI-friendly.

### Homebrew `Bootstrap` with ctx-threaded opt-in

```go
// ErrBootstrapRequired signals that a provider's prerequisite is
// missing AND the user hasn't opted into auto-installing it.
var ErrBootstrapRequired = errors.New("bootstrap required")

func (p *Provider) Bootstrap(ctx context.Context) error {
    if _, err := exec.LookPath("brew"); err == nil {
        return nil
    }
    if !BootstrapAllowed(ctx) {
        return &UserFacingError{
            Summary: "Homebrew is required but not installed",
            Detail:  p.Manifest().DependsOn[0].Script,
            Remedy:  "re-run with --bootstrap, or install Homebrew manually",
            Err:     ErrBootstrapRequired,
        }
    }
    return provider.RunBootstrap(ctx, p, p.registry)
}
```

`BootstrapAllowed(ctx)` reads a ctx value the Apply CLI layer sets when
`--bootstrap` is passed (or the interactive prompt is answered `y`).

### Apply CLI: prompt + flag wiring

```go
// internal/cli/apply.go
flagBootstrap := flags.Bool("bootstrap", false, "auto-run provider bootstrap scripts when prerequisites are missing")

ctx = provider.WithBootstrapAllowed(ctx, *flagBootstrap)

// inside the per-provider Bootstrap failure handler:
if errors.Is(err, provider.ErrBootstrapRequired) && isTTY(os.Stdin) && !*flagBootstrap {
    if userConsentedAtPrompt(...) {
        ctx = provider.WithBootstrapAllowed(ctx, true)
        err = p.Bootstrap(ctx)  // retry
    }
}
```

### Second integration container

`internal/provider/builtin/homebrew/integration/Dockerfile.bootstrap`:
start from `hams-itest-base` with NO linuxbrew. Drive `hams apply
--bootstrap` against a fixture hamsfile. Assert: brew appears on
`$PATH`, the hamsfile-declared package is installed.

## Tradeoffs

| Aspect | Chosen | Alternative | Why chosen |
|---|---|---|---|
| Default behavior | Actionable error | Silent auto-install | Security, auditability, Xcode-CLT gotcha |
| Opt-in surface | `--bootstrap` flag + TTY prompt | Flag only | Ergonomic for interactive users |
| Bootstrap mechanism | Bash provider delegation | Direct `exec.Command("bash", "-c", ...)` | Honors DI seam; `RunScript` is testable |
| Retry on failure | None | Auto-retry once | Network install failures are usually fatal; surface + exit |
| Xcode-CLT warning | Include in prompt | Only in docs | Users staring at a hung terminal will thank us |

## Out of Scope

- Auto-installing language runtimes for non-brew providers (npm, pnpm,
  cargo, uv, etc.). Those can be follow-up changes; they share this
  change's `RunBootstrap` primitive but each has its own trust /
  interactivity profile.
- A package-manager abstraction layer that enumerates "what's missing"
  up-front. The spec here handles one provider at a time, on-demand.

## Agent-team position papers (verbatim)

### Architect role

> # Position Paper: Option C — Hybrid (Opt-in `--bootstrap` with actionable default error)
>
> ## Choice: **Option C**
>
> ## Why (strongest arguments)
>
> **1. The spec is lying about a capability that doesn't exist and
> can't safely exist as default.** The current spec commits hams to
> auto-executing `curl | bash` from a remote URL. For a tool whose
> pitch is "one-command restore on a fresh machine," silently piping
> a 14KB install script into `bash` the first time a user runs
> `hams apply` violates the principle of least astonishment *and*
> trust. hams is pragmatic (Philosophy #2) but it is not reckless —
> every other provider wraps an already-installed package manager.
> Homebrew is the one exception, and it deserves an explicit
> ceremony, not a hidden side-effect.
>
> **2. The integration test already proves Option A is a fantasy.**
> `internal/provider/builtin/homebrew/integration/integration.sh`
> pre-installs linuxbrew in the Dockerfile *precisely because* the
> bootstrap path is interactive and platform-gated.
>
> **3. Opt-in preserves both promises.** `--bootstrap` on `hams
> apply` makes the fresh-machine path one *deliberate* command.
> Users who care about auditing `install.sh` first can run without
> the flag, read the actionable error, and decide. This is exactly
> the pattern cycle-2 established for destructive reconciliation
> with `--prune-orphans` — opt-in destructive/remote-executing
> behavior is already a ratified house style.

### User role

> # Position Paper: Option C — Interactive Prompt with Receipts
>
> **Choice: C.** Not "ask me each time" in the wishy-washy sense —
> **ask me once, show me the command, let me copy-paste or confirm.**
>
> ## Why
>
> **1. Option A is a trust violation.** I run `curl | bash` on my
> own machines daily, but I decide when. If hams does it silently,
> my corporate-laptop coworkers literally cannot use it (our proxy
> blocks `raw.githubusercontent.com` without explicit auth headers).
>
> **2. Option B is user-hostile on the one day it matters most.**
> Fresh Mac, 11pm, I just want my dotfiles. Printing "run this
> command yourself" and exiting means I now context-switch, copy,
> paste, re-run hams, hit the next missing dep, repeat.
>
> **3. Option C gives me the receipts AND the convenience.** Show
> me the exact command, let me hit `y`. I've seen what's about to
> run. Non-interactive (`--yes` / CI) is opt-in.
>
> ## Personal edge case
>
> Last fresh M2 Mac: brew's installer triggered the Xcode CLI
> Tools GUI dialog, which **blocks stdin**. The installer appeared
> hung for 8 minutes while I stared at a terminal, not realizing a
> dialog was waiting behind my IDE. hams MUST warn about this
> explicitly in the prompt — otherwise users will Ctrl-C and think
> hams is broken, when really macOS is waiting on a click.
