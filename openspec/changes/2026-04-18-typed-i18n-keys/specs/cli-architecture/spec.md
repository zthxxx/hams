# Spec delta: cli-architecture — typed i18n message-key catalog

## ADDED Requirement: message IDs SHALL be declared as typed constants

`internal/i18n/keys.go` SHALL declare every message ID used by the CLI as an exported `const` string. Every call-site SHALL reference the typed constant (`i18n.T(i18n.GitUsageHeader)`) rather than a string literal (`i18n.T("git.usage.header")`). Literal-string message IDs MAY appear only in unit tests that intentionally exercise the unknown-key fallback.

The test package SHALL include a `TestCatalogCoherence_EveryTypedKeyResolves` (or equivalently named) test that reads `locales/en.yaml` and `locales/zh-CN.yaml` and asserts every typed constant has a translation entry in both files. Adding a new constant without extending the catalogue test + the YAML files fails CI.

#### Scenario: developer adds a new error path with a new message

- **Given** a developer adds `const CLIErrBadExample = "cli.err.bad-example"` to `keys.go`
- **When** the developer runs `task test:unit`
- **Then** the catalog-coherence test fails until translations are added to both `en.yaml` and `zh-CN.yaml` AND the constant is added to the hand-maintained list in `TestCatalogCoherence_EveryTypedKeyResolves`.

#### Scenario: developer typos a message ID at a call-site

- **Given** `i18n.T(i18n.GitUsageHeader)` — valid constant reference
- **And** a sibling call `i18n.T(i18n.GitUsageHaeder)` — typo
- **When** the developer runs `go build`
- **Then** compilation fails with `undefined: GitUsageHaeder`.
