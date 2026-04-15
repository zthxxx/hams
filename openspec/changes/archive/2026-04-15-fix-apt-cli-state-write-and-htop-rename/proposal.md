# Proposal: Apt CLI Imperative State Write + Rename Example bat → htop

## Why

Two related issues surfaced while validating the dev sandbox:

1. **Imperative apt CLI does not update state.** When the user runs
   `hams apt install <pkg>` or `hams apt remove <pkg>` inside the
   `basic-debian` sandbox, the package is installed/removed on the host,
   but `<store>/.state/<machine-id>/apt.state.yaml` is **never touched**.
   `hams refresh` also does not pick up the new package because its probe
   only iterates resources already present in the state file. The user's
   mental model — "imperative install records the install" — is broken
   without an extra explicit `hams apply` step.

   The current spec (`builtin-providers/spec.md`, "State ownership"
   section, line 907) intentionally forbids CLI handlers from writing
   state. That separation made sense for a strict
   intent-vs-reconciliation split, but it produces a UX surprise: the
   same CLI command that mutated the host produces no audit trail in
   state until the user remembers to run `apply`.

2. **`bat` is unreliable as the canonical example apt package.** `bat`
   has surfaced install issues in some Debian repo configurations.
   `htop` is universally available across Debian/Ubuntu releases and
   makes for a more dependable example in docs, sandbox demos, and E2E
   tests.

## What Changes

### Behavior change (apt provider)

The `hams apt install` and `hams apt remove` CLI handlers SHALL load
(or initialize) `apt.state.yaml`, transition the relevant resource via
`state.SetResource(...)`, and `Save` the file atomically — alongside
the existing hamsfile mutation. The load → mutate → save cycle is the
same pattern the executor uses; it is just invoked from the CLI handler
in addition to the executor.

After this change:

- `hams apt install pkg` → state has `pkg: state=ok, first_install_at, updated_at, version`.
- `hams apt remove pkg` → state has `pkg: state=removed, first_install_at preserved, removed_at, updated_at`.
- A subsequent `hams apply` is still valid (it re-probes and re-saves
  the same fields) but is no longer required for the state to reflect
  the imperative action.

### Scenario rename: bat → htop

Across all builtin-providers / dev-sandbox / schema-design scenarios,
docs, examples, README, and the apt unit + E2E test fixtures, replace
the example package `bat` with `htop`. Keep `bat` only where it is
genuinely tied to its own ecosystem (cargo docs — `bat` is a Rust
tool — and the cargo `cargo install --list` parser test).

### New E2E coverage

Add a CLI-only scenario in `e2e/debian/assert-apt-imperative.sh` that
mirrors the user's manual sandbox flow: install an existing tracked
package (verifies `updated_at` bumps), install a brand-new package
(verifies state row is created), remove that package (verifies
`removed_at` is set), all **without** invoking `hams apply` between
steps.

## Impact

- **Affected specs**:
  - `builtin-providers/spec.md` — apt CLI install/remove SHALL write state; rename `bat` → `htop` in scenarios.
  - `dev-sandbox/spec.md` — rename `bat` → `htop` in the dev sandbox `apt apply` scenario.
  - `schema-design/spec.md` — rename `bat` → `htop` in the "Remove transitions record removed_at" scenario.
- **Affected code**:
  - `internal/provider/builtin/apt/apt.go` — CLI handlers now load/save state.
  - `internal/provider/builtin/apt/apt_test.go` — new state-write unit tests; rename existing `bat` fixtures to `htop`.
- **Affected E2E**:
  - `e2e/debian/assert-apt-imperative.sh` — rename `bat` → `htop`; add CLI-only scenarios.
- **Affected docs / examples**:
  - `examples/basic-debian/{store/dev/apt.hams.yaml, state/sandbox/apt.state.yaml, README.md}` — rename `bat` → `htop`.
  - `README.md`, `README.zh-CN.md` — rename in install example.
  - `docs/content/{en,zh-CN}/docs/**/*.mdx` — rename `bat` → `htop` wherever shown as a brew/apt example. Cargo docs keep `bat` (genuine Rust tool).
- **Affected agent / process docs**:
  - `.agents/rules/development-process.md` — replace `brew: bat` test package with `brew: htop`.

## Additional scope added mid-change: per-provider integration tests + apply/refresh scope gate

### Per-provider integration test restructure

