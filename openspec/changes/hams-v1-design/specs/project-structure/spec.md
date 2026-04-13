# Project Structure Spec

<!-- openspec:change = hams-v1-design -->
<!-- openspec:capability = project-structure -->

This spec defines the Go module layout, directory conventions, build targets, Docker-based e2e testing infrastructure, and GitHub Actions CI pipeline for the hams project.

---

## ADDED Requirements

### Requirement: Top-Level Directory Layout

The repository root SHALL organize files into well-defined top-level directories following the [Standard Go Project Layout](https://github.com/golang-standards/project-layout) conventions and Go community best practices. The layout MUST separate application entry points (`cmd/`), private application code (`internal/`), public SDK (`pkg/`), documentation (`docs/`), and JS tooling concerns into distinct top-level directories.

The canonical top-level layout SHALL be:

```
hams/
  cmd/                    # Application entry points (one sub-dir per binary)
  internal/               # Private Go packages (compiler-enforced, not importable externally)
  pkg/                    # Public Go packages (importable by external provider authors)
  docs/                   # Nextra documentation site (JS/TS, separate concern)
  e2e/                    # Docker Compose + Dockerfiles for end-to-end testing
  scripts/                # Build/install/release helper scripts (bash)
  .github/                # GitHub Actions workflows, issue templates, PR templates
  openspec/               # OpenSpec specification artifacts
  bin/                    # Build output (.gitignore'd)
  Taskfile.yml            # go-task task definitions
  go.mod / go.sum         # Go module definition
  package.json            # JS tooling dependencies (pnpm)
  .golangci.yml           # golangci-lint v2 config
  lefthook.yml            # Git hooks
  cspell.yaml             # Spell checking
  .markdownlint.yaml      # Markdown linting
  eslint.config.js        # JS/TS linting (flat config)
  .editorconfig           # Editor settings
  .gitignore              # Git ignore rules
  .gitattributes          # Git attributes
  AGENTS.md               # Agent instructions (CLAUDE.md symlinks here)
  CLAUDE.md -> AGENTS.md  # Symlink for Claude Code
```

**Rationale**: `internal/` provides compiler-enforced encapsulation. `pkg/` signals public API surface for external provider authors. `e2e/` isolates Docker infrastructure from unit tests. `scripts/` keeps shell helpers out of the Go source tree. `docs/` contains the Nextra site which is a separate JS project with its own dependencies.

#### Scenario: Repository root contains expected directories

- **WHEN** a contributor clones the repository
- **THEN** the top-level directories `cmd/`, `internal/`, `pkg/`, `e2e/`, `scripts/`, `.github/`, and `openspec/` SHALL exist
- **AND** `docs/` SHALL exist when the documentation site capability is implemented
- **AND** `bin/` SHALL NOT be committed to version control

#### Scenario: No Go source files at repository root

- **WHEN** inspecting the repository root
- **THEN** there SHALL be zero `.go` files at the root level
- **AND** all Go source code SHALL reside under `cmd/`, `internal/`, or `pkg/`

---

### Requirement: Go Module Identity and Version

The Go module SHALL be named `github.com/zthxxx/hams` and SHALL require Go 1.24 or later as specified in `go.mod`. The module path MUST match the GitHub repository URL to enable standard `go get` and `go install` workflows.

#### Scenario: Module path matches repository

- **WHEN** `go.mod` is parsed
- **THEN** the `module` directive SHALL be `github.com/zthxxx/hams`
- **AND** the `go` directive SHALL specify `1.24` or later

---

### Requirement: Application Entry Point Structure

The `cmd/` directory SHALL contain exactly one subdirectory per build target binary. For v1, the only binary is `hams`. The entry point `cmd/hams/main.go` SHALL contain only Uber Fx bootstrap wiring and MUST NOT contain business logic. All application logic SHALL be imported from `internal/` or `pkg/` packages.

```
cmd/
  hams/
    main.go               # Fx bootstrap: fx.New(...), app.Run()
```

#### Scenario: main.go contains only DI wiring

- **WHEN** `cmd/hams/main.go` is reviewed
- **THEN** it SHALL import and wire Uber Fx modules from `internal/` packages
- **AND** it SHALL NOT contain any business logic, CLI flag parsing, or provider implementations
- **AND** the `main()` function body SHALL consist of `fx.New(...)` module composition and lifecycle start

#### Scenario: Single binary target

- **WHEN** the `cmd/` directory is listed
- **THEN** it SHALL contain exactly one subdirectory named `hams`
- **AND** `cmd/hams/` SHALL contain a `main.go` file with `package main`

---

### Requirement: Internal Package Organization

The `internal/` directory SHALL organize private application packages by domain concern. Each subdirectory SHALL represent a single cohesive domain with well-defined interfaces. Packages MUST follow dependency inversion: depend on interfaces, not concrete implementations.

The canonical `internal/` layout SHALL be:

```
internal/
  cli/                    # urfave/cli command definitions, global flag parsing, command routing
  config/                 # hams.config.yaml loading, .local.yaml merge, profile resolution
  state/                  # State file read/write, lock manager (PID+cmd), baseline tracking
  hamsfile/               # Hamsfile read/write with YAML comment preservation, SDK for providers
  provider/
    registry.go           # Provider registration, DAG resolution, lifecycle management
    interface.go          # Provider interface (Probe, Apply, Remove, List, Enrich)
    hook.go               # Hook execution engine (pre/post, defer, nested calls)
    builtin/              # All builtin provider implementations (one subdir per provider)
      bash/
      homebrew/
      apt/
      pnpm/
      npm/
      uv/
      goinstall/
      cargo/
      vscodeext/
      git/
      defaults/
      duti/
      mas/
  tui/                    # BubbleTea alternate screen, progress, collapsible logs, popup
  notify/                 # Notification channels (terminal-notifier, Bark)
  otel/                   # OTel setup, span helpers, local file exporter
  i18n/                   # Locale detection, message catalog, translation loading
  logging/                # Structured logging, log rotation, session log management
  urn/                    # URN parsing and validation (urn:hams:<provider>:<id>)
  sudo/                   # Sudo credential caching, elevation helpers
  selfupdate/             # Self-upgrade logic (GitHub Releases / brew detection)
```

#### Scenario: Each internal package has a single responsibility

- **WHEN** an `internal/` subdirectory is examined
- **THEN** it SHALL have a clearly defined domain boundary documented in its package comment
- **AND** it SHALL NOT import sibling packages in a circular manner (the Go compiler enforces this)

#### Scenario: Provider interface is defined separately from implementations

- **WHEN** `internal/provider/interface.go` is examined
- **THEN** it SHALL define Go interfaces for the provider contract (`Provider`, `Prober`, `Applier`, `Remover`)
- **AND** no builtin provider package SHALL be imported from `interface.go`
- **AND** provider registration SHALL use the Uber Fx dependency injection container

#### Scenario: Internal packages are not importable externally

- **WHEN** an external Go module attempts to import `github.com/zthxxx/hams/internal/...`
- **THEN** the Go compiler SHALL reject the import with a build error
- **AND** only packages under `pkg/` SHALL be importable by external consumers

---

### Requirement: Builtin Provider Directory Structure

Each builtin provider SHALL reside in its own subdirectory under `internal/provider/builtin/`. Each provider directory SHALL contain at minimum a provider implementation file and a test file. Providers that include embedded scripts or assets SHALL place them in a subdirectory with Go embed directives.

```
internal/provider/builtin/
  bash/
    bash.go               # Provider implementation
    bash_test.go           # Tests
  homebrew/
    homebrew.go
    homebrew_test.go
  apt/
    apt.go
    apt_test.go
  pnpm/
    pnpm.go
    pnpm_test.go
  npm/
    npm.go
    npm_test.go
  uv/
    uv.go
    uv_test.go
  goinstall/
    goinstall.go
    goinstall_test.go
  cargo/
    cargo.go
    cargo_test.go
  vscodeext/
    vscodeext.go
    vscodeext_test.go
  git/
    git.go                # Covers both git-config and git-clone resource types
    git_test.go
  defaults/
    defaults.go
    defaults_test.go
  duti/
    duti.go
    duti_test.go
  mas/
    mas.go
    mas_test.go
```

**Rationale**: The package name `goinstall` (not `go`) avoids shadowing the `go` builtin. The `vscodeext` name is concise and avoids hyphens (invalid in Go package names). The `git` package handles both `git config` and `git clone` resource types because they share the same underlying tool dependency.

#### Scenario: Each builtin provider is an isolated package

- **WHEN** a new builtin provider is added
- **THEN** it SHALL be placed in its own subdirectory under `internal/provider/builtin/`
- **AND** the directory name SHALL be a valid Go package name (lowercase, no hyphens, no underscores)
- **AND** it SHALL contain at minimum `<name>.go` and `<name>_test.go`

#### Scenario: Provider packages do not import each other

- **WHEN** any builtin provider package is examined
- **THEN** it SHALL NOT import any other builtin provider package
- **AND** shared logic SHALL be factored into `internal/provider/` or `pkg/sdk/` packages
- **AND** inter-provider dependencies SHALL be expressed through the DAG `depend-on` mechanism, not Go imports

#### Scenario: Provider with embedded assets

- **WHEN** a provider requires embedded scripts or templates (e.g., bash provider with helper scripts)
- **THEN** it SHALL use Go `//go:embed` directives to embed assets from a subdirectory
- **AND** embedded assets SHALL reside within the provider's own directory subtree

---

### Requirement: Public SDK Package

The `pkg/sdk/` directory SHALL contain the public Go SDK for external provider authors. This package SHALL export the provider interface types, resource identity types (URN), common helpers, and any types needed to implement a provider via `hashicorp/go-plugin`.

```
pkg/
  sdk/
    provider.go           # Exported Provider interface, resource types
    urn.go                # URN type and helpers (re-exported from internal/urn)
    resource.go           # Resource identity, status types
    plugin.go             # go-plugin handshake, GRPCProvider interface
```

#### Scenario: External provider authors can import the SDK

- **WHEN** an external Go module adds `github.com/zthxxx/hams/pkg/sdk` as a dependency
- **THEN** it SHALL compile successfully
- **AND** the SDK SHALL expose all types needed to implement a provider plugin

#### Scenario: SDK types are stable and versioned

- **WHEN** the SDK package is published
- **THEN** exported types SHALL follow Go API compatibility promises
- **AND** breaking changes SHALL only occur with a major version bump of the module

---

### Requirement: Separation of Go Code and JS Tooling

Go application code and JS/TS tooling (documentation site, linting configs, future Bun SDK) SHALL be cleanly separated. The Go module SHALL NOT depend on any JS/TS files for compilation. JS tooling SHALL be managed by pnpm and executed via bun. The two ecosystems SHALL share only the repository root for configuration files.

```
# Go ecosystem
cmd/          internal/          pkg/          go.mod          go.sum

# JS ecosystem
docs/         package.json       pnpm-lock.yaml     eslint.config.js

# Shared (configuration only)
.editorconfig     .gitignore     .gitattributes     cspell.yaml
```

#### Scenario: Go build does not require Node.js or Bun

- **WHEN** `go build ./cmd/hams` is executed on a system without Node.js, Bun, or pnpm
- **THEN** the build SHALL succeed
- **AND** no JS/TS files SHALL be referenced by Go source code

#### Scenario: JS tooling does not affect Go compilation

- **WHEN** `node_modules/` is deleted or `pnpm install` has not been run
- **THEN** `go build`, `go test`, and `golangci-lint run` SHALL all succeed
- **AND** only `lint:md`, `lint:spell`, and `docs/` operations SHALL require JS tooling

---

### Requirement: Build Targets and Static Linking

The hams binary SHALL be built as a statically linked executable with `CGO_ENABLED=0` for all target platforms. This ensures the binary runs on minimal environments (Alpine musl, OpenWrt, fresh macOS installs) without dynamic library dependencies.

The v1 build matrix SHALL include:

| GOOS    | GOARCH | Target Environment              |
|---------|--------|---------------------------------|
| `darwin`  | `arm64`  | Apple Silicon macOS (M1/M2/M3/M4/M5) |
| `linux`   | `amd64`  | Debian, Ubuntu, Alpine (x86_64)  |
| `linux`   | `arm64`  | ARM64 OpenWrt, Raspberry Pi, ARM Linux servers |

The build command SHALL inject version metadata via `-ldflags`:

```bash
CGO_ENABLED=0 GOOS=${os} GOARCH=${arch} go build \
  -ldflags "-s -w -X github.com/zthxxx/hams/internal/version.Version=${VERSION} \
                    -X github.com/zthxxx/hams/internal/version.Commit=${COMMIT} \
                    -X github.com/zthxxx/hams/internal/version.Date=${DATE}" \
  -o bin/hams-${os}-${arch} \
  ./cmd/hams
```

#### Scenario: Static binary with no dynamic dependencies

- **WHEN** the hams binary is built with `CGO_ENABLED=0`
- **THEN** `ldd` (Linux) or `otool -L` (macOS) SHALL report no dynamic library dependencies (or "not a dynamic executable")
- **AND** the binary SHALL execute on a minimal Alpine container without installing additional libraries

#### Scenario: Cross-compilation for all targets

- **WHEN** the build matrix is executed
- **THEN** it SHALL produce valid executables for `darwin/arm64`, `linux/amd64`, and `linux/arm64`
- **AND** each binary SHALL report the correct `GOOS`/`GOARCH` when queried via `hams version`

#### Scenario: Version metadata is embedded at build time

- **WHEN** `hams version` is executed
- **THEN** it SHALL display the Git tag version, commit SHA, and build date
- **AND** these values SHALL be injected via `-ldflags` at build time, not hardcoded

#### Scenario: Version package exists for ldflags injection

- **WHEN** the `internal/version/` package is examined
- **THEN** it SHALL contain exported `var` declarations for `Version`, `Commit`, and `Date`
- **AND** these variables SHALL have default values of `"dev"`, `"unknown"`, and `"unknown"` respectively for local development builds

---

### Requirement: Taskfile Build and Development Tasks

The `Taskfile.yml` SHALL define all common development tasks using [go-task](https://taskfile.dev/). Tasks SHALL cover setup, building (local and cross-compilation), testing, linting, formatting, and cleaning.

The following tasks SHALL be defined:

| Task | Description |
|------|-------------|
| `setup` | Install all dev tools (pnpm install, golangci-lint, goimports, lefthook) |
| `build` | Build binary for current platform to `bin/hams` |
| `build:all` | Cross-compile for all target platforms (darwin/arm64, linux/amd64, linux/arm64) |
| `build:release` | Build all targets with version ldflags from Git tag |
| `run` | Run the application locally |
| `test` | Run tests with `-race -count=1 -coverprofile` |
| `test:cover` | Open coverage report in browser |
| `test:e2e` | Run Docker-based e2e tests via `docker compose` |
| `lint` | Run all linters (Go + Markdown + cspell) |
| `lint:go` | Run golangci-lint |
| `lint:go:fix` | Run golangci-lint with auto-fix |
| `lint:md` | Run markdownlint |
| `lint:spell` | Run cspell |
| `fmt` | Format Go source (gofmt + goimports) |
| `tidy` | Run `go mod tidy` |
| `check` | Run fmt, lint, test in sequence (full pre-push check) |
| `clean` | Remove `bin/`, coverage files, and Docker e2e artifacts |

#### Scenario: Local build produces platform-native binary

- **WHEN** `task build` is executed
- **THEN** it SHALL produce `bin/hams` built for the current `GOOS`/`GOARCH`
- **AND** the binary SHALL be executable on the current machine

#### Scenario: Cross-compilation builds all targets

- **WHEN** `task build:all` is executed
- **THEN** it SHALL produce `bin/hams-darwin-arm64`, `bin/hams-linux-amd64`, and `bin/hams-linux-arm64`
- **AND** all three binaries SHALL be statically linked with `CGO_ENABLED=0`

#### Scenario: Release build includes version metadata

- **WHEN** `task build:release` is executed
- **THEN** it SHALL read the version from the latest Git tag (or `dev` if no tag exists)
- **AND** it SHALL pass version, commit SHA, and build date via `-ldflags` to all cross-compiled binaries

#### Scenario: E2E test task orchestrates Docker Compose

- **WHEN** `task test:e2e` is executed
- **THEN** it SHALL run `docker compose -f e2e/docker-compose.yml up --build --abort-on-container-exit`
- **AND** it SHALL exit with a non-zero code if any e2e test container fails

---

### Requirement: Docker Compose E2E Testing Infrastructure

The `e2e/` directory SHALL contain a Docker Compose configuration and Dockerfiles for end-to-end testing across target environments. E2E tests SHALL validate that the statically linked hams binary operates correctly on each target OS, including provider interactions, state management, and Hamsfile read/write.

```
e2e/
  docker-compose.yml      # Orchestrates all e2e test containers
  debian/
    Dockerfile            # Debian-based e2e test environment
  alpine/
    Dockerfile            # Alpine (musl libc) e2e test environment
  openwrt/
    Dockerfile            # OpenWrt-like ARM64 environment (uses alpine + busybox)
  fixtures/               # Shared test Hamsfiles and configs
    test-store/
      macOS/
        Homebrew.hams.yaml
      linux/
        apt.hams.yaml
  scripts/
    run-e2e.sh            # Test runner script executed inside containers
```

#### Scenario: Debian container validates full install flow

- **WHEN** the Debian e2e container starts
- **THEN** it SHALL copy in the pre-built `hams-linux-amd64` binary
- **AND** it SHALL execute `hams apply` against test fixtures
- **AND** it SHALL verify that apt packages listed in fixtures are installed
- **AND** the container SHALL exit with code 0 on success, non-zero on failure

#### Scenario: Alpine container validates musl compatibility

- **WHEN** the Alpine e2e container starts
- **THEN** it SHALL verify the statically linked binary executes without glibc
- **AND** it SHALL run basic `hams version`, `hams apply`, and state file operations
- **AND** YAML comment preservation SHALL be validated via round-trip tests on fixture files

#### Scenario: OpenWrt-like container validates ARM64 minimal environment

- **WHEN** the OpenWrt e2e container starts
- **THEN** it SHALL use a minimal ARM64-compatible base image (alpine with busybox)
- **AND** it SHALL validate that hams starts, reads config, and manages state
- **AND** it SHALL NOT require any pre-installed package manager beyond what OpenWrt provides

#### Scenario: E2E fixtures are version-controlled

- **WHEN** the `e2e/fixtures/` directory is examined
- **THEN** it SHALL contain representative Hamsfiles for testing
- **AND** fixtures SHALL include YAML comments to validate comment preservation
- **AND** fixtures SHALL NOT contain real credentials, API keys, or machine-specific paths

---

### Requirement: GitHub Actions CI Pipeline

The GitHub Actions CI pipeline SHALL run on every push to `main` and every pull request targeting `main`. The pipeline SHALL validate code quality, build correctness, and test coverage across all supported platforms.

The CI workflow SHALL define the following jobs:

| Job | Runner | Purpose |
|-----|--------|---------|
| `lint` | `ubuntu-latest` | Run golangci-lint v2 |
| `lint-markdown` | `ubuntu-latest` | Run markdownlint-cli2 |
| `lint-spell` | `ubuntu-latest` | Run cspell |
| `test` | `ubuntu-latest` | Run `go test -race` with coverage upload |
| `build` | matrix: `ubuntu-latest`, `macos-latest` | Cross-compile for all targets, verify binary artifacts |
| `e2e` | `ubuntu-latest` | Run Docker Compose e2e tests (depends on `build`) |

#### Scenario: Lint job catches Go code issues

- **WHEN** a PR contains Go code changes
- **THEN** the `lint` job SHALL run golangci-lint with the project's `.golangci.yml` config
- **AND** the job SHALL fail if any linter reports an error

#### Scenario: Build job produces all target binaries

- **WHEN** the `build` job runs
- **THEN** it SHALL cross-compile with `CGO_ENABLED=0` for `darwin/arm64`, `linux/amd64`, and `linux/arm64`
- **AND** it SHALL upload all binaries as workflow artifacts for downstream jobs
- **AND** the `macos-latest` runner SHALL natively build and test `darwin/arm64`

#### Scenario: E2E job validates Docker-based tests

- **WHEN** the `e2e` job runs (after `build` completes)
- **THEN** it SHALL download the `linux/amd64` binary artifact from the `build` job
- **AND** it SHALL execute `docker compose -f e2e/docker-compose.yml up --build --abort-on-container-exit`
- **AND** the job SHALL fail if any e2e test container exits with a non-zero code

#### Scenario: Test job uploads coverage

- **WHEN** the `test` job completes successfully
- **THEN** it SHALL upload `coverage.out` as a workflow artifact
- **AND** the test command SHALL use `-race -count=1` flags

#### Scenario: CI uses consistent Go version

- **WHEN** any CI job sets up Go
- **THEN** it SHALL use Go version `1.24` matching the `go.mod` requirement
- **AND** the Go version SHALL be defined as a workflow-level variable or matrix parameter to avoid drift

---

### Requirement: Gitignore Conventions

The `.gitignore` file SHALL exclude all generated artifacts, local-only configuration, runtime state, and environment-specific files. The ignore rules SHALL be organized by category with comments.

The following patterns MUST be ignored:

| Category | Patterns | Rationale |
|----------|----------|-----------|
| Build output | `bin/`, `dist/`, `*.exe`, `*.dll`, `*.so`, `*.dylib` | Generated binaries |
| Test artifacts | `coverage.out`, `coverage.html`, `*.test` | Generated test output |
| Go vendoring | `vendor/` | Not used (module mode) |
| Node.js | `node_modules/`, `*.tsbuildinfo`, `.eslintcache` | JS dependency tree |
| Local config | `*.local.*` | Machine-specific overrides (e.g., `hams.config.local.yaml`) |
| Environment | `.env`, `.env.local`, `.env.*.local` | Secrets and env vars |
| Runtime state | `.state/` | Terraform-style state directory |
| IDE | `.idea/`, `.vscode/`, `*.swp`, `*.swo`, `*~` | Editor-specific files |
| OS | `.DS_Store`, `Thumbs.db` | OS metadata |
| Debug | `__debug_bin*`, `*.log` | Debug binaries and log files |
| go-task | `.task/` | Task runner cache |
| Playwright | `.playwright-cli/` | Browser automation screenshots |

#### Scenario: State directory is never committed

- **WHEN** `.gitignore` is parsed
- **THEN** it SHALL contain a rule matching `.state/` to prevent committing Terraform-style state files
- **AND** state files generated during local development SHALL NOT appear in `git status`

#### Scenario: Local config overrides are excluded

- **WHEN** a file matching `*.local.*` exists (e.g., `hams.config.local.yaml`, `Homebrew.hams.local.yaml`)
- **THEN** it SHALL be excluded from version control by the `*.local.*` gitignore pattern
- **AND** only the non-local variants SHALL be committed

#### Scenario: Build output is excluded

- **WHEN** `task build` or `task build:all` is executed
- **THEN** all files produced in `bin/` SHALL be excluded from version control
- **AND** the `bin/` directory itself SHALL NOT be committed

---

### Requirement: Version Package for Build Metadata

An `internal/version/` package SHALL exist to hold build metadata variables that are injected at compile time via `-ldflags`. This package SHALL be the single source of truth for version information displayed by `hams version` and embedded in OTel traces.

```go
package version

// Version is the semantic version, injected at build time.
var Version = "dev"

// Commit is the git commit SHA, injected at build time.
var Commit = "unknown"

// Date is the build date in RFC3339 format, injected at build time.
var Date = "unknown"
```

#### Scenario: Local development uses default values

- **WHEN** hams is built without `-ldflags` (e.g., `go run ./cmd/hams`)
- **THEN** `hams version` SHALL display `Version: dev`, `Commit: unknown`, `Date: unknown`

#### Scenario: Release build displays injected values

- **WHEN** hams is built with `-ldflags "-X .../version.Version=v1.0.0 -X .../version.Commit=abc123 -X .../version.Date=2026-04-12T00:00:00Z"`
- **THEN** `hams version` SHALL display `Version: v1.0.0`, `Commit: abc123`, `Date: 2026-04-12T00:00:00Z`

---

### Requirement: Scripts Directory for Build Helpers

The `scripts/` directory SHALL contain shell scripts for build automation, release, and installation. These scripts SHALL be referenced by Taskfile tasks and CI workflows. All scripts MUST be POSIX-compatible (`#!/bin/sh` or `#!/usr/bin/env bash`) and MUST be executable (`chmod +x`).

```
scripts/
  install.sh              # One-line curl installer for fresh machines
  build-all.sh            # Cross-compilation loop for all targets
  release.sh              # GitHub Release creation helper (used by CI)
```

#### Scenario: Install script works on fresh machines

- **WHEN** `bash -c "$(curl -fsSL .../install.sh)"` is executed on a fresh macOS or Linux machine
- **THEN** it SHALL detect the current OS and architecture
- **AND** it SHALL download the appropriate binary from GitHub Releases
- **AND** it SHALL place the binary in a PATH-accessible location (default: `/usr/local/bin/` or `~/.local/bin/`)
- **AND** it SHALL NOT require root/sudo unless writing to a system directory

#### Scenario: Build script iterates all targets

- **WHEN** `scripts/build-all.sh` is executed
- **THEN** it SHALL loop over all GOOS/GOARCH pairs in the build matrix
- **AND** it SHALL set `CGO_ENABLED=0` for each build
- **AND** it SHALL produce named binaries in `bin/` (e.g., `hams-linux-amd64`, `hams-darwin-arm64`)

---

### Requirement: Uber Fx Module Organization

Each `internal/` package that provides injectable components SHALL export an Uber Fx module variable (conventionally named `Module`) that bundles its constructors and lifecycle hooks. The top-level `cmd/hams/main.go` SHALL compose all modules via `fx.Options(...)`.

#### Scenario: Package exports Fx module

- **WHEN** a new `internal/` package provides services (e.g., `internal/config`)
- **THEN** it SHALL export a `var Module = fx.Module("config", ...)` containing its `fx.Provide` and `fx.Invoke` registrations
- **AND** `cmd/hams/main.go` SHALL include this module in its `fx.New(...)` call

#### Scenario: Builtin providers register via Fx

- **WHEN** a builtin provider is added (e.g., `internal/provider/builtin/homebrew`)
- **THEN** it SHALL export a constructor function compatible with Fx injection
- **AND** the `internal/provider/builtin/` package SHALL aggregate all builtin provider modules into a single `BuiltinProviders` Fx option
- **AND** provider registration into the DAG SHALL occur via Fx lifecycle hooks, not init() functions

#### Scenario: No use of init() for registration

- **WHEN** any Go source file in the project is examined
- **THEN** it SHALL NOT use `func init()` for provider registration or global state mutation
- **AND** all initialization SHALL go through Uber Fx dependency injection

---

### Requirement: Go-Git Bundling for Bootstrap

The hams binary SHALL bundle [go-git](https://github.com/go-git/go-git) as a compiled-in dependency for Git operations. This ensures `hams apply --from-repo=<repo>` works on fresh machines where system `git` may not be installed. The go-git dependency SHALL be used as a fallback when system `git` is not available.

#### Scenario: Fresh machine without git

- **WHEN** `hams apply --from-repo=zthxxx/hams-store` is executed on a machine without `git` in PATH
- **THEN** hams SHALL clone the repository using the bundled go-git library
- **AND** the clone SHALL succeed for HTTPS URLs
- **AND** an informational log message SHALL indicate that bundled go-git is being used

#### Scenario: System git is preferred when available

- **WHEN** `git` is found in PATH
- **THEN** hams SHALL prefer the system `git` command for clone/pull operations
- **AND** go-git SHALL only be used as a fallback when system git is unavailable

---

### Requirement: Test File Conventions

Test files SHALL follow Go conventions: `*_test.go` co-located with the source file they test. Integration tests that require external resources (Docker, network) SHALL be guarded by build tags. Property-based testing SHALL be preferred over example-based testing.

#### Scenario: Unit tests are co-located

- **WHEN** a Go source file `foo.go` exists in a package
- **THEN** its unit tests SHALL be in `foo_test.go` in the same package
- **AND** the test file SHALL use the same `package` declaration (not `_test` external test package) unless testing exported API surface

#### Scenario: Integration tests use build tags

- **WHEN** a test requires Docker, network access, or external tools
- **THEN** it SHALL include a `//go:build integration` build tag
- **AND** `task test` SHALL NOT run integration tests by default
- **AND** a separate `task test:integration` (or explicit `-tags integration`) SHALL be required

#### Scenario: E2E tests are in the e2e directory

- **WHEN** end-to-end tests are added
- **THEN** they SHALL reside in `e2e/` (not under `internal/` or `cmd/`)
- **AND** they SHALL be executed via Docker Compose, not directly by `go test`
