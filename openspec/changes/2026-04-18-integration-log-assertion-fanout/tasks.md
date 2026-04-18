# Tasks: Fan out log-emission assertions to every provider integration test

- [x] **Helpers — port stderr-based log assertions**
  - [x] `e2e/base/lib/assertions.sh` — add `assert_stderr_contains <desc> <expected> <cmd...>` ported verbatim from `/tmp/hams-loop/e2e/base/lib/assertions.sh` (lines 33–57).
  - [x] `e2e/base/lib/assertions.sh` — add `assert_log_line <provider> <expected> <cmd...>` thin wrapper around `assert_stderr_contains` for greppable test output.
  - [x] `e2e/base/lib/assertions.sh` — add `assert_hams_apply_session_logged <provider> [args...]` framework-level helper that checks BOTH stderr and the rolling log file for `hams session started`. Not wired into any provider script; reserved for a future framework-level integration test.
  - [x] `e2e/base/lib/assertions.sh` — keep the existing file-based `assert_log_contains` + `assert_log_records_session`. They are stricter than the stderr checks and verify the slog → file handoff.

- [x] **apt — add stderr pair alongside existing file-based set**
  - [x] `internal/provider/builtin/apt/integration/integration.sh` — append `assert_stderr_contains` pair (session-start + Manifest.Name) after the existing `assert_log_records_session` / `assert_log_contains` block, before the final pass message.
  - [x] Comment marks apt as the canonical "both families" example.

- [x] **10-provider fan-out**
  - [x] `internal/provider/builtin/ansible/integration/integration.sh` — add stderr pair for `ansible` at the end.
  - [x] `internal/provider/builtin/bash/integration/integration.sh` — add stderr pair for `bash` after existing scenarios, before the final pass message. Re-creates a small marker hamsfile so the apply call has real work.
  - [x] `internal/provider/builtin/cargo/integration/integration.sh` — add stderr pair for `cargo` after `standard_cli_flow`.
  - [x] `internal/provider/builtin/git/integration/integration.sh` — add stderr pair for each sub-provider (`git-config` + `git-clone`) before the final pass message.
  - [x] `internal/provider/builtin/goinstall/integration/integration.sh` — add stderr pair for `goinstall` after `standard_cli_flow`.
  - [x] `internal/provider/builtin/homebrew/integration/integration.sh` — add stderr pair for `brew` using the existing `BREW_RUN` sudo wrapper.
  - [x] `internal/provider/builtin/npm/integration/integration.sh` — add stderr pair for `npm` after `standard_cli_flow`.
  - [x] `internal/provider/builtin/pnpm/integration/integration.sh` — add stderr pair for `pnpm` after `standard_cli_flow`.
  - [x] `internal/provider/builtin/uv/integration/integration.sh` — add stderr pair for `uv` after `standard_cli_flow`.
  - [x] `internal/provider/builtin/vscodeext/integration/integration.sh` — add stderr pair for `code` (Manifest.Name after 2026-04-18 full rename) after `standard_cli_flow`.

- [x] **AGENTS.md — mark the task done**
  - [x] Change the checklist item to `[x]` and replace the TODO bullets with a one-line summary referencing this change folder.

- [x] **Spec delta**
  - [x] `openspec/changes/2026-04-18-integration-log-assertion-fanout/specs/code-standards/spec.md` — codify the "every provider integration.sh SHALL verify log emission" rule, with a scenario that names the required helpers and the two assertion pairs.

- [x] **Verification**
  - [x] `bash -n` on every edited `.sh` file — 0 syntax errors.
  - [x] `task fmt && task lint && task test:unit` — passes; changes are additive to shell scripts + markdown only, no Go code touched.
  - [x] Docker-backed `task ci:itest:run PROVIDER=<name>` is the load-bearing check for the new assertions themselves and runs in CI; local sandboxes without docker registry access skip it per the environment rule in `.claude/rules/development-process.md`.
