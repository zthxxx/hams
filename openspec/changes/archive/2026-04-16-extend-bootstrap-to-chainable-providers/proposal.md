# Extend Bootstrap consent pattern to chainable providers (pnpm, duti, mas, ansible)

## Why

The just-archived `homebrew-bootstrap-opt-in` cycle (2026-04-16) built
a generic consent-gated bootstrap primitive:

- `provider.BootstrapRequiredError{Provider, Binary, Script}` — typed,
  wraps `ErrBootstrapRequired`, surfaces the install script verbatim.
- `provider.RunBootstrap(ctx, p, registry)` — delegates script execution
  through a registered `BashScriptRunner`.
- CLI `--bootstrap` / `--no-bootstrap` / TTY `[y/N/s]` consent gate.

Only the `homebrew` provider uses it. The other 11 builtin providers
still return plain `fmt.Errorf(...)` pointing at an install command
in a string. Result: fresh-machine users get a beautifully formatted
actionable error for missing `brew` but a plain string for missing
`pnpm`. `--bootstrap` works for brew but is a no-op for the others.

This is a **spec/code drift identical in shape to the one cycle 5 just
closed**: a declarative capability is promised by the manifest
(e.g. `pnpm.DependsOn = [{Provider: "npm", Package: "pnpm"}]`) but
the provider code ignores it and prints an error string.

An autonomous architect+user Agent debate (transcripts preserved in
`design.md`) converged on **Option C — narrow extension**:

- **Extend to 4 providers:** `pnpm`, `duti`, `mas`, `ansible`. Each
  has a reasonable, well-understood install command that succeeds
  unattended on a typical fresh machine.
- **Skip 8 providers:** `npm`, `cargo`, `goinstall`, `uv` (language
  runtime installers — user-owned decision about nvm/fnm/rustup/…);
  `vscodeext` (GUI app); `apt`/`defaults` (platform-gated, not
  missing-binary); `git` (should be pre-present).

Architect recommended: **file the proposal, stop reactive cycling,
implement in a fresh session** rather than chaining a 6th cycle
in-session on top of 5 back-to-back cycles.

## What Changes

Each target provider's `Bootstrap()` method:

- Returns `*provider.BootstrapRequiredError` (not `fmt.Errorf`) when
  its prerequisite binary is missing.
- Populates `.Script` with the install command listed below.
- Populates `.Provider` / `.Binary` per the manifest.

Each target provider's `Manifest().DependsOn[i]` gains a matching
`Script:` field so `provider.RunBootstrap` can execute it under
user consent via the already-implemented `bash.RunScript` seam.

Per-provider install script decisions:

| Provider | `Binary` | `Script` | Dep host | Rationale |
|---|---|---|---|---|
| `pnpm` | `pnpm` | `npm install -g pnpm` | bash (npm already on PATH for this path to be reachable) | Promotes existing `DependsOn: [{Provider: "npm", Package: "pnpm"}]` from dead data to executable |
| `duti` | `duti` | `brew install duti` | bash (brew already consent-gated upstream via cycle 5) | macOS-only; no chicken-and-egg because brew is the only non-brew bootstrap in the session |
| `mas` | `mas` | `brew install mas` | bash | Same as duti |
| `ansible` | `ansible-playbook` | `pipx install --include-deps ansible` | bash | pipx chosen over pip to avoid polluting system site-packages. pipx is available on Debian 12+ via `apt install pipx` and on macOS via `brew install pipx` — document the prerequisite in the error body |

Explicitly out of scope (spec scenarios document **why not**):

- `npm`, `cargo`, `goinstall`, `uv` — language runtime install is a
  user-owned decision (nvm vs fnm vs n vs volta, rustup vs distro,
  etc.). hams does not have the right abstraction to own that.
- `vscodeext` — requires a GUI app install, not a CLI install. The
  integration test already proves this (uses the Microsoft apt repo
  + root-safe wrapper, not a one-liner install).
- `apt` / `defaults` — these are platform-gated (`runtime.GOOS` check),
  not a missing-binary signal. Converting the error shape would be
  misleading.
- `git` — git is pre-installed on any machine that ran the hams
  curl-installer. No fresh-machine case where `git` is missing.
- `bash` — always present; `Bootstrap` is already a no-op.

## Impact

- **Affected specs:** `builtin-providers` (modify 4 provider sections:
  pnpm, duti, mas, ansible — each gets a new "missing-binary signals
  consent required" scenario).
- **Affected code:** 4 provider files + 4 corresponding unit-test
  files. Per architect's review: each provider is ~15 LoC of swap +
  a few tests; the architecture cost was already paid by cycle 5.
- **Integration tests:** 4 new variants (Dockerfile + integration.sh)
  starting WITHOUT the target binary, exercising `--bootstrap` and
  asserting the binary lands on PATH + the declared package gets
  installed. OR: skip integration tests per cycle 5's precedent
  (unit tests cover the orchestration; integration tests would
  re-exercise network dependencies that cycle 5 explicitly deferred).
  Defer the integration-test scope decision to `design.md`.
- **Backwards compatibility:** preserved. Default behavior is still
  "fail fast with actionable error" — we just upgrade the error
  shape from string → `BootstrapRequiredError`. `--bootstrap`
  newly works for these 4 providers; it was previously a no-op.
- **User-visible:** fresh-Linux users with `pnpm`/`ansible` hamsfile
  entries can now one-command restore with `hams apply --bootstrap`.
  Fresh-Mac users can one-command restore the full `brew → mas →
  duti` chain.

## Provenance

- Follow-up to cycle 5 (`homebrew-bootstrap-opt-in`, 2026-04-16).
  That cycle's `design.md` explicitly deferred: *"Auto-installing
  language runtimes for non-brew providers (npm, pnpm, cargo, uv,
  etc.). Those can be follow-up changes; they share this change's
  `RunBootstrap` primitive but each has its own trust / interactivity
  profile."*
- Critical-architect sweep post-cycle-5 did not flag this as a
  ship-blocker (canonical paths all pass), but flagged it as a
  consistency gap worth a new cycle.
- Architect + user Agent-team debate (2026-04-16, position papers
  preserved in `design.md`) both rejected Option A (do nothing).
  Architect: narrow C. User: broad B with guardrails. Synthesized
  position: architect's C, with explicit scenarios documenting the
  skipped 8 providers so future maintainers don't re-ask this question.

## Scope assessment

Per the architect's explicit recommendation: this is **not** a
session-stopping fix. The project is in a stable, shipped state at
commit `79c7db3`. Five cycles + reviewer passes happened this session.
**Do not chain a sixth implementation cycle reactively.** File this
proposal, pick up the implementation in a fresh session with a clear
head. The proposal + design.md + spec deltas captured here are the
execution for this session.
