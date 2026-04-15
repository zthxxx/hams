# Tasks: fix-apt-provider-and-store-config-scope

Atomic implementation checklist. Each group maps to an independent commit per `.claude/rules/development-process.md`. Complete a group fully (including its tests) before moving to the next.

## 1. State schema: rename + RemovedAt + v1→v2 migration

Delivers: state/schema-design requirements for `first_install_at`, `removed_at`, auto-migration.

- [x] 1.1 Bump `SchemaVersion` constant in `internal/state/state.go` from `1` to `2`.
- [x] 1.2 Rename `Resource.InstallAt` → `Resource.FirstInstallAt`; YAML tag `install_at` → `first_install_at`. Keep `omitempty`.
- [x] 1.3 Add `Resource.RemovedAt string` with tag `removed_at,omitempty`.
- [x] 1.4 Keep a transient legacy `legacyInstallAt string \`yaml:"install_at,omitempty"\`` field (unexported) on a loader-side intermediary struct for reading v1 files; move its value into `FirstInstallAt` during load, then discard.
- [x] 1.5 Rewrite `SetResource` per the semantics table in §2.2 of design.md:
  - [x] 1.5.1 `StateOK`: set `FirstInstallAt = now` only if empty; clear `RemovedAt`; bump `UpdatedAt`; clear `LastError`.
  - [x] 1.5.2 `StateRemoved`: set `RemovedAt = now`; bump `UpdatedAt`; leave `FirstInstallAt` untouched.
  - [x] 1.5.3 `StateFailed`/`StateHookFailed`: bump `UpdatedAt`; leave `FirstInstallAt` and `RemovedAt` untouched.
  - [x] 1.5.4 `StatePending`: no timestamp changes.
- [x] 1.6 Create `internal/state/migration.go` with `migrate(f *File) error`. Handles `schema_version: 0|1` → `2`. Sets `f.SchemaVersion = 2` after migration.
- [x] 1.7 Call `migrate` from inside `state.Load` after `yaml.Unmarshal` but before returning. Reject `schema_version > 2` with the error message defined in the spec.
- [x] 1.8 Update all producers of state that reference `InstallAt` in the codebase — search `rg -w 'InstallAt' -g '!openspec/**'` and update each hit to `FirstInstallAt`.
- [x] 1.9 Unit tests in `internal/state/state_test.go` covering S1–S6:
  - [x] 1.9.1 S1: new resource → `SetResource(id, StateOK)` sets `FirstInstallAt`, `UpdatedAt`, no `RemovedAt`.
  - [x] 1.9.2 S2: second `SetResource(id, StateOK)` keeps `FirstInstallAt`, bumps `UpdatedAt`.
  - [x] 1.9.3 S3: `SetResource(id, StateRemoved)` sets `RemovedAt`, bumps `UpdatedAt`, `FirstInstallAt` unchanged.
  - [x] 1.9.4 S4: `SetResource(id, StateOK)` after removed clears `RemovedAt`, keeps `FirstInstallAt`, bumps `UpdatedAt`.
  - [x] 1.9.5 S5: write a v1 state file with `install_at: "20260410T091500"`; `Load` produces in-memory `schema_version: 2` + `FirstInstallAt: "20260410T091500"`; `Save` writes v2 YAML with `first_install_at` only (no `install_at` key).
  - [x] 1.9.6 S6: round-trip v2 → marshal → unmarshal yields identical struct.
- [x] 1.10 Verify no `install_at` string literal remains in production code (only in migration path + tests). `rg "install_at" -g '!openspec/**' -g '!*_test.go' -g '!migration.go'` returns empty.
- [x] 1.11 `go build ./...` + `go vet ./...` + `go test -race ./internal/state/...` pass.
- [x] 1.12 Commit: `refactor(state): rename InstallAt → FirstInstallAt + add RemovedAt + schema v1→v2 migration`.

