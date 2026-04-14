# E2E Verification & Test Infrastructure Hardening

## Context
User requested comprehensive verification of unit tests and E2E tests, with fixes for any issues found. Key requirements:
- Per-distro hamsfiles (not shared) for different Linux distributions
- Real provider execution testing (apt on Debian, etc.)
- Provider runtime bootstrapping (sudo root detection)
- Codex code review cycles until clean

## Completed
- [x] Fix `sudo.go` — DI-injectable `isRoot` for testable root detection (uid 0)
- [x] Fix `sudo_test.go` — tests both root and non-root branches via `isRoot` override
- [x] Create `e2e/lib/assertions.sh` — shared assertion helpers with `grep -qF`, subshell `cd`, `printf`
- [x] Create per-distro fixture hamsfiles:
  - [x] `e2e/fixtures/debian-store/test/{apt,bash,git-config}.hams.yaml`
  - [x] `e2e/fixtures/alpine-store/test/{bash,git-config}.hams.yaml`
  - [x] `e2e/fixtures/openwrt-store/test/{bash,git-config}.hams.yaml`
- [x] Create per-distro test scripts:
  - [x] `e2e/debian/run-tests.sh` — tests apt + bash + git-config
  - [x] `e2e/alpine/run-tests.sh` — tests bash + git-config
  - [x] `e2e/openwrt/run-tests.sh` — tests bash + git-config
- [x] Update all Dockerfiles — per-distro scripts, lib copy, consolidated chmod layers
- [x] Keep Debian apt lists for apt provider E2E testing
- [x] Remove orphaned `e2e/run-tests.sh`
- [x] Verify all unit tests pass (33 packages, 0 failures)
- [x] Verify all E2E tests pass (debian, alpine, openwrt)
- [x] Verify integration tests pass in Docker (33 packages, 0 failures)
- [x] Codex code review cycle 1 — fixed: subshell cd, grep -qF, DI isRoot, Dockerfile layers
- [x] Codex code review cycle 2 — fixed: parallel-safety comment, exact length assertions, printf, orphan removal
- [x] Codex code review cycle 2 verdict: clean, no critical or important issues remaining
