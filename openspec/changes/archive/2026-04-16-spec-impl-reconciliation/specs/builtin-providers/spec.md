# builtin-providers — Spec Delta

## MODIFIED Requirements

### Requirement: VSCode Extension Provider — Renamed `vscode-ext` → `code-ext` (CLI name only)

The VSCode Extension provider's `Manifest().Name` and CLI verb SHALL use the string `code-ext` (NOT `vscode-ext`). The `FilePrefix` SHALL remain `vscodeext` for historical reasons — early v1 shipped `vscodeext.hams.yaml` as the canonical filename before the CLI name was finalized, and renaming the prefix now would invalidate every existing user's `vscodeext.hams.yaml` file and `.state/<machine-id>/vscodeext.state.yaml` path.

This is an intentional CLI-name-vs-file-prefix divergence. Users type `code-ext` at the CLI; the provider writes `vscodeext.*.yaml` on disk. Both are documented in the provider's `code-ext.mdx` doc page.

The display name SHALL be `VS Code Extensions` (note the space; previous spec wrote `VSCode Extension` without space).

#### Scenario: Install a VS Code extension uses code-ext CLI name

- **WHEN** the user runs `hams code-ext install ms-python.python`
- **THEN** the provider SHALL execute `code --install-extension ms-python.python`, record it in `<profile>/vscodeext.hams.yaml`, and update `<store>/.state/<machine-id>/vscodeext.state.yaml`.

#### Scenario: Provider table reflects code-ext CLI name with divergent file prefix

- **WHEN** a developer reads `openspec/specs/builtin-providers/spec.md` table of 15 builtin providers
- **THEN** row 9 SHALL list `code-ext` (NOT `vscode-ext`) as the CLI name
- **AND** the file column SHALL list `vscodeext.hams.yaml` (NOT `code-ext.hams.yaml`) to document the intentional divergence.

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
