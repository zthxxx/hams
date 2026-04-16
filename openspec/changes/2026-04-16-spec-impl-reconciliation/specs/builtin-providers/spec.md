# builtin-providers — Spec Delta

## MODIFIED Requirements

### Requirement: VSCode Extension Provider — Renamed `vscode-ext` → `code-ext`

The VSCode Extension provider's `Manifest().Name`, `FilePrefix`, CLI verb, and state-file path SHALL all use the string `code-ext` (NOT `vscode-ext`). The rename reconciles the spec with the impl shipped in `internal/provider/builtin/vscodeext/vscodeext.go` since v1; renaming the impl would invalidate every existing user's `code-ext.hams.yaml` file and `.state/<machine-id>/code-ext.state.yaml` path.

The display name SHALL remain `VS Code Extensions` (note the space; previous spec wrote `VSCode Extension` without space).

#### Scenario: Install a VS Code extension uses code-ext name

- **WHEN** the user runs `hams code-ext install ms-python.python`
- **THEN** the provider SHALL execute `code --install-extension ms-python.python`, record it in `code-ext.hams.yaml`, and update `code-ext.state.yaml`.

#### Scenario: Provider table reflects code-ext name

- **WHEN** a developer reads `openspec/specs/builtin-providers/spec.md` table of 15 builtin providers
- **THEN** row 9 SHALL list `code-ext` (NOT `vscode-ext`) as the provider name
- **AND** the file column SHALL list `code-ext.hams.yaml` (NOT `VSCode Extension.hams.yaml`).

### Requirement: Go Install Provider — Renamed `go` → `goinstall`

The Go install provider's `Manifest().Name`, `FilePrefix`, CLI verb, and state-file path SHALL all use the string `goinstall` (NOT `go`). The rename reconciles the spec with the impl shipped in `internal/provider/builtin/goinstall/goinstall.go` since v1.

Rationale for `goinstall` over `go`:

1. The verb form `goinstall` matches the imperative form `go install`, distinguishing it from the `go` toolchain itself.
2. Avoids ambiguity with `hams go ...` being parsed as "use the go toolchain" by users.
3. Matches the file-prefix convention (`goinstall.hams.yaml`) already shipped in user stores.

The display name SHALL be `go install` (with space).

#### Scenario: Install a Go module uses goinstall name

- **WHEN** the user runs `hams goinstall install github.com/example/tool@latest`
- **THEN** the provider SHALL execute `go install github.com/example/tool@latest`, record it in `goinstall.hams.yaml`, and update `goinstall.state.yaml`.

#### Scenario: Provider table reflects goinstall name

- **WHEN** a developer reads `openspec/specs/builtin-providers/spec.md` table of 15 builtin providers
- **THEN** row 7 SHALL list `goinstall` (NOT `go`) as the provider name.
