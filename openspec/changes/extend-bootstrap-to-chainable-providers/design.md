# Design: Extend bootstrap consent pattern to chainable providers

## Context

Cycle 5 (`homebrew-bootstrap-opt-in`, archived 2026-04-16) built a
generic consent-gated bootstrap primitive in `internal/provider/bootstrap.go`:

- `BashScriptRunner` interface
- `RunBootstrap(ctx, p, registry)` that iterates `DependsOn`,
  platform-filters, and delegates non-empty `.Script` to a registered
  `BashScriptRunner`
- `BootstrapRequiredError{Provider, Binary, Script}` typed error +
  `ErrBootstrapRequired` sentinel
- CLI `--bootstrap` / `--no-bootstrap` flags + TTY `[y/N/s]` prompt

Only the `homebrew` provider uses it. The other 11 builtin providers
return plain strings on missing prerequisites. This cycle extends the
pattern to 4 providers where auto-install is safe and useful:
`pnpm`, `duti`, `mas`, `ansible`.

## Decision: Option C — Narrow extension

Agent-team debate converged (both papers below) on narrow extension:

- **Architect position**: 4 providers. The architecture cost is
  already paid by cycle 5; the gating question is "does the install
  command have reasonable success rate on a fresh, unattended
  machine?" Four pass that test.
- **User position**: extend everywhere; one-command restore is the
  product promise; fragmented UX is the bug.

The **synthesis** is the architect's narrow scope because:

1. The user's Option B requires solving hard problems (which node
   installer for npm? nvm vs fnm vs apt) that hams does not have
   the right abstraction to own. A rushed decision here would lock
   hams into a stance that's painful to reverse.
2. The user's minimum-acceptable bar (structured error) is satisfied
   by C for 4/11 providers — that's **4 more than zero**. The
   remaining 7 still get their existing plain-string errors, which
   is the status quo, not a regression.
3. The architect's criteria for each skip are explicit scenarios
   future maintainers can debate. Skipped providers are not orphans;
   they have recorded reasoning.

## Per-provider decisions

### `pnpm` — extend

**Current Bootstrap:**

```go
func (p *Provider) Bootstrap(_ context.Context) error {
    if _, err := exec.LookPath("pnpm"); err != nil {
        return fmt.Errorf("pnpm not found in PATH; install via: npm install -g pnpm")
    }
    return nil
}
```

**Target:**

```go
func (p *Provider) Bootstrap(_ context.Context) error {
    if _, err := exec.LookPath("pnpm"); err == nil {
        return nil
    }
    return &provider.BootstrapRequiredError{
        Provider: "pnpm", Binary:   "pnpm",
        Script:   "npm install -g pnpm",
    }
}
```

**Manifest change:** `DependsOn[0].Script = "npm install -g pnpm"`
(currently just has `{Provider: "npm", Package: "pnpm"}` — the
`Package` field is unused anywhere in the codebase; this is the same
dead-data shape cycle 5 addressed).

**Chain safety:** Low risk. Any machine where pnpm is declared will
have npm (npm provider is the host). If npm is missing, the npm
provider surfaces its own error first (or would if we extended npm —
but npm is out of scope per Option C). Worst case: the script
fails with "npm: command not found" and the existing "bootstrap
script failed" error surfaces.

### `duti` — extend

**Script:** `brew install duti` (macOS-only package manager).
**Chain safety:** Requires brew. Brew is the other bootstrap host
(cycle 5). Thanks to the DAG resolver, brew bootstraps first; by the
time duti's Bootstrap runs, brew is on PATH (or duti is itself skipped
via the W2 skip-cascade from cycle-5 post-archive). No new chain
risk beyond what cycle 5 already validated.

### `mas` — extend

**Script:** `brew install mas`. Same logic as duti.

### `ansible` — extend, with the trickiest install-chain

**Script:** `pipx install --include-deps ansible`.

**Why pipx over pip:**

- `pip install ansible` pollutes the system site-packages, which is
  flagged by PEP 668 on modern Python installations (Debian 12+,
  macOS via brew-python). Users hit an error like "externally-managed
  environment."
- `pipx` creates an isolated venv per app, which is the Python
  community's accepted answer for installing apps from PyPI.

**Prerequisite:** pipx itself must be installed. On Debian 12+:
`apt install pipx`. On macOS: `brew install pipx`. The `BootstrapRequiredError`
for ansible SHOULD surface this caveat in the error body.

**Alternative considered:** wrapping the script as
`pipx install --include-deps ansible || (apt install -y pipx && pipx install --include-deps ansible)`.
Rejected because: (a) assumes apt, (b) requires sudo, (c) wraps two
provisions in one opaque script that's hard for a user to audit
when the TTY prompt shows `.Script`. Better to keep the script
single-purpose and surface the pipx prerequisite in the error body.

### Skipped providers (with explicit reasoning)

- `npm`, `cargo`, `goinstall`, `uv`: language runtime. User-owned
  decision; hams doesn't have the right abstraction to pick nvm vs
  fnm vs n vs volta for node, rustup vs distro for Rust, etc. Note:
  Homebrew's install script IS a curl-to-bash; the precedent is not
  "hams runs curl-to-bash" but "hams runs curl-to-bash for ONE
  well-defined, widely-trusted case (brew)." Language runtimes
  don't meet the "widely-trusted one-liner" bar.
- `vscodeext`: requires a GUI app install. Integration test already
  uses the Microsoft apt repo + a root-safe wrapper; no one-liner
  exists that doesn't require Docker-like isolation.