## 2. Config scope validation: reject store-level profile_tag / machine_id

Delivers: schema-design Project-Level Config Schema requirement additions.

- [x] 2.1 Add `validateStoreScope(cfg *Config, path string) error` in `internal/config/config.go`. Error message template from design D6.
- [x] 2.2 Wire `validateStoreScope` call into the loader at the point where `<store>/hams.config.yaml` and `<store>/hams.config.local.yaml` are parsed (before merge). Exact insertion point: `internal/config/config.go:Load` (or the equivalent loader entry), immediately after `yaml.Unmarshal` of each store-level file.
- [x] 2.3 Ensure the error is returned (not panicked, not warned) so callers propagate to exit-non-zero.
- [x] 2.4 Unit tests in `internal/config/config_test.go` covering C1–C5:
  - [x] 2.4.1 C1: store `hams.config.yaml` with `profile_tag: dev` → `Load` returns error containing file path + `profile_tag` + global path pointer.
  - [x] 2.4.2 C2: store `hams.config.yaml` with `machine_id: x` → same.
  - [x] 2.4.3 C3: store `hams.config.local.yaml` with `profile_tag: dev` → error (symmetric strictness).
  - [x] 2.4.4 C4: global `hams.config.yaml` with `profile_tag: dev` → load succeeds.
  - [x] 2.4.5 C5: store config without machine-scope fields → load succeeds, merge works normally.
- [x] 2.5 Scrub `profile_tag` and `machine_id` from:
  - [x] 2.5.1 `examples/.template/store/hams.config.yaml`
  - [x] 2.5.2 `examples/basic-debian/store/hams.config.yaml`
  - [x] 2.5.3 Any additional `examples/*/store/hams.config.yaml` found via `rg -l '^(profile_tag|machine_id):' examples/`.
  - [x] 2.5.4 `e2e/fixtures/debian-store/hams.config.yaml`, `e2e/fixtures/alpine-store/hams.config.yaml`, `e2e/fixtures/openwrt-store/hams.config.yaml`, `e2e/fixtures/test-store/hams.config.yaml`.
- [x] 2.6 Add or update `examples/.template/hams.config.yaml.example` (global-level example) to show where `profile_tag` and `machine_id` belong, with inline comments.
- [x] 2.7 `go build ./...` + `go vet ./...` + `go test -race ./internal/config/...` pass.
- [x] 2.8 Commit: `feat(config): hard-fail when store-level config sets profile_tag or machine_id`.

## 3. Apt CmdRunner DI seam (no behavior change yet)

Delivers: builtin-providers apt Provider "command boundary" requirement.

- [ ] 3.1 Create `internal/provider/builtin/apt/command.go` with `CmdRunner` interface (3 methods per design D2) and a real implementation `type realCmdRunner struct { sudo sudo.CmdBuilder }`.
- [ ] 3.2 Real `Install(ctx, pkg)` runs `sudo apt-get install -y <pkg>`, sets `cmd.Stdout = os.Stdout`, `cmd.Stderr = os.Stderr`.
- [ ] 3.3 Real `Remove(ctx, pkg)` runs `sudo apt-get remove -y <pkg>`, streams stdout/stderr.
- [ ] 3.4 Real `IsInstalled(ctx, pkg)` runs `dpkg -s <pkg>`, returns `(true, version, nil)` when exit 0 and `Status: install ok installed` is present; `(false, "", nil)` when exit non-zero; `(false, "", err)` for other errors.
- [ ] 3.5 Create `internal/provider/builtin/apt/command_fake.go` (no build tag — test helper exported for tests) with `FakeCmdRunner` maintaining `installed map[string]string` (pkg → version) + `calls []FakeCall`.
- [ ] 3.6 `FakeCmdRunner.Install(ctx, pkg)` records call + marks installed. `.Remove` records + marks uninstalled. `.IsInstalled` returns from the map. Add `FakeCmdRunner.WithInstallError(pkg, err)` / `.WithRemoveError` for failure simulations.
- [ ] 3.7 Update `apt.Provider` to accept a `CmdRunner` in its constructor: `New(sb sudo.CmdBuilder, cfg *config.Config, runner CmdRunner) *Provider`.
- [ ] 3.8 Update Fx wiring (whichever file provides the apt provider constructor) to build `realCmdRunner{sudo: sb}` and pass it in.
- [ ] 3.9 Refactor `Apply` to call `p.runner.Install(ctx, action.ID)` instead of shelling out directly.
- [ ] 3.10 Refactor `Remove` to call `p.runner.Remove(ctx, resourceID)`.
- [ ] 3.11 Refactor `Probe` to call `p.runner.IsInstalled(ctx, id)` instead of `exec.CommandContext(ctx, "dpkg", ...)`.
- [ ] 3.12 `go build ./...` + `go vet ./...` + existing `go test -race ./internal/provider/builtin/apt/...` pass (no new behavior yet).
- [ ] 3.13 Commit: `refactor(apt): introduce CmdRunner DI seam + fake for tests`.

