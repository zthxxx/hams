# Code Standards

This specification defines the Go coding conventions, architectural patterns, and quality standards for the hams project. It serves as the authoritative reference for all contributors. Every requirement here is enforced either by tooling (golangci-lint, CI) or by code review convention.

hams targets world-class open-source quality. The standards below are not aspirational guidelines; they are normative rules. Violations SHALL be caught by linters, tests, or review and SHALL NOT be merged.

---

## ADDED Requirements

### Requirement: Dependency Inversion and Inversion of Control

All major components in hams SHALL be accessed through interfaces and wired together via Uber Fx dependency injection. This ensures testability, modularity, and adherence to the Dependency Inversion Principle.

No package-level global variables SHALL be used for stateful services (loggers, config readers, state managers, notification channels, etc.). The only acceptable package-level variables are constants, sentinel errors, and compile-time interface satisfaction checks (`var _ Interface = (*Impl)(nil)`).

The `cmd/hams/main.go` entry point SHALL be the sole location where Fx modules are composed and the application graph is wired. Internal packages SHALL NOT call `fx.New` or `fx.Invoke` outside of `main.go` and dedicated Fx module constructors.

#### Scenario: Registering a new provider

WHEN a developer adds a new builtin provider (e.g., `internal/provider/builtin/cargo/`)
THEN the provider package SHALL export an `fx.Option` (conventionally named `Module`) that provides the provider's concrete type as its interface
AND the `cmd/hams/main.go` SHALL include this module in the Fx application graph
AND the provider SHALL NOT directly import or instantiate any other provider, state manager, or hamsfile writer -- it SHALL receive them as constructor parameters.

#### Scenario: Accessing a stateful service

WHEN any package needs access to a stateful service (e.g., the state manager, hamsfile writer, logger, or notification sender)
THEN it SHALL declare a dependency on that service's interface in its constructor function
AND Fx SHALL inject the concrete implementation at startup
AND the package SHALL NOT use `init()` functions to set up stateful singletons.

#### Scenario: Compile-time interface checks

WHEN a concrete type implements an interface
THEN the implementing package SHALL include a compile-time assertion: `var _ SomeInterface = (*SomeImpl)(nil)`
AND this assertion SHALL appear at the top of the file containing the concrete type, immediately after the type definition's `import` block.

---

### Requirement: Interface-Driven Design

Interfaces SHALL be defined at the consumer side (the package that depends on the behavior), not at the provider side (the package that implements the behavior). This follows the standard Go convention of "accept interfaces, return structs."

Interfaces SHALL be small and focused. An interface with more than 5 methods SHOULD be decomposed into smaller, composable interfaces unless a strong cohesion argument exists.

#### Scenario: Provider consuming state management

WHEN the `internal/provider` package needs to read and write state
THEN the `internal/provider` package SHALL define a `StateManager` interface with the methods it requires
AND the `internal/state` package SHALL implement that interface on its concrete `Manager` struct
AND the `internal/state` package SHALL NOT define the interface itself for the provider's consumption.

#### Scenario: Testing with mock implementations

WHEN writing unit tests for a package that depends on an interface
THEN the test file SHALL define or use a mock/stub implementation of the consumer-side interface
AND the test SHALL NOT require importing the concrete implementation package
AND this decoupling SHALL allow the test to run in isolation without network, filesystem, or external tool dependencies.

---

### Requirement: Error Handling Patterns

All errors SHALL be wrapped with context using `fmt.Errorf("descriptive context: %w", err)`. Raw error returns (`return err`) without wrapping SHALL NOT be used except at the immediate call site where the error originates.

Sentinel errors SHALL be used for known, expected conditions that callers need to programmatically distinguish. Sentinel errors SHALL be declared as package-level variables with the `Err` prefix (e.g., `var ErrNotFound = errors.New("resource not found")`).

Sentinel error variable names SHALL follow the `errname` linter convention: `Err` prefix for exported errors, `err` prefix for unexported errors.

#### Scenario: Wrapping an error from a subprocess call

WHEN a provider executes a subprocess (e.g., `brew install foo`) and it fails
THEN the provider SHALL wrap the error with context describing the operation: `fmt.Errorf("homebrew: install %q: %w", packageName, err)`
AND the wrapping chain SHALL be sufficient for a caller to understand what operation failed without inspecting the inner error message.

