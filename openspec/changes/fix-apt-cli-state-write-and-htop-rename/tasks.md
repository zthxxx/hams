# Tasks

## 1. Spec deltas (drafted with this change)

- [x] 1.1 Draft `proposal.md` describing both the behavior change and the rename.
- [x] 1.2 Draft `design.md` with rationale, approach, and failure modes.
- [x] 1.3 Draft `specs/builtin-providers/spec.md` delta (apt CLI install/remove writes state; rename `bat` → `htop` in scenarios).
- [x] 1.4 Draft `specs/dev-sandbox/spec.md` delta (rename `bat` → `htop` in the "Hams apply succeeds end-to-end via sudo" scenario).
- [x] 1.5 Draft `specs/schema-design/spec.md` delta (rename `bat` → `htop` in the "Remove transitions record removed_at" scenario).

## 2. Apt provider code

- [x] 2.1 Add `statePath(flags) string` helper on `*Provider` that resolves `<storeDir>/.state/<machineID>/apt.state.yaml`.
- [x] 2.2 Add `loadOrCreateStateFile(flags) *state.File` helper following the `internal/provider/probe.go` pattern.
- [x] 2.3 Modify `handleInstall` to: load state → for each pkg call `runner.IsInstalled` to capture version → `sf.SetResource(pkg, StateOK, WithVersion(version))` → save state.
- [x] 2.4 Modify `handleRemove` to: load state → for each pkg `sf.SetResource(pkg, StateRemoved)` → save state.
- [x] 2.5 Preserve current atomicity: on `runner.Install`/`Remove` error, return early without writing state or hamsfile.

## 3. Apt provider unit tests

- [x] 3.1 Rename `bat` → `htop` in existing `apt_test.go` fixtures.
- [x] 3.2 Add `TestHandleCommand_InstallWritesState` — install creates state row with `state=ok`, `first_install_at`, `updated_at`, `version` populated.
- [x] 3.3 Add `TestHandleCommand_ReinstallPreservesFirstInstallAt` — second install bumps `updated_at`, leaves `first_install_at` immutable.
- [x] 3.4 Add `TestHandleCommand_RemoveWritesState` — remove transitions to `state=removed`, sets `removed_at`, preserves `first_install_at`.
- [x] 3.5 Add `TestHandleCommand_ReinstallAfterRemoveClearsRemovedAt` — install after remove clears `removed_at`, preserves `first_install_at`.
- [x] 3.6 Add `TestHandleCommand_InstallFailureLeavesStateUntouched` — failed install does not touch state file.
- [x] 3.7 Add `TestHandleCommand_DryRunDoesNotTouchState` — dry-run does not load or write state.

## 4. E2E test additions

- [x] 4.1 Rename `bat` → `htop` in `e2e/debian/assert-apt-imperative.sh` (E1–E3 + E5 fixture).
- [x] 4.2 Drop the intermediate `hams apply --only=apt` calls from E1–E3 — install / remove alone now reconcile state.
- [x] 4.3 Add a new section `assert_apt_cli_only_flow` that mirrors the user's manual sandbox flow (install jq → updated_at bumped; install btop → state row created; remove btop → state=removed) without any apply between steps.
- [x] 4.4 Wire the new section into `e2e/debian/run-tests.sh`.

## 5. Examples + docs + README rename

- [x] 5.1 `examples/basic-debian/store/dev/apt.hams.yaml` — replace `bat` package entry with `htop` (de-dup against existing `cli` `htop`).
- [x] 5.2 `examples/basic-debian/state/sandbox/apt.state.yaml` — replace `bat` resource with `htop`.
- [x] 5.3 `examples/basic-debian/README.md` — replace narrative reference.
- [x] 5.4 `README.md` + `README.zh-CN.md` — replace `hams brew install bat` install example with `hams brew install htop`.
- [x] 5.5 `docs/content/en/docs/quickstart.mdx` + `docs/content/zh-CN/docs/quickstart.mdx` — replace `bat` install example.
- [x] 5.6 `docs/content/en/docs/why-hams.mdx` + `docs/content/zh-CN/docs/why-hams.mdx` — replace `bat` in the "you read a blog post" example.
- [x] 5.7 `docs/content/en/docs/cli/global-flags.mdx` + `docs/content/zh-CN/docs/cli/global-flags.mdx` — replace `bat` in the disambiguation example.
- [x] 5.8 `docs/content/en/docs/cli/provider-commands.mdx` + `docs/content/zh-CN/docs/cli/provider-commands.mdx` — replace `bat` in install/enrich examples.
- [x] 5.9 `docs/content/en/docs/cli/store.mdx` + `docs/content/zh-CN/docs/cli/store.mdx` — replace `bat` in commit message example.
- [x] 5.10 `docs/content/en/docs/cli/list.mdx` + `docs/content/zh-CN/docs/cli/list.mdx` — replace `bat` in sample list output.
- [x] 5.11 `docs/content/en/docs/cli/index.mdx` + `docs/content/zh-CN/docs/cli/index.mdx` — replace `bat` in install/enrich examples.
- [x] 5.12 `docs/content/en/docs/providers/homebrew.mdx` + `docs/content/zh-CN/docs/providers/homebrew.mdx` — replace `bat` in enrich example + sample hamsfile entry.
- [x] 5.13 Skip `docs/content/{en,zh-CN}/docs/providers/cargo.mdx` — `bat` is genuinely a Rust tool installable via cargo.
- [x] 5.14 `.agents/rules/development-process.md` — replace `brew: bat` safe local test package with `brew: htop`.

