# Tasks: `code-ext` → `code` full rename

- [x] **Manifest + FilePrefix**
  - [x] `cliName = "code"` in `internal/provider/builtin/vscodeext/vscodeext.go`.
  - [x] `FilePrefix: "code"` in the Manifest.
  - [x] Rewritten doc comments that previously motivated the CLI/filename divergence.

- [x] **Registry wiring**
  - [x] `vscodeext.New(...)` moved from `applyOnlyProviders` → `cliProviders` in `internal/cli/register.go`.
  - [x] `cliOnlyHandlers` entry for vscodeext deleted.
  - [x] `internal/provider/builtin/vscodeext/code_handler.go` DELETED.

- [x] **CLI surfaces**
  - [x] `internal/cli/root.go` — `case "code-ext":` branch removed from `providerUsageDescription`.
  - [x] `internal/config/config.go` — `DefaultProviderPriority` entry `code-ext` → `code`.

- [x] **Integration test overrides**
  - [x] `internal/provider/builtin/vscodeext/integration/integration.sh` — `STATE_FILE_PREFIX` + `MANIFEST_NAME` env vars deleted.
  - [x] `e2e/base/lib/provider_flow.sh` — doc comment pared back (the `MANIFEST_NAME` seam stays for `git-config`/`git-clone`).

- [x] **Tests**
  - [x] `internal/provider/builtin/vscodeext/vscodeext_test.go` — Manifest.Name assertion + state.New fixtures.
  - [x] `internal/provider/builtin/vscodeext/plan_test.go` — URN fixtures + state.New.
  - [x] `internal/provider/builtin/vscodeext/vscodeext_lifecycle_test.go` — state.New fixtures.
  - [x] `internal/provider/builtin/vscodeext/handle_command_test.go` — hamsfile path fixture.
  - [x] `internal/cli/bootstrap_consent_test.go` — fake-provider name + FilePrefix + cascade assertions.
  - [x] `internal/hamsfile/hooks_test.go` — YAML fixture updated.
  - [x] `internal/cli/root_test.go` — non-package-description table entry.

- [x] **Docs / specs**
  - [x] `docs/content/en/docs/providers/{code.mdx,index.mdx}`.
  - [x] `docs/content/zh-CN/docs/providers/{code.mdx,index.mdx}`.
  - [x] `docs/content/{en,zh-CN}/docs/quickstart.mdx`.
  - [x] `README.md`, `README.zh-CN.md`, `AGENTS.md`.
  - [x] `openspec/specs/{provider-system,builtin-providers,schema-design}/spec.md`.

- [x] **Verification**
  - [x] `go build ./...` — 0 errors.
  - [x] `go test ./internal/...` — 34 packages PASS.
  - [x] `task fmt && task lint` — 0 issues (part of pre-commit hook).
  - [x] `task test:unit` — PASS (part of pre-commit hook).
  - [x] `rg code-ext docs/ openspec/specs/ README*.md AGENTS.md internal/` — only historical references remain (inside archived specs + analysis notes).