## 4. Apt CLI install/remove updates hamsfile (mirrors brew)

Delivers: builtin-providers apt Provider CLI wrapping requirements.

- [ ] 4.1 Create `internal/provider/builtin/apt/hamsfile.go` with two functions modeled on `brew/hamsfile.go`:
  - [ ] 4.1.1 `addApp(cfg *config.Config, pkg string) error` — load effective `apt.hams.yaml` + `.local.yaml`, add `{app: pkg}` to default group if not present, save atomically via hamsfile SDK.
  - [ ] 4.1.2 `removeApp(cfg *config.Config, pkg string) error` — load, remove `{app: pkg}` if present, save. No-op if absent.
- [ ] 4.2 Refactor `HandleCommand` in `apt.go`:
  - [ ] 4.2.1 `handleInstall(ctx, pkgs, flags)` — for each pkg: call `runner.Install`, on success call `addApp`. Dry-run path prints and returns.
  - [ ] 4.2.2 `handleRemove(ctx, pkgs, flags)` — for each pkg: call `runner.Remove`, on success call `removeApp`. Dry-run path prints and returns.
  - [ ] 4.2.3 Default case keeps existing `WrapExecPassthrough` behavior.
- [ ] 4.3 Remove the `cmd.Stdout = nil; cmd.Stderr = nil` lines in `Apply` (moved to real CmdRunner in Group 3).
- [ ] 4.4 Unit tests in `internal/provider/builtin/apt/apt_test.go` covering U1–U11:
  - [ ] 4.4.1 Test harness: helper that creates tempdir config + hamsfile, wires `FakeCmdRunner`, returns `*Provider`.
  - [ ] 4.4.2 U1: first install adds app to hamsfile; runner called with correct pkg.
  - [ ] 4.4.3 U2: re-install is idempotent on hamsfile (still one entry).
  - [ ] 4.4.4 U3: install failure leaves hamsfile untouched.
  - [ ] 4.4.5 U4: remove deletes app from hamsfile.
  - [ ] 4.4.6 U5: remove failure leaves hamsfile untouched.
  - [ ] 4.4.7 U6: remove of absent app is no-op on hamsfile, no error.
  - [ ] 4.4.8 U7: dry-run prints expected command, does NOT call runner, does NOT mutate hamsfile.
  - [ ] 4.4.9 U8: apply of new resource sets `FirstInstallAt`, `UpdatedAt`, no `RemovedAt`.
  - [ ] 4.4.10 U9: apply of existing resource preserves `FirstInstallAt`, bumps `UpdatedAt`.
  - [ ] 4.4.11 U10: remove transition sets `RemovedAt`, bumps `UpdatedAt`, preserves `FirstInstallAt`.
  - [ ] 4.4.12 U11: re-install after remove clears `RemovedAt`, preserves `FirstInstallAt`, bumps `UpdatedAt`.
