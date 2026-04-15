# Proposal: fix-apt-provider-and-store-config-scope

## Why

The apt builtin provider is currently stubbed: `hams apt install <pkg>` and `hams apt remove <pkg>` shell out to `apt-get` but do NOT update the hamsfile or the state file, silence stdout/stderr, and the state enum lacks a `removed_at` timestamp. Store-level config files additionally accept `profile_tag` and `machine_id`, which are machine-scoped and must only live in the global config — this lets a shared store repo silently overwrite every teammate's machine identity. The current behavior breaks the core "declarative serialization of host state" promise: a `hams apt install bat` on a fresh machine does not record into YAML, so `hams apply` on another machine won't reinstall it. The GitHub Actions workflow also inlines raw commands (`go test`, `golangci-lint run`, `docker build`, etc.) instead of routing through Taskfile, which creates silent drift between the `task <name>` used locally and the literal commands run in CI.

## What Changes

- **Apt provider CLI handlers persist state.** `hams apt install <pkg>` and `hams apt remove <pkg>` SHALL run the real `apt-get` command AND update the corresponding `apt.hams.yaml` (add/remove the `{app: <pkg>}` entry), mirroring the brew provider's established pattern (`homebrew.go:292-337`). Stdout and stderr SHALL stream to the user.
- **Apt provider DI seam for `apt-get`.** A new `CmdRunner` interface wraps `apt-get install`, `apt-get remove`, and `dpkg -s`. Production wires a real implementation through `sudo.CmdBuilder`; unit tests inject a fake that records calls and maintains an in-memory installed set — enabling host-safe unit tests that catch "command ran but state not updated" regressions.
- **State schema evolves to v2**. **BREAKING for on-disk state files** (migrated automatically on first read):
  - Rename `Resource.InstallAt` → `Resource.FirstInstallAt` (YAML: `install_at` → `first_install_at`).
  - `FirstInstallAt` is set once on first install and is **immutable** across re-install, upgrade, remove, and re-install-after-remove.
  - Add `Resource.RemovedAt *time.Time` (YAML: `removed_at,omitempty`). Set on transition to `StateRemoved`, cleared on any subsequent transition to `StateOK`.
  - `UpdatedAt` is bumped on every state transition.
  - `schema_version` bumps `1` → `2`. Loader performs forward-only migration for v1 files on read; rewrites as v2 on next save.
- **Store-level config rejects machine-scoped fields**. `<store>/hams.config.yaml` and `<store>/hams.config.local.yaml` SHALL fail to load if `profile_tag` or `machine_id` is set at the top level. Error message MUST name the offending field, the file path, and point the user to `${HAMS_CONFIG_HOME}/hams.config.yaml`. Example templates (`examples/.template/store/`, `examples/basic-debian/store/`) and E2E fixtures (`e2e/fixtures/*/hams.config.yaml`) are scrubbed of these fields.
- **GitHub Actions workflow routes all steps through Taskfile via `go-task/setup-task@v2`.** Every build, test, lint, integration, and e2e step in `.github/workflows/ci.yml` SHALL invoke a `task <name>` command. No inline `go build`, `go test`, `golangci-lint run`, or `docker build` commands remain in workflow YAML. Missing Taskfile tasks are added first, then referenced.
- **Unit + E2E test coverage.** Apt provider gains DI-isolated unit tests (11 scenarios) covering install/remove/re-install-after-remove flows with the fake `CmdRunner`. Debian E2E (`e2e/debian/run-tests.sh`) gains four scenarios covering imperative install/remove end-to-end against real `apt-get`, plus a store-level config hard-fail scenario. New bash helpers `e2e/lib/yaml_assert.sh` enable structural YAML field assertions.

## Capabilities

### New Capabilities

None. All changes map to existing capabilities.

### Modified Capabilities

- `schema-design`: State file schema gains `first_install_at` (renamed from `install_at`, with immutability semantics) and `removed_at` (new). `schema_version` bumps to `2` with a forward migration. Store-level config schema adds an explicit rejection rule for `profile_tag` and `machine_id`.
- `builtin-providers`: Apt provider section is rewritten — CLI install/remove SHALL update the hamsfile after successful command execution (matching the Package Provider Common Pattern already followed by brew), stdout/stderr SHALL stream, and the `apt-get`/`dpkg` command boundary SHALL be exposed through a DI-injectable interface.
- `project-structure`: GitHub Actions CI pipeline requirement is tightened — workflow steps SHALL invoke Taskfile tasks via `go-task/setup-task@v2`; no raw build/test/lint/docker commands may appear directly in workflow YAML.

## Impact

**Code paths affected:**

- `internal/provider/builtin/apt/` — new `command.go`, `command_fake.go`, `hamsfile.go`; `apt.go` refactored; new `apt_test.go`.
- `internal/state/` — `state.go` field rename + new `RemovedAt`; new `migration.go` for v1→v2; extended `state_test.go`.
- `internal/config/` — `config.go` gains `validateStoreScope`; `merge.go` unchanged; extended `config_test.go`.
- All 15 builtin providers — any reference to the renamed `InstallAt` field must be updated (search-and-replace) even though only apt's behavior changes.
- `examples/.template/store/hams.config.yaml`, `examples/basic-debian/store/hams.config.yaml`, and any other `examples/*/store/hams.config.yaml` — scrubbed of `profile_tag` / `machine_id`.
- `e2e/fixtures/debian-store/hams.config.yaml`, `e2e/fixtures/alpine-store/hams.config.yaml`, `e2e/fixtures/openwrt-store/hams.config.yaml`, `e2e/fixtures/test-store/hams.config.yaml` — same scrub.

**CI and testing:**

- `.github/workflows/ci.yml` — every step migrated to `setup-task` + `task <name>`. Any missing `ci:*` compositions are added to `Taskfile.yml` first.
- `Taskfile.yml` — may gain `ci:unit`, `ci:build`, `ci:lint` compositions depending on current coverage.
- `e2e/debian/run-tests.sh` — appends apt imperative scenarios.
- `e2e/debian/assert-apt-imperative.sh` (new) — scenarios E1–E5.
- `e2e/lib/yaml_assert.sh` (new) — `assert_yaml_field_eq / absent / present` helpers using `yq`.
- `e2e/integration/Dockerfile` — must include `yq` if not already present.

**User-visible impact:**

- **Breaking for existing `apt.state.yaml` files** (and any other provider's state file on disk): first read migrates `install_at` → `first_install_at` and bumps `schema_version` to `2`. One-way; downgrading hams afterwards will fail to load state. Call out in CHANGELOG.
- **Breaking for existing store repos** that rely on `profile_tag` or `machine_id` at store level: users must move those to `~/.config/hams/hams.config.yaml` or `hams.config.local.yaml`. The failure message tells them where.
- No changes to existing `hams apply --from-repo` flows; apt resources continue to reconcile normally.

**Dependencies:**

- Adds `yq` to debian integration Dockerfile.
- Adds `go-task/setup-task@v2` to CI workflow.
- No new Go module dependencies.