The `run_apt_cli_only_flow` shell section was originally bolted onto
`e2e/debian/assert-apt-imperative.sh`, which conflates two different
concerns: (a) apt-provider integration testing, and (b) the Debian
cross-provider OS smoke test (`hams apply --from-repo` + bash +
git-config + store-config E4 + schema migration E5).

Restructure:

- Each builtin provider SHALL own its integration test under
  `internal/provider/builtin/<provider>/integration/`, with its own
  `Dockerfile` + `integration.sh`. One container per provider, zero
  test-runtime contamination across providers.
- Shared base image `hams-itest-base` (debian-slim + ca-certs, curl,
  bash, git, sudo, yq; NO language/runtime toolchains) lives at
  `e2e/base/Dockerfile`. Each provider overlay is a minimal `FROM
  hams-itest-base:<hash>` that typically adds nothing — every
  provider's integration.sh installs whatever runtime hams needs as
  part of its own test, because that is what hams must do in real
  user scenarios.
- Shared shell helpers at `e2e/base/lib/{assertions,yaml_assert,provider_flow}.sh`.
  The new `provider_flow.sh` exposes `standard_cli_flow` — the
  canonical install-existing → re-install → install-new → refresh →
  remove sequence that every provider integration test calls.
- Two-tier docker cache keyed on input-file SHAs (same pattern as
  existing `ci:e2e:run`): base image rebuilt only when
  `e2e/base/Dockerfile` changes; per-provider image rebuilt only when
  its `integration/Dockerfile` changes. `docker image inspect` gates
  skip the rebuild on hash match.
- Taskfile entries: `ci:itest:base`, `ci:itest:run PROVIDER=<name>`
  (direct docker, invoked by CI workflow and local dev),
  `test:itest:one PROVIDER=<name>` (through `act`, isomorphic with
  CI). GitHub Actions matrix job over in-scope providers.

**In scope for integration-test coverage** (11 linux-containerizable
providers): apt, ansible, bash, cargo, git (config + clone),
goinstall, homebrew, npm, pnpm, uv, vscodeext.

**Out of scope** (macOS-only providers): defaults, duti, mas. No docker
path exists for these; a future change can add a macOS-runner path
if/when we decide how to fund it.

Reference implementation: the apt provider migrates its existing
CLI-only flow into the new layout as part of this change. The
remaining 10 in-scope providers are tracked as individual follow-up
tasks; each is small once the pattern and shared helpers are in
place.

### Apply/refresh scope gate (two-stage filter)

Today `hams apply` / `hams refresh` dispatch to every registered
provider unconditionally. If the machine has no `Homebrew.hams.yaml`
and no `Homebrew.state.yaml`, the Homebrew provider's `Bootstrap`
still runs and may error (brew may not even be installed). Worse,
in integration tests, one provider's test would trip other providers'
bootstrap — forcing every test container to install every runtime.

New behavior:

- **Stage 1 (artifact presence)**: `runApply` and `runRefresh` SHALL
  include a provider only if at least one of the following exists
  under the resolved store/profile/machine paths:
  - `<profile>/<FilePrefix>.hams.yaml` (or `.hams.local.yaml`)
  - `.state/<machine>/<FilePrefix>.state.yaml`
- **Stage 2 (`--only` / `--except`)**: after stage 1 narrows the set,
  the existing `--only` / `--except` flags further filter within
  stage 1's result. `--only` does NOT bypass stage 1; it narrows it.
  If stage 1 yields the empty set, the command logs "no providers
  match" and exits 0 (no-op, not an error).
- Skipped providers log at debug level only; the command's summary
  counts remain accurate (skipped-by-scope-gate is not the same as
  skipped-by-plan).

This makes `hams apply` / `hams refresh` safe to run on machines that
only need a subset of providers, and makes per-provider integration
tests possible without forcing every test container to install every
provider's runtime.

## Out of Scope

- Generalizing the state-write behavior to other providers (brew,
  pnpm, cargo, etc.). Those providers retain the current spec'd
  behavior (CLI mutates hamsfile; apply reconciles state). A
  follow-up change can roll the same pattern out provider-by-provider.
- Changing `hams refresh` to discover packages declared in hamsfile
  but missing from state. With install now writing state, the
  imperative path no longer relies on refresh for discovery; manual
  hamsfile edits still require `hams apply`.
- Integration tests for macOS-only providers (defaults, duti, mas).
- Running integration tests on a macOS runner for homebrew (linuxbrew
  path only; macOS-specific brew flows remain covered only by unit
  tests + manual verification until a macOS CI runner lands).