- `apt`: platform-gated (`runtime.GOOS == "linux"`). Converting the
  error shape would mislead — it's not a "binary missing" signal.
- `defaults`: platform-gated (macOS-only). Same reasoning as apt.
- `git`: pre-installed on any machine that ran the hams curl-installer
  (the installer itself uses git). No plausible fresh-machine case
  where git is missing.

## Integration test scope

Deferred to the implementation session. Two candidate approaches:

1. **Unit tests only** (cycle 5 precedent). 16 unit tests covered
   every branch of cycle 5's consent matrix. For this cycle: per
   provider, one test asserting `Bootstrap` returns
   `BootstrapRequiredError` with the correct `.Script` when the
   binary is missing. ~4 tests, ~40 LoC.

2. **Integration tests** (one per extended provider). Expensive:
   4 new Dockerfile variants that start without the target binary.
   Each exercises `hams apply --bootstrap` end-to-end. Probably
   ~5 minutes CI time per variant. Total: +20 minutes to the itest
   matrix for coverage that unit tests already give.

Recommendation: **unit tests only.** Matches cycle 5's scope decision
for the same reason: the orchestration is fully covered at the unit
level, and the main brew integration test still validates the cross-
cutting RunBootstrap + BashScriptRunner path end-to-end.

## Tradeoffs

| Aspect | Chosen | Alternative | Why chosen |
|---|---|---|---|
| Scope breadth | 4 providers | 11 providers (B) | Install chain fragility outweighs completeness |
| Install for ansible | `pipx` | `pip --break-system-packages` | PEP 668 + isolated venvs = better long-term |
| Skip vs extend for `npm`/`cargo`/`go`/`uv` | Skip | Extend with detected-installer | hams shouldn't own the nvm/fnm decision |
| Extended integration tests | Unit tests only | Per-provider Dockerfile variant | Cycle 5 precedent; unit tests cover the branch |
| Session timing | Defer implementation | Ship this session | Architect flagged reactive-cycle risk after 5 cycles in one session |

## Out of Scope (explicit deferrals)

- **Auto-installing pipx itself** if ansible's bootstrap finds it
  missing. Meta-bootstrap is chain-depth-2. Handle in a follow-up
  cycle that generalizes chain-depth handling if it becomes a
  real user issue.
- **npm/cargo/go/uv runtime bootstrapping.** Needs a separate
  product decision (which installer per runtime?) that shouldn't be
  rushed.
- **vscodeext bootstrapping.** Requires GUI install; ties into
  macOS app management that isn't in scope for hams today.

## Agent-team position papers (verbatim)

### Architect role (Option C — Narrow Extension, 4 providers)

> The `DependOn.Script` field and `RunBootstrap` primitive are
> already generic — the cost is the trust audit, not the code.
> Adding each new provider is ~15 LoC. What actually gates this is:
> does the declared script have a reasonable success rate on a fresh,
> unattended machine? That's a per-provider judgment call. Four
> providers pass the test (`pnpm`: `npm i -g pnpm`; `duti`/`mas`:
> `brew install X`; `ansible`: `pipx install --include-deps ansible`).
> Eight don't: language runtimes are user decisions about shell env;
> `vscodeext` needs a GUI app; `apt`/`defaults` are platform gates.
>
> Option A (do nothing) silently betrays Philosophy #4 on exactly
> the paths cycle 5 was supposed to unblock. A user who typed `hams
> apply --bootstrap --from-repo=...` gets brew installed, then gets
> a plain-text `duti not found in PATH` — especially galling because
> brew IS now installed AND the fix is literally one `brew install`
> away. Leaving `pnpm`'s dead `DependsOn: [{Provider: "npm", Package:
> "pnpm"}]` (no `Script:` field set!) is a smaller-but-identical
> version of the exact bug cycle 5 addressed — the
> "spec-lies-about-capability" Provenance note applies verbatim.
>
> **Do not chain a sixth cycle reactively.** File it as
> `openspec/changes/extend-bootstrap-to-chainable-providers/` with a
> stub proposal; pick it up fresh next session with a clear head.

### User role (Option B — Extend everywhere, with guardrails)

> The scavenger hunt IS the bug. The whole sales pitch of hams is
> "one command restores my environment." The moment I hit `npm not
> found`, I'm off-reservation: tab-switching to a node install guide,
> picking nvm vs apt vs nodesource, re-running hams, hitting pnpm,
> tab-switching again. Four context switches at 11pm on a laptop I
> haven't even configured my terminal on yet. If hams can't deliver
> *one command*, its core promise is theater.
>
> I already consented with `--bootstrap`. That flag is not ambient
> — I typed it. Consent is the line between "silently runs arbitrary
> network commands" (bad) and "does what I told it to" (fine).
> Homebrew already proves the pattern works.
>
> Chain depth is a design constraint, not a veto: hams should pin
> the chain (npm via apt's nodejs, pipx via apt's python3-pip,
> rustup via curl-to-sh) and own those choices in specs. If the
> chain breaks, I want hams to own the breakage and ship a fix —
> not punt it back to me.

### Synthesis

Architect's C scope (4 providers) is accepted because:

- The user's B requires hams to own runtime-installer choices (nvm
  vs fnm vs apt) that are too opinionated for a single-cycle answer.
- C delivers the user's floor (structured error + --bootstrap works)
  for 4/11 providers — 4 more than today.
- C's skipped-providers rationale is concrete and auditable; future
  maintainers inherit a clear "here's why we didn't extend this one"
  rather than a vague "someday maybe."
- The architect's explicit "file and stop reactive cycling" is the
  right session-level move after 5 back-to-back cycles.
