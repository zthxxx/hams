# Spec delta: cli-architecture — CI verification paths

## MODIFIED Requirement: Local/CI verification isomorphism

The test-execution tasks in `Taskfile.yml` SHALL be defined so that the local default does NOT depend on `act` for integration/e2e/itest tests.

The Taskfile SHALL expose:

- `test:integration`, `test:e2e`, `test:e2e:one TARGET=<target>`, `test:itest`, `test:itest:one PROVIDER=<provider>`, `test:sudo` — each routes to the corresponding `ci:*` task executed directly against Docker with a locally-built `bin/hams-linux-amd64` (produced via the `build:linux` task dep).
- `test:e2e:one-via-act TARGET=<target>` and `test:itest:one-via-act PROVIDER=<provider>` — explicit opt-in simulation of the full GitHub Actions runner via `act`. These MAY fail on `actions/upload-artifact@v4` due to `act`'s artifact-server limitation; that failure mode SHALL be documented in the task's `desc:` string.

The `.github/workflows/ci.yml` workflow SHALL guard all `actions/upload-artifact@v4` steps with `if: ${{ !env.ACT }}` so that `act` simulation does not fail on the upload path. Jobs that depended on the uploaded artifact (`integration`, `e2e`, `itest`) SHALL fall back to rebuilding the binary in-place via `task build:linux` when `env.ACT` is set.

#### Scenario: developer runs a single-provider integration test

- **Given** a developer on a laptop with Docker available
- **When** they run `task test:itest:one PROVIDER=apt`
- **Then** the binary is built via `task build:linux`, the `ci:itest:run` task spins up the apt integration container, and the integration script executes — with no dependency on `act` and no artifact-server failures.

#### Scenario: developer needs full GitHub Actions parity

- **Given** a developer wants to reproduce the exact CI runner environment
- **When** they run `task test:itest:one-via-act PROVIDER=apt`
- **Then** `act` executes the workflow; artifact upload steps are skipped (gated by `!env.ACT`), and the job falls back to building the binary in-place inside the runner container.

#### Scenario: real GitHub Actions CI

- **Given** a push to origin
- **When** GitHub Actions runs `.github/workflows/ci.yml`
- **Then** `env.ACT` is unset, artifact uploads run normally, downstream jobs download the artifact, and the act-fallback build step is skipped.
