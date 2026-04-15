# Design: Apt CLI Imperative State Write

## Decision

Reverse the existing "State ownership" rule in `builtin-providers/spec.md`
that forbids the apt CLI handlers from writing state. The CLI handlers
(`hams apt install`, `hams apt remove`) now own state writes for the
imperative path, in addition to the executor owning state writes for
the declarative `apply` path.

## Rationale

The current rule produces a UX surprise: `hams apt install pkg` mutates
the host but produces no audit trail in `<store>/.state/...` until the
user explicitly runs `hams apply`. Users reasonably expect the audit
trail to track the action that produced it. The strict
intent-vs-reconciliation split is a fine architectural boundary in
theory, but in practice the imperative CLI is an explicit user action
and SHOULD be reflected in state immediately.

## Approach

In `handleInstall` and `handleRemove`:

1. Run `runner.Install(ctx, pkg)` / `runner.Remove(ctx, pkg)` for each
   package, exactly as today. On error from any package, return the
   error without writing hamsfile or state (preserves current
   atomicity guarantee).
2. After all `runner` calls succeed:
   a. Load the hamsfile (existing helper `loadOrCreateHamsfile`).
   b. Load (or create) the state file. Reuse the existing
      `loadOrCreateState` pattern from `internal/provider/probe.go`.
   c. For each package, mutate the hamsfile (existing logic) AND call
      `sf.SetResource(pkg, state.StateOK | StateRemoved, ...)` with
      version captured from `runner.IsInstalled(ctx, pkg)` for the
      install path.
   d. Persist hamsfile (`hf.Write()`) and then state (`sf.Save(path)`).

## Failure Modes

- **runner.Install fails for some package mid-batch** → return the error
  without persisting any state or hamsfile change. Host has partial
  state; state file is untouched. Same as today's hamsfile behavior.
- **runner.Install succeeds, but version probe fails** → record the
  resource with empty version. Better than dropping the state row.
- **Hamsfile write fails** → return the error before saving state. State
  remains stale; the next `hams apply` (or another imperative call)
  will re-probe and reconcile. Acceptable.
- **State save fails after hamsfile write succeeds** → return the error.
  Hamsfile has the entry, state lags one step. The next apply / refresh
  reconciles. Acceptable.

## Alternatives Considered

- **Trigger a mini-apply for the touched package(s)**. Would route the
  state write through the executor (preserving the "single owner"
  property), but would re-call `runner.Install` and risk side effects
  from hooks — the executor is designed around full reconciliation,
  not single-package mutations. Rejected as more complex than the
  benefit.
- **Leave the spec as-is and document the apply step in onboarding**.
  Cleaner separation but loses imperative UX. Rejected because the
  user explicitly requested the immediate-state-write semantics.

## Cross-Provider Implications

The state-write behavior change applies to apt only. Other providers
(brew, pnpm, cargo, npm, uv, go, vscode-extension, mas, duti,
defaults, ansible, git-clone, git-config, bash) retain the current
"CLI mutates hamsfile; apply reconciles state" pattern. A follow-up
change can roll the same pattern out provider-by-provider once the apt
implementation has soaked in real use.

## Apply/Refresh Scope Gate

### Problem

