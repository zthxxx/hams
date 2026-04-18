# Tasks: docs sync to `dev` state

## 1. Scope audit (read-only)

- [x] `rg -n 'code-ext|vscodeext\.hams|vscodeext\.state' docs/content/ openspec/specs/ README*.md AGENTS.md` — zero stale hits in the in-scope paths (all remaining hits are inside `openspec/changes/archive/**` and `internal/**` legacy-helper comments).
- [x] `rg -n '\-\-profile' docs/content/ README*.md AGENTS.md` — 7 hits; each is either `--tag` (canonical) or `--profile` as legacy alias.
- [x] `rg -n 'git-config|git-clone' docs/content/ README*.md AGENTS.md` — hits are all legitimate references to the underlying provider manifest names / hamsfile + state file paths (kept for back-compat), NOT user-facing CLI verbs. No rewrite needed.
- [x] `rg -n '\bact\b|nektos' docs/content/ README*.md AGENTS.md` — only `--no-act` in `apt.mdx` (apt-get dry-run flag, NOT `nektos/act`). No rewrite needed.

## 2. en docs sweep

- [x] `docs/content/en/docs/cli/index.mdx` — replace `--profile=<tag>` row with `--tag=<name>` row + alias note.
- [x] `docs/content/en/docs/cli/apply.mdx` — add a conflict-error bullet near the `--tag` flag row.
- [x] `docs/content/en/docs/cli/global-flags.mdx` — extend the `--tag` row + auto-init section with dry-run, ctx-timeout, identity-seeding notes + conflict-error note.
- [x] `docs/content/en/docs/quickstart.mdx` — extend the "no interactive prompts" paragraph with the `profile_tag + machine_id` seed detail.
- [x] `docs/content/en/docs/providers/git.mdx` — add "Passthrough for unmanaged subcommands" subsection with `hams git log --oneline` example.

## 3. zh-CN docs sweep

- [x] `docs/content/zh-CN/docs/cli/index.mdx` — mirror the en `--tag` + alias note.
- [x] `docs/content/zh-CN/docs/cli/apply.mdx` — mirror the conflict-error bullet.
- [x] `docs/content/zh-CN/docs/cli/global-flags.mdx` — mirror auto-init dry-run / ctx-timeout / identity-seeding + conflict-error.
- [x] `docs/content/zh-CN/docs/quickstart.mdx` — mirror the seeding note.
- [x] `docs/content/zh-CN/docs/providers/git.mdx` — mirror the passthrough subsection.

## 4. README sweep

- [x] `README.md` — one-liner in the features list that mentions `hams git <verb>` passthrough.
- [x] `README.zh-CN.md` — mirror.

## 5. AGENTS.md

- [x] Mark `docs-sync` complete with the landed commit SHAs + the scope of file touches.

## 6. Verification

- [x] `bun markdownlint-cli2 "docs/**/*.md" "docs/**/*.mdx" "README*.md"` — passes (zero warnings).
- [x] `task fmt` — passes (no files changed beyond the ones in this sweep).
- [x] Spot-check: read the top + bottom of every edited `.mdx` to confirm frontmatter, headings, and closing code-fences still balance.