- [ ] 4.5 Verify stdout/stderr streaming: unit test asserting `realCmdRunner.Install` sets `cmd.Stdout == os.Stdout` (or equivalent behavior via interface-level assertion).
- [ ] 4.6 `go build ./...` + `go vet ./...` + `go test -race ./internal/provider/builtin/apt/...` pass.
- [ ] 4.7 Commit: `feat(apt): CLI install/remove updates hamsfile (mirrors brew)`.

## 5. E2E integration scenarios + yaml_assert helper

Delivers: debian E2E integration covering real apt-get end-to-end + store-level config rejection.

- [ ] 5.1 Create `e2e/lib/yaml_assert.sh` with functions `assert_yaml_field_eq <file> <yq-path> <expected>`, `assert_yaml_field_absent <file> <yq-path>`, `assert_yaml_field_present <file> <yq-path>`. Uses `yq` (Mike Farah's Go implementation).
- [ ] 5.2 Verify `yq` is available in `e2e/integration/Dockerfile`; add `apt-get install -y yq` if missing.
- [ ] 5.3 Create `e2e/debian/assert-apt-imperative.sh` — sourced bash file exporting `run_apt_imperative_tests()`.
- [ ] 5.4 Scenario E1: `hams apt install bat` + `hams apply` → assert:
  - [ ] 5.4.1 `command -v bat` succeeds.
  - [ ] 5.4.2 `apt.hams.yaml` contains `{app: bat}` (via yq).
  - [ ] 5.4.3 `apt.state.yaml` has `resources.bat.state == ok`.
  - [ ] 5.4.4 `resources.bat.first_install_at` is a non-empty timestamp.
  - [ ] 5.4.5 `resources.bat.removed_at` is absent.
- [ ] 5.5 Scenario E2: record `first_install_at` as `$T_FI`; `hams apt remove bat` + `hams apply` → assert:
  - [ ] 5.5.1 `command -v bat` fails (non-zero exit).
  - [ ] 5.5.2 `apt.hams.yaml` no longer has `{app: bat}`.
  - [ ] 5.5.3 `apt.state.yaml` has `resources.bat.state == removed`.
  - [ ] 5.5.4 `resources.bat.first_install_at == $T_FI`.
  - [ ] 5.5.5 `resources.bat.removed_at` is present + non-empty.
  - [ ] 5.5.6 `resources.bat.updated_at > first_install_at` (lexicographic compare, since format is sortable).
- [ ] 5.6 Scenario E3: `hams apt install bat` again + `hams apply` → assert:
  - [ ] 5.6.1 `command -v bat` succeeds.
  - [ ] 5.6.2 `resources.bat.state == ok`.
  - [ ] 5.6.3 `resources.bat.first_install_at == $T_FI` (immutable).
  - [ ] 5.6.4 `resources.bat.removed_at` is absent.
- [ ] 5.7 Scenario E4: write `profile_tag: dev` into `<store>/hams.config.yaml`; run any hams command → assert:
  - [ ] 5.7.1 Exit code non-zero.
  - [ ] 5.7.2 Stderr contains `profile_tag`, the full file path, and `hams.config.yaml` (pointing to global location).
  - [ ] 5.7.3 Cleanup: restore the fixture file.
- [ ] 5.8 Scenario E5: pre-create synthetic v1 state file, run `hams apply` → assert file is rewritten with `schema_version: 2` and `first_install_at`.
- [ ] 5.9 Hook `run_apt_imperative_tests` into `e2e/debian/run-tests.sh` (call after existing apply-based tests).
- [ ] 5.10 `task ci:integration` passes locally via `act`.
- [ ] 5.11 Commit: `test(e2e): add debian apt imperative scenarios E1–E5 + yaml_assert helpers`.

## 6. CI workflow refactor: setup-task + Taskfile tasks only

Delivers: project-structure GitHub Actions CI Pipeline requirement additions.

- [ ] 6.1 Audit current `.github/workflows/ci.yml`. Record which steps inline raw commands.
- [ ] 6.2 For each inlined raw command, ensure a Taskfile task exists. Add missing ones under `ci:*` namespace (e.g., `ci:lint`, `ci:lint:md`, `ci:lint:spell`, `ci:unit`, `ci:build`).
- [ ] 6.3 Add `go-task/setup-task@v1` step to every job that needs to invoke `task`. Pin to `@v1`.
- [ ] 6.4 Replace each inlined command with `run: task <name>`.
- [ ] 6.5 Retain `actions/checkout@v4`, `actions/setup-go@v5`, `actions/upload-artifact@v4`, and other setup/upload actions (per the carve-out in the spec).
- [ ] 6.6 Verify workflow runs locally via `act pull_request -j lint` (dry-run compile + one job) — smoke test.
- [ ] 6.7 `task lint` and `task test` still pass after any Taskfile additions.
- [ ] 6.8 Commit: `ci: route all workflow steps through Taskfile tasks via setup-task`.

## 7. Documentation sync

Delivers: i18n-consistent docs updates.

- [ ] 7.1 Search docs for `install_at` string → rename to `first_install_at`. `rg 'install_at' docs/ README.md` and fix each hit.
- [ ] 7.2 Search docs for store-level examples showing `profile_tag` / `machine_id` → correct them.
- [ ] 7.3 Add a "Breaking changes" section to `CHANGELOG.md` (create if absent) covering state schema v1→v2 auto-migration + store-level config rejection.
- [ ] 7.4 Sync the same changes to `*.zh-CN.*` variants (e.g., `README.zh-CN.md`, any `docs/**/*.zh-CN.*`).
- [ ] 7.5 Run `task lint` to catch any markdown or spelling regressions.
- [ ] 7.6 If docs site has live dev server, run the `docs-verification.md` process (sections 1–3) to confirm pages render.
- [ ] 7.7 Commit: `docs: sync install_at rename + store-level config guidance (en + zh-CN)`.

## 8. Full-suite verification gate

Final pre-archive validation. ALL must pass before archiving.

- [ ] 8.1 `go build ./...` — zero errors.
- [ ] 8.2 `go vet ./...` — zero warnings.
- [ ] 8.3 `go test -race ./...` — all packages pass.
- [ ] 8.4 `task lint` — golangci-lint v2 + markdownlint + cspell all pass.
- [ ] 8.5 `task ci:integration` — via `act`, full integration suite green.
- [ ] 8.6 `task ci:e2e` — all E2E targets (debian + alpine + openwrt where applicable) green.
- [ ] 8.7 `rg -w 'InstallAt' -g '!openspec/**' -g '!*.md'` — zero production-code hits (all renamed).
- [ ] 8.8 `rg '^profile_tag|^machine_id' examples/ e2e/fixtures/` — zero hits at store level (all scrubbed).
- [ ] 8.9 `openspec validate fix-apt-provider-and-store-config-scope --strict` — passes.
- [ ] 8.10 Manual smoke test: build `bin/hams`, launch dev sandbox (`task dev EXAMPLE=basic-debian`), reproduce the original bug scenario from the user's message (`hams apt install bat` → `command -v bat` succeeds), verify state file fields.

## 9. OpenSpec archive

- [ ] 9.1 Run `/opsx:verify` — resolve any findings (findings become new task items in the relevant task.md file).
- [ ] 9.2 Run `/codex:review --wait --base <base-sha>` — record findings as tasks, fix via `/codex:rescue`.
- [ ] 9.3 Run `/simplify` — address any review issues.
- [ ] 9.4 Run `/opsx:archive` — move change to `openspec/archive/` and merge deltas into `openspec/specs/`.
- [ ] 9.5 Commit: `chore: archive fix-apt-provider-and-store-config-scope`.
