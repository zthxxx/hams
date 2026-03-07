---
description: Agent behavior rules for corrections, doc sync, and execution gates
globs: ["**/*"]
---

# Agent Behavior Rules

## Execution Gate

Do NOT begin implementation (code writing, scaffolding, task execution, or `/opsx:apply`) until the user **explicitly** confirms that specs are approved and execution can start. Presenting specs for review is not approval. Silence is not approval.

## Recording Corrections and Rules

When the user corrects an error, interrupts to redirect, or states a new rule:

1. **Corrections**: summarize the correction and record it in `.claude/rules/` or `.claude/` memory so it persists across sessions.
2. **Normative/principled/rule content**: add to the appropriate `.claude/rules/*.md` file.
3. **Feature or interaction changes**: if the change affects user-facing behavior, sync updates to `docs/` or `README.md` AND auto-check corresponding i18n files (e.g., `*.zh-CN.*`) for parallel updates.

## Documentation i18n Sync

When updating any documentation file (`docs/**`, `README.md`), check for locale-suffixed variants (e.g., `README.zh-CN.md`, `docs/**/*.zh-CN.*`) and update them in the same pass. Do not leave i18n files stale.