#### Scenario: Structured CLI-facing errors

WHEN an error is surfaced to the user via the CLI
THEN the error SHALL be a structured error type implementing a `UserFacingError` interface that provides:
  - a human-readable message
  - a suggested fix command (may be empty)
  - an exit code
AND the CLI layer SHALL format these errors consistently, displaying the suggestion when present
AND the error format SHALL be AI-agent friendly (parseable, with actionable suggestions).

#### Scenario: Sentinel error for resource not found

WHEN a state lookup or hamsfile query finds no matching resource
THEN it SHALL return a sentinel error (e.g., `ErrResourceNotFound`)
AND callers SHALL check for this sentinel using `errors.Is(err, ErrResourceNotFound)`
AND callers SHALL NOT compare error strings.

---

### Requirement: Package Naming Conventions

Go package names SHALL be short, lowercase, and singular nouns (e.g., `state`, `provider`, `config`, `notify`). Multi-word package names SHALL NOT use underscores or camelCase; prefer a single descriptive word or abbreviate.

All private application code SHALL reside under `internal/`. The only public Go API SHALL be under `pkg/sdk/`, which provides the SDK for external provider authors.

Test helper packages SHALL use the `_test` suffix on the package name (e.g., `package state_test`) for black-box tests, or the same package name for white-box tests that need access to unexported symbols.

#### Scenario: Adding a new internal package

WHEN a developer creates a new package for internal functionality
THEN the package SHALL be placed under `internal/`
AND the package name SHALL be a singular noun (e.g., `internal/urn`, not `internal/urns`)
AND the package name SHALL NOT repeat the parent directory path (e.g., `package urn` inside `internal/urn/`, not `package internalurn`).

#### Scenario: Exposing SDK for external providers

WHEN functionality needs to be available to external provider authors (outside the hams binary)
THEN it SHALL be placed under `pkg/sdk/`
AND the public API surface SHALL be minimal -- only types, interfaces, and functions required by the plugin contract
AND breaking changes to `pkg/sdk/` SHALL follow semantic versioning of the hams module.

---

### Requirement: Testing Standards

Property-based testing SHALL be the preferred testing approach for all logic that transforms, validates, parses, or round-trips data. The project SHALL use `pgregory.net/rapid` as the property-based testing library.

Table-driven tests SHALL be used for deterministic cases with well-defined input/output pairs (e.g., CLI flag parsing, URN validation, YAML field mapping).

Test files (`_test.go`) are exempt from `gosec`, `goconst`, and `gocritic` linters, as configured in `.golangci.yml`.

Tests SHALL be run with `-race` to detect data races. The CI pipeline SHALL enforce this.

#### Scenario: Testing YAML comment preservation round-trip

WHEN testing the hamsfile read/write module
THEN the test SHALL use property-based testing with `rapid` to generate arbitrary YAML documents with comments
AND the test SHALL assert the invariant: `parse(serialize(doc)) == doc` for all generated inputs
AND the test SHALL verify that comments survive the round-trip.

#### Scenario: Testing CLI flag parsing

WHEN testing the CLI flag parser for provider subcommands
THEN the test SHALL use table-driven tests with cases covering:
  - global flags before provider name
  - `--hams-` prefixed provider-self flags
  - `--` passthrough separator
  - `--help` interception priority
AND each test case SHALL specify input args, expected parsed result, and expected error (if any).

#### Scenario: Coverage targets

WHEN the CI pipeline runs tests
THEN the overall line coverage SHALL be at least 80%
AND critical packages (`internal/state`, `internal/hamsfile`, `internal/provider`, `internal/urn`) SHALL maintain at least 90% line coverage
AND coverage reports SHALL be generated with `-coverprofile` and archived as CI artifacts.

---

### Requirement: Import Ordering

All Go source files SHALL follow a three-group import ordering, separated by blank lines:

1. Standard library packages
2. Third-party packages
3. Local project packages (`github.com/zthxxx/hams/...`)

This ordering is enforced by `goimports` with `local-prefixes: github.com/zthxxx/hams`, configured as a formatter in `.golangci.yml`.

#### Scenario: Import block in a provider file

WHEN a developer writes a Go file that imports from all three groups
THEN the import block SHALL be formatted as:

