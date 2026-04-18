# Delta — code-standards

## ADDED

### Requirement: Provider Name & Display-Name Constants

Every builtin provider SHALL extract its `Manifest.Name` and `Manifest.DisplayName` string literals into package-level `const cliName` and `const displayName` (or equivalent named constants) whenever either literal appears ≥3 times in the same file — satisfying the `goconst` linter without nolint directives.

The established precedent is `internal/provider/builtin/apt/apt.go:22` (`const cliName = "apt"`) and `internal/provider/builtin/git/git.go:18` (`const configProviderName = "git-config"`). Every provider added after 2026-04-16 SHALL follow this convention from the start.

#### Scenario: goconst passes without nolint directives

- **WHEN** `task lint:go` is run after adding or modifying a provider file
- **THEN** `goconst` SHALL report zero issues for `Manifest.Name`, `Manifest.DisplayName`, `FilePrefix`, `Provider.Name()`, and `Provider.DisplayName()` literals
- **AND** no `//nolint:goconst` suppression directive SHALL appear in the file

### Requirement: Taskfile `check` Scope Matches Description

The `task check` command SHALL execute only host-local checks — formatter, linter, unit tests. It SHALL NOT transitively invoke any task that requires `act`, Docker, or a containerized package manager.

The rationale: `task check` is the pre-commit hook and the baseline verification step. If it requires Docker/act, every contributor hits a "command not found" wall before they can verify their change. Integration and e2e suites have their own dedicated tasks (`task ci:itest:run`, `task test:e2e:one`) for contributors who need them.

#### Scenario: check passes on a developer machine without act installed

- **WHEN** a developer without `act` in PATH runs `task check`
- **THEN** the command SHALL complete successfully (exit code 0) after running fmt + lint + unit tests
- **AND** the `test:integration` and `test:e2e` steps SHALL NOT be invoked
