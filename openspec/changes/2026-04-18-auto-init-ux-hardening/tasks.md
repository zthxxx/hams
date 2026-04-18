# Tasks: auto-init UX hardening

- [x] **internal/storeinit — context timeout on `git init`**
  - [x] In `initGitRepo(ctx, dir)`, wrap the `exec.CommandContext(ctx, gitBin, "init", …)` branch with `ctxTimeout, cancel := context.WithTimeout(ctx, GitInitTimeout); defer cancel()`.
  - [x] Pass `ctxTimeout` (not the caller's `ctx`) to `exec.CommandContext`.
  - [x] Do NOT wrap the go-git `PlainInit` branch in a timeout (in-process, cannot wedge on a global hook).
  - [x] Expose `LookPathGit` + `ExecCommandContext` package-level vars so the unit test can inject a fake git binary path that sleeps longer than the timeout, plus `GitInitTimeout` as a tunable `var` so the test can shrink it to a few hundred milliseconds.

- [x] **internal/cli/autoinit — signature + dry-run short-circuit**
  - [x] Extend `EnsureGlobalConfig(paths)` → `EnsureGlobalConfig(paths, flags)`.
  - [x] Extend `EnsureStoreReady(paths, cfg, cliOverride)` → `EnsureStoreReady(paths, cfg, cliOverride, flags)`.
  - [x] When `flags != nil && flags.DryRun`:
    - [x] `EnsureGlobalConfig` — print `[dry-run] Would auto-create hams global config at <path>` to `flags.Stderr()`, skip the file write, return `nil`.
    - [x] `EnsureStoreReady` — print `[dry-run] Would auto-init hams store at <path>` to `flags.Stderr()`, return `(resolvedPath, false, nil)` without calling `storeinit.Bootstrap` or `config.WriteConfigKey`.

- [x] **internal/cli/autoinit — seed identity helper**
  - [x] Add `seedIdentityIfMissing(paths config.Paths)` that inspects the on-disk config via `config.Load(paths, "", "")`, then for each of `profile_tag` / `machine_id`:
    - [x] If the key is empty in the loaded config, call `config.WriteConfigKey(paths, "", key, value)` where `value` is `config.DefaultProfileTag` / `config.DeriveMachineID()`.
    - [x] Failures are logged (slog.Warn) but not propagated (best-effort).
  - [x] Call `seedIdentityIfMissing` from `EnsureStoreReady` only after a successful non-dry-run `Bootstrap`.

- [x] **call-site updates**
  - [x] `internal/cli/apply.go` — the `storePath == ""` branch passes `flags` to both `EnsureGlobalConfig` and `EnsureStoreReady`.
  - [x] `internal/cli/commands.go` — the refresh-side auto-init branch passes `flags` to both helpers.
  - [x] `internal/cli/provider_cmd.go::autoInitForProvider` — passes `flags` to both helpers.

- [x] **i18n**
  - [x] Add `AutoInitDryRunGlobalConfig` + `AutoInitDryRunStore` typed keys to `internal/i18n/keys.go`.
  - [x] Add translations in `internal/i18n/locales/en.yaml` + `internal/i18n/locales/zh-CN.yaml`.
  - [x] Add the two new keys to the `TestCatalogCoherence_EveryTypedKeyResolves` list in `internal/i18n/i18n_test.go`.

- [x] **unit tests**
  - [x] `internal/storeinit/storeinit_test.go::TestBootstrap_ContextTimeoutStopsHungGitHook` — injects a fake `exec` seam pointing at `sleep 5`, shrinks `GitInitTimeout` to 150 ms, asserts `BootstrapContext` returns within ~3 s with a deadline-related error.
  - [x] `internal/cli/autoinit_test.go::TestEnsureStoreReady_DryRunHasNoSideEffects` — calls with `flags.DryRun = true`, asserts the target directory was NOT created and `flags.Stderr()` captured the preview line.
  - [x] `internal/cli/autoinit_test.go::TestEnsureStoreReady_SeedsIdentity` — calls on a fresh config, asserts `profile_tag == config.DefaultProfileTag` and `machine_id` matches the seam-injected hostname.
  - [x] `internal/cli/autoinit_test.go::TestEnsureStoreReady_RespectsPreSetIdentity` — pre-writes `profile_tag: macOS` + `machine_id: laptop-m5x`, asserts the scaffolder does NOT overwrite them.
  - [x] `internal/cli/autoinit_test.go::TestEnsureGlobalConfig_DryRunSkipsWrite` — mirror assertion for the global config helper.

- [x] **verification**
  - [x] `go build ./...` — 0 errors.
  - [x] `task fmt && task lint` — 0 issues.
  - [x] `task test:unit` — relevant packages PASS.
