# provider-system — Spec Delta

## MODIFIED Requirements

### Requirement: DAG Resolution Examples — `code-ext` and `goinstall` naming

All DAG resolution examples and the `DefaultProviderPriority` example list SHALL use the impl names `code-ext` (NOT `vscode-ext`) and `goinstall` (NOT `go`).

#### Scenario: Provider list reflects impl names

- **WHEN** a reader inspects `provider-system/spec.md` enumerations of registered providers
- **THEN** the list SHALL read `Homebrew, pnpm, npm, uv, goinstall, cargo, mas, apt, code-ext` (NOT `..., go, ..., vscode-ext`).

#### Scenario: DAG dependency examples use impl names

- **WHEN** the spec illustrates `code-ext` declaring `depends_on: brew`
- **THEN** the executable order in the example SHALL be `bash → brew → code-ext` (NOT `bash → homebrew → vscode-ext`).
- **AND** the example MAY use either `brew` or `homebrew` (impl exposes both as `Provider.Name() = "brew"` with `DisplayName() = "Homebrew"`).
