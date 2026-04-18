# Proposal: Fan out log-emission assertions to every provider integration test

## Why

CLAUDE.md §Current Tasks requires: "Whether logging is emitted — for each provider as well as for hams itself — must be verified in integration tests." The 2026-04-17 onboarding change landed the core helpers (`e2e/base/lib/assertions.sh::assert_log_contains` + `assert_log_records_session`) and wired them into `internal/provider/builtin/apt/integration/integration.sh` as the canonical example. Fan-out to the remaining ten provider integration scripts was deferred and tracked as a follow-up.

Consequence today: ten provider integration scripts exercise install/remove/refresh but never assert that hams wrote a single log line. A regression that silenced slog output (wrong handler wiring, hijacked stderr, dropped file rotation) would slip through integration tests because nothing asks "did the log fire?"

## What changes

1. `e2e/base/lib/assertions.sh`:
   - ADD `assert_stderr_contains <desc> <expected> <cmd...>` — runs a command with stdout discarded, asserts the captured stderr contains a substring. Ported from the reference branch (`/tmp/hams-loop/e2e/base/lib/assertions.sh` lines 33–70).
   - ADD `assert_log_line <provider> <expected> <cmd...>` — thin wrapper around `assert_stderr_contains` that labels output by provider for greppable test logs.
   - ADD `assert_hams_apply_session_logged <provider> [args...]` — framework-level helper that runs `hams apply --only=<provider>` and asserts BOTH the rolling log file AND stderr contain the `hams session started` framework line. Not wired into any provider script yet; reserved for a future framework-level integration test.
   - KEEP the existing file-based `assert_log_contains` + `assert_log_records_session` — they verify the slog → rolling log file handoff, which is strictly stronger than stderr-only checks.
2. For each of the ten remaining provider integration scripts (`ansible`, `bash`, `cargo`, `git`, `goinstall`, `homebrew`, `npm`, `pnpm`, `uv`, `vscodeext`), ADD two stderr-based assertions at the end of the script:
   - one verifying the framework itself emits `"hams session started"` on stderr during `hams apply --only=<provider>`;
   - one verifying the provider's own Manifest.Name appears in a slog line on stderr during the same invocation.
   For the `git` package (two providers, `git-config` + `git-clone`), assertions fire for each sub-provider separately. For `vscodeext`, the Manifest.Name is `"code"` after the 2026-04-18 full rename, so the assertion key and the `--only=` filter both use `code`. For `homebrew`, the assertions run through the existing `BREW_RUN` sudo wrapper so the `HAMS_*` env vars reach the `hams` binary with the correct store / config / data home.
3. `internal/provider/builtin/apt/integration/integration.sh` — ADD the stderr-based assertion pair alongside the existing file-based assertions so apt ends up with BOTH assertion families and serves as the canonical "full coverage" example for future providers.
4. `AGENTS.md` — mark `integration-log-assertion-fanout` as `[x]` with a one-line summary of what landed.

## Impact

- **Capability `code-standards`** — gains a new requirement: every provider's `integration.sh` SHALL verify log emission on at least stderr (and SHOULD verify the rolling log file when the script is not otherwise sudo-wrapped). The OpenSpec delta under `specs/code-standards/spec.md` codifies the rule.
- **Test surface** — integration test count grows by 2 × 10 = 20 new assertions (plus 2 for the apt addition), all at the end of each script so failures are localized and do not mask earlier lifecycle regressions.
- **Local/CI isomorphism** — unaffected; the new assertions run inside the same Docker container the existing lifecycle checks run in, and use the same shared helper library (`e2e/base/lib/assertions.sh`). No Taskfile task changes.
- **Future-proofing** — `assert_hams_apply_session_logged` is the shared helper a subsequent "framework-level" integration test can call once we decide where it lives (candidate: a standalone `integration-framework/integration.sh` overlay on the `hams-itest-base` image, separate from any single provider).
- **Back-compat** — none required; these are purely additive assertions in test files.
