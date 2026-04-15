# Project Structure — Delta for fix-apt-provider-and-store-config-scope

## MODIFIED Requirements

### Requirement: GitHub Actions CI Pipeline

The GitHub Actions CI pipeline SHALL run on every push to `main` and every pull request targeting `main`. The pipeline SHALL validate code quality, build correctness, and test coverage across all supported platforms.

Every workflow step that performs build, test, lint, formatting, Docker operations, or any other project action SHALL invoke a `task <name>` command via the `arduino/setup-task@v2` action. Raw invocations of `go build`, `go test`, `go vet`, `golangci-lint`, `markdownlint-cli2`, `cspell`, `pnpm`, `docker build`, `docker run`, `docker compose`, or equivalent shell commands SHALL NOT appear directly in any workflow YAML `run:` step, except for:

- `arduino/setup-task@v2` itself (bootstraps the task runner).
- Actions-provided setup steps (e.g., `actions/checkout@v4`, `actions/setup-go@v5`, `actions/upload-artifact@v4`) when no equivalent Taskfile-wrapped step exists.
- `echo` / `printenv` / trivial diagnostics (e.g., `run: task --version`) for debugging or provenance logs.

Any workflow requirement that lacks a corresponding Taskfile task SHALL first have the task added to `Taskfile.yml` (under an appropriate namespace — `ci:*` for CI-specific compositions, `test:*` / `lint:*` for reusable work) and then be invoked via `run: task <name>` from the workflow.

The CI workflow SHALL define the following jobs:

| Job | Runner | Purpose | Invoked Taskfile task(s) |
|-----|--------|---------|--------------------------|
| `lint` | `ubuntu-latest` | Run golangci-lint v2 | `task lint` (or `task ci:lint`) |
| `lint-markdown` | `ubuntu-latest` | Run markdownlint-cli2 | `task lint:md` (or equivalent) |
| `lint-spell` | `ubuntu-latest` | Run cspell | `task lint:spell` (or equivalent) |
| `test` | `ubuntu-latest` | Run `go test -race` with coverage | `task test` (or `task ci:unit`) |
| `build` | matrix: `ubuntu-latest`, `macos-latest` | Cross-compile for all targets | `task build:all` (or equivalent) |
| `integration` | `ubuntu-latest` | Run integration tests | `task ci:integration` |
| `e2e` | `ubuntu-latest` | Run Docker-based e2e tests | `task ci:e2e` |

All jobs SHALL install `task` via `arduino/setup-task@v2` before invoking any task command. The action SHALL be pinned to `@v2` (major version), not `@latest`. A workflow-level variable SHALL define the Go version (`1.24`) to match `go.mod`.

#### Scenario: Lint job catches Go code issues via Taskfile

- **WHEN** a PR contains Go code changes
- **THEN** the `lint` job SHALL install `task` via `arduino/setup-task@v2`
- **AND** SHALL execute `run: task lint` (or `run: task ci:lint` if a CI-specific composition exists)
- **AND** the job SHALL fail if any linter reports an error
- **AND** the workflow YAML SHALL NOT contain a raw `golangci-lint run` command.

#### Scenario: Build job produces all target binaries via Taskfile

- **WHEN** the `build` job runs
- **THEN** it SHALL invoke a task (e.g., `task build:all`) that cross-compiles with `CGO_ENABLED=0` for `darwin/arm64`, `linux/amd64`, and `linux/arm64`
- **AND** it SHALL upload all binaries as workflow artifacts for downstream jobs
- **AND** the workflow YAML SHALL NOT contain raw `go build` commands.

#### Scenario: E2E job validates Docker-based tests via Taskfile

- **WHEN** the `e2e` job runs
- **THEN** it SHALL execute `run: task ci:e2e`
- **AND** the task SHALL drive all Docker builds, container runs, and assertions internally
- **AND** the workflow YAML SHALL NOT contain raw `docker build`, `docker run`, or `docker compose` commands.

#### Scenario: Test job runs via Taskfile

- **WHEN** the `test` job runs
- **THEN** it SHALL execute `run: task test` (or `run: task ci:unit`)
- **AND** the task SHALL use `-race -count=1` flags internally
- **AND** the workflow YAML SHALL NOT contain raw `go test` commands.

#### Scenario: CI uses consistent Go version

- **WHEN** any CI job sets up Go
- **THEN** it SHALL use Go version `1.24` matching the `go.mod` requirement
- **AND** the Go version SHALL be defined as a workflow-level variable or matrix parameter to avoid drift.

#### Scenario: Local `task ci:integration` and CI `task ci:integration` are identical

- **WHEN** a developer runs `task ci:integration` on their local machine
- **AND** a CI workflow runs `task ci:integration` in the `integration` job
- **THEN** both invocations SHALL execute the identical Taskfile recipe with identical argument semantics
- **AND** any divergence in outcome SHALL be attributable only to environment differences (e.g., missing local Docker), never to different commands.

#### Scenario: New capability added to Taskfile before workflow uses it

- **WHEN** a contributor needs a new CI-only operation (e.g., a new linter, a new E2E variant)
- **THEN** they SHALL first add a Taskfile task (e.g., `ci:lint:new-thing`) that performs the operation locally
- **AND** the workflow step SHALL then invoke `run: task ci:lint:new-thing`
- **AND** SHALL NOT inline the operation's raw commands directly in the workflow YAML.