```go
import (
    "context"
    "fmt"

    "go.uber.org/fx"

    "github.com/zthxxx/hams/internal/state"
)
```

AND `goimports` SHALL automatically enforce this grouping on `task fmt`.

---

### Requirement: Exported Symbol Documentation

Every exported function, type, method, constant, and variable SHALL have a godoc comment. The comment SHALL start with the name of the symbol and SHALL end with a period. This is enforced by the `godot` linter.

Comments SHALL describe the "what" and "why," not the "how." Implementation details belong in inline comments, not godoc.

#### Scenario: Documenting an exported function

WHEN a developer adds an exported function
THEN the function SHALL have a preceding comment in the form `// FunctionName does X and Y.`
AND the comment SHALL end with a period
AND the comment SHALL appear on the line immediately above the function signature with no blank line separating them.

#### Scenario: Documenting an exported interface

WHEN a developer defines an exported interface
THEN the interface SHALL have a godoc comment explaining its purpose and contract
AND each method in the interface SHALL have a godoc comment
AND all comments SHALL end with a period.

---

### Requirement: Context Propagation

All functions that perform I/O, call subprocesses, make network requests, or perform any operation that may block or be cancelled SHALL accept `context.Context` as their first parameter.

The context SHALL be propagated through the entire call chain from the CLI entry point down to the lowest-level I/O operation. Functions SHALL NOT create new root contexts (`context.Background()`, `context.TODO()`) except in `main.go`, top-level test functions, and explicitly documented exception points.

#### Scenario: Provider executing a subprocess

WHEN a provider calls an external CLI tool (e.g., `brew install`)
THEN the function signature SHALL be `func (p *BrewProvider) Install(ctx context.Context, pkg Package) error`
AND the subprocess SHALL be started with `exec.CommandContext(ctx, ...)`
AND cancellation of the context SHALL terminate the subprocess.

#### Scenario: Probe with timeout

WHEN the provider system probes all providers during refresh
THEN each provider probe SHALL receive a context with a timeout derived from the global apply context
AND if a provider probe exceeds its timeout, the context cancellation SHALL cause the probe to return an error
AND the error SHALL be recorded in state as a probe failure without blocking other providers.

---

### Requirement: Concurrency Patterns

Goroutines SHALL communicate via channels. Shared mutable state SHALL be protected by `sync.Mutex` or `sync.RWMutex`.

Parallel operations (e.g., provider probes) SHALL use `golang.org/x/sync/errgroup` to manage goroutine lifecycles, collect errors, and respect context cancellation.

Goroutines SHALL NOT be launched without a mechanism to observe their completion (errgroup, WaitGroup, or channel). Fire-and-forget goroutines are prohibited.

#### Scenario: Parallel provider probes