## 6. Process / rules

- [x] 6.1 Add a "Test dimensions clarified" subsection under `.claude/rules/development-process.md` "Verification" bullet, distinguishing unit / integration / E2E by scope, definition, and DI requirement.
- [x] 6.2 Add a "Standardized provider integration test process" rule (or pointer file) capturing the manual sandbox flow as a generalizable verification pattern.

## 7. Verification

- [x] 7.1 `task fmt` clean.
- [x] 7.2 `task lint:go` clean.
- [x] 7.3 `task test:unit` (incl. new apt unit tests) green with `-race`.
- [x] 7.4 `task ci:e2e:run TARGET=debian` (or `task test:e2e:one TARGET=debian`) green — exercises the new install-only / remove-only flow against a real Debian container.
- [x] 7.5 `grep -RIn '\bbat\b' --exclude-dir=openspec/changes/archive --exclude-dir=node_modules .` reports only the intentional cargo references + the spec/state machinery generic test fixtures.

## 8. Apply/refresh scope gate (two-stage filter)

- [ ] 8.1 Add `internal/provider/filter.go::HasArtifacts(p, profileDir, stateDir) bool` — returns true if `<profile>/<FilePrefix>.hams.yaml`, its `.local.yaml` sibling, or `.state/<machine>/<FilePrefix>.state.yaml` exists.
- [ ] 8.2 Modify `internal/cli/commands.go::runRefresh` to filter registry by `HasArtifacts` BEFORE applying `--only`/`--except`. Debug-log skipped providers.
- [ ] 8.3 Modify `internal/cli/apply.go` (or its equivalent) to apply the same two-stage filter before `ProbeAll`, `Plan`, and `Execute`.
- [ ] 8.4 Unit tests in `internal/provider/filter_test.go` — empty profile + empty state → skipped; hamsfile only → included; state only → included; both → included; `--only` narrows within the stage-1 result; `--only` outside stage-1 result → empty-result no-op.
- [ ] 8.5 Unit tests in `internal/cli/*_test.go` — `runApply` / `runRefresh` with mixed providers: only providers with artifacts are probed.

## 9. Per-provider integration test scaffolding (new test infra)

- [ ] 9.1 Create `e2e/base/Dockerfile` — `FROM debian:bookworm-slim` + `ca-certs`, `curl`, `bash`, `git`, `sudo`, `yq` (from GitHub release, pinned version). No language runtimes.
- [ ] 9.2 Relocate `e2e/lib/*.sh` → `e2e/base/lib/*.sh`. Update callers in `e2e/debian/`, `e2e/alpine/`, `e2e/openwrt/`, `e2e/sudo/`, `e2e/integration/`.
- [ ] 9.3 Add `e2e/base/lib/provider_flow.sh` — exports `standard_cli_flow <provider> <install_verb> <existing_pkg> <new_pkg>` implementing the canonical 5-step flow (seed-install existing → sleep → re-install existing → install new → refresh → remove new) with state-file assertions after each step.
- [ ] 9.4 Add Taskfile entries `ci:itest:base` (build base image if missing), `ci:itest:run PROVIDER=<name>` (build provider image + run integration.sh), `ci:itest` (loop all in-scope providers), `test:itest:one PROVIDER=<name>` (same via `act`).
- [ ] 9.5 Add `.github/workflows/ci.yml` matrix job `itest` with one entry per in-scope provider, each invoking `task ci:itest:run PROVIDER=<name>`.

## 10. Migrate apt to new per-provider integration layout

