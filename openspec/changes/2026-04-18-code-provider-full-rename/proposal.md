# Proposal: Full `code-ext` → `code` rename (no compat layer)

## Why

The 2026-04-17 provider-unification work (`commit 05237b3`) shipped a `CodeHandler` wrapper so `hams code …` worked, but it left `Manifest().Name = "code-ext"` and `Manifest().FilePrefix = "vscodeext"` untouched. Consequences:

- `hams apply --only=code` failed because the registry filter uses `Manifest.Name` (`code-ext`), not the CLI verb. The 2026-04-17 integration test fix `f6c063d` papered over this by adding a `MANIFEST_NAME=code-ext` override in `internal/provider/builtin/vscodeext/integration/integration.sh` — symptom treatment, not a cure.
- `hamsfile` on disk is `vscodeext.hams.yaml` even though the CLI verb is `hams code`. First-time users who `ls <store>/<tag>/` see a filename that does not match what they typed.
- Registry priority list still reads `code-ext`. Docs still reference `hams code-ext install`. Spec tables still show `code-ext`. Every new reader has to mentally reconcile the divergence.

hams has not formally released, so there is no shipped `vscodeext.hams.yaml` to migrate. Every internal and external reference SHOULD be `code`. The CLAUDE.md rule — "Provider wrapped commands MUST behave exactly like the original command … at the first-level command entry point" — strongly implies the internal name should match the external name so there is no surface drift.

## What changes

1. `internal/provider/builtin/vscodeext/vscodeext.go` — `cliName = "code"`, `FilePrefix = "code"`. Error strings and doc comments updated.
2. `internal/provider/builtin/vscodeext/code_handler.go` — DELETED. The wrapper was only there to rename the CLI verb; now unnecessary.
3. `internal/cli/register.go` — `vscodeext.New(...)` moves from `applyOnlyProviders` to `cliProviders`; no more `cliOnlyHandlers` entry for vscodeext.
4. `internal/cli/root.go` — drop the `case "code-ext":` fallback in `providerUsageDescription` (the only emitter was the code-ext name).
5. `internal/config/config.go` — `DefaultProviderPriority` entry renamed `code-ext` → `code`.
6. `internal/provider/builtin/vscodeext/integration/integration.sh` — delete the `STATE_FILE_PREFIX=vscodeext` + `MANIFEST_NAME=code-ext` overrides; the defaults in `provider_flow.sh` now apply verbatim.
7. `e2e/base/lib/provider_flow.sh` — doc comments updated; the `MANIFEST_NAME` seam is kept (still needed by `git-config` / `git-clone`).
8. Tests (`vscodeext_test.go`, `plan_test.go`, `vscodeext_lifecycle_test.go`, `bootstrap_consent_test.go`, `hooks_test.go`, `root_test.go`) — fixtures + name assertions updated to `code`.
9. Docs (`docs/content/{en,zh-CN}/docs/providers/code.mdx`, `providers/index.mdx`, `quickstart.mdx`, `README.md`, `README.zh-CN.md`, `AGENTS.md`) — all user-facing references rewritten.
10. Current specs (`openspec/specs/{provider-system,builtin-providers,schema-design}/spec.md`) — tables + examples updated.

## Impact

- **Capability `builtin-providers`** — VS Code Extensions provider shedulers under the canonical `code` name across manifest, file prefix, registry key, and CLI verb. Zero divergence.
- **Capability `provider-system`** — `Manifest.Name` and `FilePrefix` MAY be equal to each other for non-case-sensitive providers (homebrew is the exception; see its own `FilePrefix: "Homebrew"`). Removes the need for the `MANIFEST_NAME` override in `standard_cli_flow`.
- **Capability `schema-design`** — hamsfile filename is `code.hams.yaml`, not `vscodeext.hams.yaml`. Default provider priority list reflects the rename.
- **User-visible** — first-time user sees `<store>/<tag>/code.hams.yaml` matching the CLI verb they typed (`hams code install …`). No more `vscodeext.hams.yaml` surprise.
- **Back-compat** — none needed (pre-release).
