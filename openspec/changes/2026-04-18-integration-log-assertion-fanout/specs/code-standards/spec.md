# Spec delta: code-standards — integration tests verify log emission for every provider

## ADDED Requirements

### Requirement: Integration tests SHALL verify log emission for every provider

Every provider's `internal/provider/builtin/<provider>/integration/integration.sh` SHALL verify that running `hams apply --only=<Manifest.Name>` (or a provider-appropriate equivalent) produces structured log output on at least one observable sink — stderr (slog console handler) and/or the rolling log file under `${HAMS_DATA_HOME}/<YYYY-MM>/hams.YYYYMM.log`. Absence of log output from either sink SHALL fail the provider's integration test.

This requirement exists because slog is the hams framework's only runtime observability surface. A silent regression — a handler that drops records, an accidentally hijacked stderr, a misrouted file sink — would slip past unit tests (which stub the logger) unless integration tests explicitly assert "logs fired." Closing that gap was tracked in CLAUDE.md §Current Tasks as: *"Whether logging is emitted — for each provider as well as for hams itself — must be verified in integration tests."*

The shared helpers in `e2e/base/lib/assertions.sh` provide both families:

- **Stderr-based** — `assert_stderr_contains <desc> <expected> <cmd...>` runs a command with stdout discarded, captures stderr, and greps for a substring. `assert_log_line <provider> <expected> <cmd...>` is a thin provider-tagged wrapper. Use when the script cannot easily resolve `${HAMS_DATA_HOME}` or when the provider is invoked through a sudo/env wrapper that would otherwise split the log file.
- **File-based** — `assert_log_contains <desc> <expected>` + `assert_log_records_session <desc>` read the most recent rolling log under `${HAMS_DATA_HOME}` and grep for a substring. Stricter than stderr checks: asserts the slog → file handoff worked. Preferred when the script controls `HAMS_DATA_HOME`.

The per-invocation framework bootstrap line is `"hams session started"`; every provider emits its own `Manifest.Name` as a slog attribute on at least one lifecycle record. Integration scripts SHALL assert both the framework line and the provider name appear on the chosen sink.

A framework-level helper `assert_hams_apply_session_logged <provider> [args...]` in the same file verifies BOTH sinks in one call. It is reserved for a future standalone framework-level integration test and is NOT required to be wired into any per-provider integration.sh.

#### Scenario: package-like provider integration test asserts log emission

- **Given** a provider such as `cargo` whose integration test drives `standard_cli_flow cargo install tokei just`
- **When** the integration script runs inside the provider's Docker overlay
- **Then** after `standard_cli_flow` completes, the script SHALL invoke `assert_stderr_contains "<provider>: hams itself emits session-start log" "hams session started" hams --store="$HAMS_STORE" apply --only=<Manifest.Name>` and `assert_stderr_contains "<provider>: provider emits slog line" "<Manifest.Name>" hams --store="$HAMS_STORE" apply --only=<Manifest.Name>`;
- **And** failure of either assertion SHALL abort the test with the recorded FAIL message.

#### Scenario: declarative-only provider integration test asserts log emission

- **Given** a provider such as `bash` whose integration test does not use `standard_cli_flow`
- **When** the integration script reaches the end of its custom lifecycle assertions
- **Then** before the final `"=== <provider> integration test passed ==="` message, the script SHALL re-declare a minimal hamsfile workload so the final `hams apply --only=<provider>` has real work to run;
- **And** the script SHALL invoke the same two `assert_stderr_contains` calls described in the package-like scenario.

#### Scenario: multi-provider package (`git`) asserts each sub-provider independently

- **Given** the `git` package shipping both `git-config` and `git-clone` providers
- **When** the integration script finishes exercising both
- **Then** the script SHALL invoke the `assert_stderr_contains` pair (framework line + Manifest.Name) once for `git-config` and once for `git-clone`, using `--only=git-config` and `--only=git-clone` respectively.

#### Scenario: apt retains both stderr and file-based assertion families

- **Given** the `apt` provider has been the canonical example since the 2026-04-17 onboarding change
- **When** the integration script reaches its logging gate
- **Then** the script SHALL invoke BOTH `assert_log_records_session` / `assert_log_contains` (file-based) AND the `assert_stderr_contains` pair (stderr-based);
- **And** either family alone would satisfy the minimum requirement, but keeping both makes `apt` the reference implementation that future providers can mirror.
