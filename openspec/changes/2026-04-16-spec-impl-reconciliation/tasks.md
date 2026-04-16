# Tasks — 2026-04-16-spec-impl-reconciliation

## Naming reconciliation: `vscode-ext` → `code-ext`, `go` → `goinstall`

- [ ] Replace in `openspec/specs/builtin-providers/spec.md` (lines 23, 25, 41, 1588, 1629-1632, 1640, 2464, 2475, 2510, 2543).
- [ ] Replace in `openspec/specs/provider-system/spec.md` (lines 83, 154-156, 171-172, 286-288, 386, 418).
- [ ] Replace in `openspec/specs/schema-design/spec.md` (lines 49, 207, 236).
- [ ] Rename `docs/content/en/docs/providers/go.mdx` → `goinstall.mdx`; update internal references (lines 32, 72) + CLI examples (15, 18, 83).
- [ ] Rename `docs/content/en/docs/providers/vscode-ext.mdx` → `code-ext.mdx`; update internal references (18, 21, 24, 33, 35, 77).
- [ ] Update `docs/content/en/docs/providers/_meta.ts` provider keys.
- [ ] Update `docs/content/en/docs/providers/index.mdx` (lines 20, 36).
- [ ] Update `docs/content/en/docs/cli/apply.mdx` (line 82).
- [ ] Mirror all the above for `docs/content/zh-CN/docs/`.
- [ ] Update `README.md` line 47 (15 builtin providers list).
- [ ] Update `AGENTS.md` line 71 (15 builtin providers list).
- [ ] Update `CLAUDE.md` line 71 (15 builtin providers list — same content as AGENTS.md).
- [ ] Final grep: zero `vscode-ext` or "go" provider references outside `openspec/changes/archive/`.

## Lucky enrichment defer

- [ ] Modify `openspec/specs/cli-architecture/spec.md` `--hams-lucky` section to mark the user-facing scenarios as v1.1-deferred and explicitly state the current shipped behavior (flag parses but is silently dropped at provider boundary; no provider implements `Enricher`).
- [ ] Add a `## DEFERRED` section to the spec delta documenting WHY (saves scaffolding for v1.1, no removal needed).

## Verification

- [ ] `task check` passes (zero issues).
- [ ] Atomic commit per logical sub-change (naming, lucky-defer, doc renames).
