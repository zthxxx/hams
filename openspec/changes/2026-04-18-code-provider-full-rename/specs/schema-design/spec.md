# Spec delta: schema-design — hamsfile + state file names for VS Code extensions

## MODIFIED Requirement: Provider hamsfile file names

The VS Code Extensions provider's hamsfile SHALL be named `code.hams.yaml` (previously `vscodeext.hams.yaml`). The state file SHALL be named `code.state.yaml` under `.state/<machine-id>/`.

Default provider priority list SHALL reference the provider by its canonical name `"code"`.

#### Scenario: first-run user on fresh machine

- **Given** a user types `hams code install vscode-icons-team.vscode-icons`
- **When** the store is auto-scaffolded
- **Then** the hamsfile written to disk is `<store>/default/code.hams.yaml` — the filename matches the CLI verb the user typed.
