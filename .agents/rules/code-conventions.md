---
description: Go and JS code conventions for hams project
globs: ["**/*.go", "**/*.js", "**/*.ts"]
---

# Code Conventions

## Go

- Go linting is strict: golangci-lint v2 with `version: "2"` format. `gofmt` and `goimports` are configured as **formatters** (not linters) in `.golangci.yml`.
- Import grouping: stdlib, third-party, then `github.com/zthxxx/hams` (enforced by goimports `local-prefixes`).
- `nolint` directives require both a specific linter name AND an explanation.
- Comments on exported symbols must end with a period (godot).
- Test files are exempt from gosec, goconst, and gocritic.
- Dependency inversion: all major components accessed via interfaces, injected via Uber Fx. No package-level globals for stateful services. Architecture MUST use DI to isolate uncontrollable external boundaries (filesystem, network, package managers, OS APIs) so that unit tests can inject mock boundary-layer services and run without side effects.
- Context propagation: `context.Context` as first parameter for all blocking/cancellable operations.
- Error handling: wrap with `fmt.Errorf("...: %w", err)`, sentinel errors for known conditions, structured `UserFacingError` for CLI output.
- Logging: `log/slog` with structured fields. No `log.Fatal` outside `main`.
- Testing: property-based testing (using `rapid`) preferred over example-based. Table-driven for deterministic cases.
- When adding new Go tool/linter names, add them to `cspell.yaml` words list.

## JS/TS

- JS tooling runs via `bun` (not `node`/`npx`). Dependencies installed via `pnpm`.
- ESLint 9 flat config in `eslint.config.ts`.

## Full Details

See `openspec/changes/hams-v1-design/specs/code-standards/spec.md` for the comprehensive code standards specification.
