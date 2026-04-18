# schema-design — Spec Delta

## MODIFIED Requirements

### Requirement: Package-class Provider Enumeration — `code-ext` and `goinstall` naming

All schema-design enumerations of Package-class providers SHALL use the impl names `code-ext` (NOT `vscode-ext`) and `goinstall` (NOT `go`).

#### Scenario: Hamsfile examples use impl provider names

- **WHEN** the spec lists supported provider keys for hamsfile entries
- **THEN** the list SHALL include `code-ext` and `goinstall` (NOT `vscode-ext` and `go`).

#### Scenario: Hook example uses impl name

- **WHEN** the spec shows a Hook example invoking the VSCode extension provider
- **THEN** the example SHALL read `hams code-ext apply` (NOT `hams vscode-ext apply`).
