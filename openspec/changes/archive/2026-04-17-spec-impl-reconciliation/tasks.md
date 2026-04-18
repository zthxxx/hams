# Tasks — 2026-04-16-spec-impl-reconciliation

Status: all sub-changes landed ahead of this tasks file being ticked off.
Verification pass at 2026-04-17 confirmed zero stale references via grep
across the listed files; checkboxes updated retrospectively.

## Naming reconciliation: `vscode-ext` → `code-ext`, `go` → `goinstall`

- [x] Replace in `openspec/specs/builtin-providers/spec.md` — all hits gone (`grep vscode-ext` returns 0).
- [x] Replace in `openspec/specs/provider-system/spec.md` — clean.
- [x] Replace in `openspec/specs/schema-design/spec.md` — clean.
- [x] Rename `docs/content/en/docs/providers/go.mdx` → `goinstall.mdx`; internal references + CLI examples updated.
- [x] Rename `docs/content/en/docs/providers/vscode-ext.mdx` → `code-ext.mdx`; internal references updated.
- [x] Update `docs/content/en/docs/providers/_meta.ts` provider keys.
- [x] Update `docs/content/en/docs/providers/index.mdx`.
- [x] Update `docs/content/en/docs/cli/apply.mdx`.
- [x] Mirror all the above for `docs/content/zh-CN/docs/`.
- [x] Update `README.md` builtin-provider list.
- [x] Update `AGENTS.md` builtin-provider list.
- [x] Update `CLAUDE.md` builtin-provider list (symlink → AGENTS.md).
- [x] Final grep: zero `vscode-ext` or bare `go` provider references outside `openspec/changes/archive/` and this change dir itself (verified 2026-04-17).

## Lucky enrichment defer

- [x] Modify `openspec/specs/cli-architecture/spec.md` `--hams-lucky` section to mark user-facing scenarios as v1.1-deferred (commit `f4c0f20`). See lines 660–690, 758–760.
- [x] Add the `## DEFERRED` section to the spec describing WHY (saves scaffolding for v1.1, no removal needed).

## Verification

- [x] `task check` passes (zero issues) — verified 2026-04-17, all 32 packages PASS with `-race`.
- [x] Atomic commits per logical sub-change: naming reconciliation (`6f9e533`), lucky-defer (`f4c0f20`).