Today `runRefresh` (and the apply path) calls
`provider.ProbeAll(ctx, providers, stateDir, cfg.MachineID)` against
every registered provider. ProbeAll invokes each provider's
`Bootstrap` → `Probe`, regardless of whether the machine has anything
tracked for that provider. On machines that only use apt + bash +
git-config, the Homebrew provider's `Bootstrap` still runs,
potentially fails (brew isn't installed), and produces misleading log
output.

This also makes per-provider integration testing painful: every test
container must install every provider's runtime just to prevent
`Bootstrap` failures in unrelated providers.

### Fix

Introduce a two-stage provider filter. Stage 1 (artifact-presence)
runs first and mechanically prunes providers that have no local
artifacts (no hamsfile, no state file). Stage 2 (`--only` / `--except`)
narrows within stage 1's result.

Implementation:

- Add `provider.HasArtifacts(p Provider, profileDir, stateDir string) bool`
  that returns true when at least one of these exists:
  - `<profileDir>/<FilePrefix>.hams.yaml`
  - `<profileDir>/<FilePrefix>.hams.local.yaml`
  - `<stateDir>/<FilePrefix>.state.yaml`
- In `runApply` and `runRefresh`, filter `registry.Ordered(...)` via
  `HasArtifacts` before calling `filterProviders(...)` for
  `--only`/`--except`.
- If stage 1 returns empty set, print "no providers match" and exit 0
  (no-op, not an error).
- Debug-level log lists the skipped providers for diagnostics.
- `--only=<name>` does NOT bypass stage 1. A user who explicitly names
  a provider with no artifacts gets a "no providers match" no-op. This
  preserves the principle that hams never touches a provider whose
  upstream tool might not even be on the machine.

### Cross-cutting impact

- The existing `filterProviders` helper stays unchanged; it runs on
  the stage-1-filtered slice.
- No spec change to `Bootstrap`, `Probe`, `Plan`, or `Execute` — they
  simply aren't called for pruned providers.
- Per-provider integration tests rely on this: the apt test container
  has no Homebrew hamsfile/state, so Homebrew is pruned at stage 1 and
  never tries to run linuxbrew's install script.

## Per-Provider Integration Test Layout

### Current state (before this change's late-scope addition)

- `e2e/debian/`, `e2e/alpine/`, `e2e/openwrt/` — OS-level Dockerfiles
  + `run-tests.sh` that invokes multiple providers together.
- `e2e/lib/{assertions,yaml_assert}.sh` — shared shell helpers.
- `e2e/debian/assert-apt-imperative.sh` — conflates apt-provider
  scenarios with cross-provider Debian smoke (E1–E5 are apt-specific;
  E4 is config-scope; E5 is state-schema).

### Target state

- `e2e/base/Dockerfile` — base image (`debian:bookworm-slim` + yq + sudo
  + basics). Built once per base-file hash.
- `e2e/base/lib/{assertions,yaml_assert,provider_flow}.sh` — shared
  helpers (relocated from `e2e/lib/`; the new `provider_flow.sh` hosts
  `standard_cli_flow`).
- `internal/provider/builtin/<provider>/integration/{Dockerfile,integration.sh}`
  — per-provider test artifacts. `Dockerfile` is `FROM hams-itest-base`
  with minimal delta. `integration.sh` installs the provider's runtime
  (if needed) and calls `standard_cli_flow`.
- `e2e/debian/` keeps the cross-provider OS smoke test (bootstrap from
  repo + store-config E4 + schema v1→v2 migration E5), stripped of
  apt-specific scenarios.

### Docker cache strategy

Two-tier, hash-gated, same pattern as existing `ci:e2e:run`:

```bash
BASE_HASH=$(sha256sum e2e/base/Dockerfile | head -c 12)
BASE_IMAGE="hams-itest-base:${BASE_HASH}"
docker image inspect "$BASE_IMAGE" >/dev/null 2>&1 || \
  docker build -f e2e/base/Dockerfile -t "$BASE_IMAGE" e2e/base

PROV_HASH=$(sha256sum internal/provider/builtin/${PROVIDER}/integration/Dockerfile | head -c 12)
PROV_IMAGE="hams-itest-${PROVIDER}:${PROV_HASH}"
docker image inspect "$PROV_IMAGE" >/dev/null 2>&1 || \
  docker build --build-arg BASE="${BASE_IMAGE}" \
    -f internal/provider/builtin/${PROVIDER}/integration/Dockerfile \
    -t "$PROV_IMAGE" .
```

No rebuild when hashes match. Provider Dockerfiles pin the base via
`ARG BASE=hams-itest-base:latest` + `FROM ${BASE}` so the test
runner can inject the exact base hash it just built.

### Taskfile entries

- `ci:itest:base` — direct docker, called by `ci:itest:run` (or
  standalone for dev).
- `ci:itest:run PROVIDER=<name>` — direct docker, invoked by CI
  workflow and local dev for fast feedback on a single provider.
- `ci:itest` — loop all 11 in-scope providers sequentially.
- `test:itest:one PROVIDER=<name>` — through `act`, isomorphic with
  CI (for rare cases where the developer wants the full CI runner
  simulation).

### GitHub Actions matrix

Add an `itest` job to `.github/workflows/ci.yml` with a matrix over
the 11 in-scope providers. Each matrix row calls
`task ci:itest:run PROVIDER=<name>`. Fails fast on first red provider.

## Alternatives Considered (for the new scope)

- **Keep integration tests at `e2e/<target-os>/`**: Simpler but
  conflates provider concerns with OS concerns. One red provider
  masks others; no way to run "just the cargo test".
  Rejected.
- **Pre-install every runtime in the base image**: Faster start
  (no apt-get install during each test run) but violates the principle
  that hams should install its own runtime. Also makes the base image
  enormous and couples providers that should be independent.
  Rejected.
- **One big multi-stage Dockerfile with per-provider stages**:
  Clever but brittle — changing one provider's setup triggers rebuilds
  of unrelated stages, and the Dockerfile becomes hard to read.
  Rejected.
- **Run integration tests as non-root**: Matches real user reality
  (macOS/Linux dev machines don't run as root) but adds setup
  complexity (create user, grant sudo) that doesn't test anything
  hams-specific. Tests run as root for simplicity; real-user sudo
  paths are covered by the existing sudo-isolation unit tests.
  Rejected for now.
