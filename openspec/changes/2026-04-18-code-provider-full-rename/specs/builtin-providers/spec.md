# Spec delta: builtin-providers — VS Code extension provider name canonicalization

## MODIFIED Requirement: VS Code Extensions provider identity

The VS Code extensions provider SHALL expose a single name across:

- `Manifest().Name` = `"code"` (registry key).
- `Manifest().FilePrefix` = `"code"` (hamsfile basename: `code.hams.yaml`; state file: `code.state.yaml`).
- CLI verb = `hams code …` (no separate handler wrapper).
- Default provider priority list entry = `"code"`.

No compatibility shim is required because hams has not formally released; there is no `vscodeext.hams.yaml` in the wild to migrate.

#### Scenario: first-time user runs `hams code install publisher.ext`

- **Given** a pristine machine
- **When** the user runs `hams code install vscode-icons-team.vscode-icons`
- **Then** the store scaffolds at `${HAMS_DATA_HOME}/store/` (per auto-init), a `default/code.hams.yaml` is written with the extension recorded under `packages:`, state is persisted at `.state/<machine-id>/code.state.yaml`, and the real `code --install-extension` runs.

#### Scenario: user runs `hams apply --only=code`

- **Given** a store with a recorded VS Code extension
- **When** the user runs `hams apply --only=code`
- **Then** the registry resolves the filter to the single provider whose `Manifest().Name` == `"code"`; the apply path runs against `code.hams.yaml` and writes `code.state.yaml`.

## REMOVED Requirement: VS Code CLI handler wrapper

**Reason:** The `CodeHandler` wrapper existed only to project the legacy `Manifest.Name = "code-ext"` onto the user-facing verb `hams code`. With `Manifest.Name` renamed to `"code"`, the wrapper is redundant. Its removal eliminates the code path where the CLI verb and the registry key could drift.