WHEN `hams refresh` probes all providers concurrently
THEN the implementation SHALL use `errgroup.WithContext` to launch one goroutine per provider
AND each goroutine SHALL respect the group's context for cancellation
AND if any probe fails, the error SHALL be collected and reported, but other probes SHALL continue (using `errgroup` with `SetLimit` or individual error handling, not `errgroup`'s default fail-fast).

#### Scenario: Protecting shared state writes

WHEN multiple goroutines may access the state manager concurrently (e.g., during parallel probes writing results)
THEN the state manager SHALL use a `sync.RWMutex` to allow concurrent reads and exclusive writes
AND all write operations SHALL hold the write lock for the minimum necessary duration.

---

### Requirement: Structured Logging

All logging SHALL use Go's `log/slog` structured logger. The `log` standard library package and `fmt.Println` for runtime output SHALL NOT be used (except in `main.go` for fatal startup errors before the logger is initialized).

Log messages SHALL use lowercase with no trailing punctuation. Structured fields SHALL use consistent key names across the codebase (e.g., `provider`, `resource`, `duration`, `error`).

`log.Fatal` and `os.Exit` SHALL NOT be called outside of `main.go`. All other code SHALL return errors to be handled by the caller.

#### Scenario: Logging a provider operation

WHEN a provider begins an install operation
THEN it SHALL log at `Info` level: `slog.InfoContext(ctx, "installing resource", "provider", p.Name(), "resource", res.ID())`
AND on failure, it SHALL log at `Error` level with the error as a structured field: `slog.ErrorContext(ctx, "install failed", "provider", p.Name(), "resource", res.ID(), "error", err)`
AND all log calls SHALL use the `Context`-suffixed variants (`InfoContext`, `ErrorContext`, etc.) to propagate trace context.

#### Scenario: Log level usage

WHEN deciding which log level to use
THEN the following conventions SHALL apply:
  - `Debug`: internal state transitions, detailed probe results, diff calculations -- useful for developer debugging
  - `Info`: significant lifecycle events (apply started, provider completed, resource installed)
  - `Warn`: recoverable anomalies (probe timeout, LLM unavailable, deprecated config field)
  - `Error`: operation failures that affect the result (install failed, state write failed)
AND `slog.Error` SHALL NOT be used for conditions that are subsequently retried and succeed.

---

### Requirement: golangci-lint v2 Configuration

The project SHALL use golangci-lint v2 with the configuration in `.golangci.yml`. The configuration uses `version: "2"` format with `gofmt` and `goimports` configured as **formatters** (not linters).

The following linter categories SHALL be enabled and SHALL NOT be disabled without a documented rationale in the PR description:

**Bug detection**: `govet` (all analyzers except `fieldalignment`), `staticcheck`, `errcheck` (with `check-type-assertions` and `check-blank`), `bodyclose`, `durationcheck`, `nilerr`, `noctx`, `rowserrcheck`, `sqlclosecheck`.

**Style and convention**: `revive` (with 20 rules including `context-as-argument`, `error-return`, `exported`, `var-naming`), `misspell` (US locale), `unconvert`, `unparam`, `usestdlibvars`, `whitespace`, `predeclared`.

**Security**: `gosec` (disabled in test files).

**Performance**: `prealloc`.

**Code quality**: `gocritic` (diagnostic + style + performance + opinionated tags), `errname`, `errorlint`, `goconst` (min-len: 3, min-occurrences: 3, disabled in test files), `godot`, `makezero`, `nakedret` (max-func-lines: 30), `nestif` (min-complexity: 6), `nolintlint` (require-explanation + require-specific), `tparallel`, `wastedassign`.

**Modernization**: `modernize`, `intrange`, `copyloopvar`.

#### Scenario: Adding a new linter

WHEN a developer proposes enabling a new linter
THEN the PR SHALL document why the linter is being added and what class of bugs or style violations it catches
AND the linter SHALL be added to the appropriate category comment in `.golangci.yml`
AND any existing violations SHALL be fixed in the same PR (not suppressed with `nolint`).

#### Scenario: Disabling a linter for the project

WHEN a developer proposes removing an enabled linter
THEN the PR description SHALL include a rationale explaining why the linter is no longer valuable
AND the change SHALL be approved by at least one maintainer.

---

### Requirement: nolint Directive Usage

Every `nolint` directive SHALL specify the exact linter name being suppressed and SHALL include an explanation. This is enforced by the `nolintlint` linter with `require-explanation: true` and `require-specific: true`.

Blanket `//nolint` (without a linter name) is prohibited. `//nolint:all` is prohibited.

#### Scenario: Suppressing a false positive

WHEN a developer needs to suppress a linter warning that is a false positive
THEN the directive SHALL be formatted as: `//nolint:lintername // explanation of why this is a false positive.`
AND the explanation SHALL be specific enough for a reviewer to verify the suppression is justified
AND the explanation SHALL end with a period.

#### Scenario: Suppressing in test files

WHEN a test file triggers `gosec`, `goconst`, or `gocritic` warnings
THEN no `nolint` directive is needed because these linters are globally excluded for `_test.go` files in `.golangci.yml`
AND developers SHALL NOT add redundant `nolint` directives for already-excluded linters.

---

### Requirement: Commit Conventions

All commits SHALL follow the [Conventional Commits](https://www.conventionalcommits.org/) specification. The commit message format SHALL be:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

Allowed types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `ci`, `perf`, `style`, `build`.

The scope SHALL be the affected package or area (e.g., `provider`, `state`, `cli`, `tui`, `hamsfile`, `ci`). Multi-scope changes SHALL use the most significant scope or omit the scope for truly cross-cutting changes.

The description SHALL be lowercase, imperative mood, and SHALL NOT end with a period.

#### Scenario: Adding a new provider

WHEN a developer commits a new builtin provider implementation
THEN the commit message SHALL be: `feat(provider): add cargo provider`
AND the body MAY include details about the probe/apply/remove implementation.

#### Scenario: Fixing a bug in state management

WHEN a developer fixes a bug where state drift was not detected
THEN the commit message SHALL be: `fix(state): detect drift when resource version changes`
AND if the fix resolves a GitHub issue, the footer SHALL include `Fixes #123`.

---

### Requirement: Function and Method Design

Functions SHALL follow the single-responsibility principle. A function that exceeds 60 lines (excluding comments and blank lines) SHOULD be decomposed into smaller functions.

Named return values SHALL NOT be used except when required for documentation clarity on interfaces with more than two return values. Naked returns are prohibited for functions longer than 30 lines (enforced by `nakedret`).

Constructor functions SHALL follow the `NewXxx` convention and SHALL return the concrete type (not an interface), per the Go proverb "return structs, accept interfaces."

#### Scenario: Constructor function for a provider

WHEN a developer creates a constructor for a provider
THEN the function SHALL be named `NewBrewProvider` (or equivalent)
AND the function SHALL return `*BrewProvider` (concrete type), not the `Provider` interface
AND Fx SHALL bind the concrete type to the interface at the module level.

#### Scenario: Deeply nested if-else chains

WHEN a function contains nested `if-else` chains
THEN the `nestif` linter SHALL flag any nesting with complexity above 6
AND the developer SHALL refactor using early returns, guard clauses, or extracted helper functions.

---

### Requirement: Type Assertion Safety

All type assertions SHALL be checked. Unchecked type assertions (`x.(T)` without the `ok` return) are prohibited. This is enforced by `errcheck` with `check-type-assertions: true`.

#### Scenario: Asserting an interface to a concrete type

WHEN code needs to assert an `interface{}` or a broader interface to a specific type
THEN the assertion SHALL use the two-value form: `val, ok := x.(T)`
AND the code SHALL handle the `!ok` case explicitly (return error, log warning, or use a default).

---

### Requirement: Resource Cleanup

All resources that implement `io.Closer` or require cleanup (file handles, HTTP response bodies, database connections, subprocess pipes) SHALL be cleaned up using `defer` immediately after successful acquisition.

`defer` SHALL NOT be used inside loops where the deferred cleanup must happen on each iteration. In such cases, explicit cleanup or an extracted function SHALL be used.

#### Scenario: HTTP response body cleanup

WHEN code makes an HTTP request
THEN `resp.Body.Close()` SHALL be deferred immediately after checking the error from `http.Do`
AND the `bodyclose` linter SHALL enforce this automatically.

#### Scenario: Subprocess pipe cleanup

WHEN a provider starts a subprocess with `exec.CommandContext`
THEN stdout/stderr pipes SHALL be properly closed after the subprocess completes
AND the function SHALL not leak goroutines reading from the pipes.

---

### Requirement: Code Review Culture

Pull requests SHALL be the sole mechanism for merging code into the main branch. Direct pushes to `main` SHALL be prohibited via branch protection rules.

Every PR SHALL pass the full CI pipeline (`task check`: format, lint, test) before merge. The CI pipeline is defined in `.github/workflows/ci.yml`.

Reviewers SHALL verify that new code adheres to all requirements in this spec. The code review checklist includes:
- Interfaces defined at consumer side
- Errors wrapped with context
- Context propagated as first parameter
- No package-level mutable globals
- Exported symbols documented with godoc ending in period
- Tests present and using property-based approach where applicable
- Imports ordered correctly
- `nolint` directives have linter name and explanation

#### Scenario: Pre-merge CI gate

WHEN a pull request is opened or updated
THEN GitHub Actions SHALL run `task check` (which executes `fmt`, `lint`, `test` sequentially)
AND the PR SHALL NOT be mergeable until all checks pass
AND the branch protection rule SHALL require at least one approving review.

#### Scenario: Lefthook pre-commit and pre-push hooks

WHEN a developer commits locally
THEN `lefthook` SHALL run `fmt` and `lint` as a pre-commit hook
AND `lefthook` SHALL run `test` as a pre-push hook
AND developers SHALL NOT bypass hooks with `--no-verify` unless explicitly justified.