- [ ] 10.1 Create `internal/provider/builtin/apt/integration/Dockerfile` — `FROM hams-itest-base:<base-hash>` with no delta (apt-get already in base).
- [ ] 10.2 Create `internal/provider/builtin/apt/integration/integration.sh` — sources shared helpers, runs `apt-get update`, then calls `standard_cli_flow apt install jq btop` + apt-specific extras (E1–E3 install/remove cycle, schema v1→v2 migration E5 moved from `e2e/debian/assert-apt-imperative.sh`).
- [ ] 10.3 Prune `e2e/debian/assert-apt-imperative.sh` so it keeps ONLY the store-level profile_tag rejection E4 (cross-provider config concern), or merge E4 into a new `e2e/debian/assert-config-scope.sh`. Update `e2e/debian/run-tests.sh` to drop the `run_apt_imperative_tests` + `run_apt_cli_only_flow` calls — apt integration now lives in its own container.
- [ ] 10.4 Update `.claude/rules/development-process.md` — the "Standardized provider integration test process" section now points at `internal/provider/builtin/<provider>/integration/integration.sh` with `standard_cli_flow` as the reference helper; the apt path is the canonical example.

## 11. Remaining provider integration tests

All 10 remaining in-scope providers (beyond apt) shipped in this change.
Each provides its own `integration/{Dockerfile, integration.sh}`. Unit
tests (Go) pass under `task test:unit`; Docker-based integration tests
are validated by the `itest` matrix in `.github/workflows/ci.yml`.
Where docker was unreachable during development, CI remains the
authoritative validator.

- [x] 11.1 ansible — Dockerfile installs ansible + python3 via apt; integration.sh exercises one-shot `hams ansible <playbook>` + declarative `hams apply --only=ansible` with a localhost `ansible.builtin.file` task.
- [x] 11.2 bash — empty Dockerfile (bash in base); integration.sh declares a bash resource with `run`/`check`/`remove`, verifies `hams apply` / drift detection via `refresh` / recovery / declarative remove.
- [x] 11.3 cargo — Dockerfile installs rustup + stable toolchain + gcc; integration.sh runs `standard_cli_flow cargo install xsv xcp`.
- [x] 11.4 git-config — shares the `git` integration container with git-clone; integration.sh declares two `integration.hams.*` keys, asserts `git config --global --get` reads them back, verifies `refresh` bump + single-entry removal.
- [x] 11.5 git-clone — shares the `git` integration container with git-config; integration.sh seeds a bare repo fixture, declares a clone entry, verifies clone presence + `refresh` bump + declarative remove via hamsfile delete.
- [x] 11.6 goinstall — Dockerfile downloads Go 1.24 tarball to `/usr/local/go`; integration.sh runs `standard_cli_flow goinstall install github.com/rakyll/hey@latest github.com/mgechev/revive@latest`.
- [x] 11.7 homebrew — Dockerfile creates a non-root `brew` user (linuxbrew refuses root) + runs the official install script as that user + NOPASSWD sudo back to root for test harness access; integration.sh runs the canonical lifecycle manually via `sudo -u brew` so the state-file assertions stay in the driving container.
- [x] 11.8 npm — Dockerfile installs Node.js 20 via NodeSource repo; integration.sh runs `standard_cli_flow npm install serve sort-package-json`.
- [x] 11.9 pnpm — Dockerfile installs Node.js 20 + activates pnpm via `corepack`; integration.sh runs `standard_cli_flow pnpm add serve sort-package-json`.
- [x] 11.10 uv — Dockerfile installs uv via the official install script; integration.sh runs `standard_cli_flow uv install ruff tomli-lint`.
- [x] 11.11 vscodeext (code-ext) — Dockerfile installs the VS Code CLI standalone binary; integration.sh defines a `POST_INSTALL_CHECK` hook using `code --list-extensions` and runs `standard_cli_flow code-ext install vscode-icons-team.vscode-icons bungcip.better-toml`.

Architectural note: each provider's Dockerfile is minimal delta on top of
`hams-itest-base`. Docker cache hashing keys on the SHA of the provider
Dockerfile — so changing one provider's runtime install does not
invalidate any other provider's cached image. Homebrew is the only
provider that departs from "everything runs as root inside the
container" (linuxbrew's own restriction); the workaround is scoped
entirely to its Dockerfile and wrapper function.

## 12. Archive

- [ ] 12.1 `/opsx:verify` — confirm spec deltas vs implementation align.
- [ ] 12.2 `/opsx:archive fix-apt-cli-state-write-and-htop-rename` — auto-sync deltas into main specs.
