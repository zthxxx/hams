---
description: Core development process principles for hams project
globs: ["**/*"]
---

# Development Process Principles

- **CLAUDE.md is a map, not an encyclopedia**: keep it under 200 lines, pointing to `docs/`, `openspec/`, `.claude/rules/` for depth. Each layer exposes only its own information plus navigation to the next level.

- **The repo is the single system of record**: knowledge not in the repo does not exist for agents. Discussions, mental decisions, external documents — if they affect development, they MUST land as a versioned artifact inside the repo.

- **Plans are first-class artifacts**: execution plans with progress logs are versioned and centralized in `openspec/changes/<id>/tasks.md`. Complex task breakdowns go to `openspec/changes/<id>/tasks/<capability>.task.md`. Tasks can be refined and decomposed during execution — they are not fixed upfront.

- **Encode taste as rules**: prefer linters, structural tests, and CI checks over natural-language instructions. Mechanically verifiable > prose guidelines.

- **Continuous garbage collection**: pay down tech debt in small, continuous increments — never accumulate for a large cleanup. Track gaps in openspec tasks.

- **When stuck, fix the environment, not the effort**: when an agent hits difficulty, ask "what context, tool, or constraint is missing?" and record the answer in `.claude/rules/`.

- **Default language is en-US**: all files use English unless the filename contains a locale suffix (e.g., `*.zh-CN.*` like `README.zh-CN.md`).

- **Universal secret decoupling**: all token/key/credential values MUST be stored in OS keychain (via keyring, `kind: application password`) or in `*.local.*` config files (which are gitignored). Secret values SHALL never appear in git-tracked config files. This applies to notification tokens (Bark, Discord), LLM API keys, and any future integration credentials.

- **Frequent atomic commits**: during implementation, commit after each independent task/feature is complete. Every commit should be a coherent, revertable unit. Never batch unrelated changes into one commit.

- **TDD with real-environment safety**: always write unit tests and E2E tests alongside implementation, not after.
  - Providers that interact with real package managers (brew, pnpm, apt, etc.) MUST be tested inside Docker containers to avoid corrupting the host machine.
  - When Docker infrastructure is not yet ready, use these **safe local test packages** for manual verification:
    - `brew`: `bat`
    - `pnpm`: `serve`
    - `bash`: `git config --global rerere.autoUpdate true`
  - Prioritize implementing the smallest verifiable slice first.
