# Tasks — shared-abstraction-fanout

## 1. baseprovider package

- [x] 1.1 Create `internal/provider/baseprovider/baseprovider.go` with
  `EffectiveConfig(cfg *config.Config, flags *GlobalFlags) *config.Config`,
  `HamsfilePath(cfg, filePrefix, hamsFlags, flags) string`,
  `LoadOrCreateHamsfile(cfg, filePrefix, hamsFlags, flags) (*hamsfile.File, error)`.
- [x] 1.2 Add `internal/provider/baseprovider/baseprovider_test.go` with
  table-driven tests for each function (≥6 cases covering profile/tag
  override, local override, missing file → create path).
- [x] 1.3 Migrate inlined call sites:
  homebrew, cargo, apt, npm, pnpm, uv, mas, vscodeext, goinstall,
  ansible, defaults, duti, bash, git/clone — replace local helpers
  with `baseprovider.*` calls.
- [x] 1.4 Verify `go vet ./...` + `golangci-lint run` clean.
- [x] 1.5 Verify `task test:unit` passes.
- [x] 1.6 Atomic commit: `feat(provider): add baseprovider package with
  EffectiveConfig/HamsfilePath/LoadOrCreateHamsfile`.

## 2. Passthrough primitive

- [x] 2.1 Create `internal/provider/passthrough.go`:
  `var PassthroughExec = func(ctx, tool, args) error { exec.CommandContext(...).Run() }`
  + `Passthrough(ctx, tool, args, flags)` function honouring
  `flags.DryRun` (prints i18n preview, no exec).
- [x] 2.2 Create `internal/provider/passthrough_test.go` swapping
  `PassthroughExec` to record invocations; covers DryRun-no-exec
  case + normal-pass-through case + nil-flags safety.
- [x] 2.3 Migrate every `WrapExecPassthrough` call site to
  `Passthrough`: homebrew (default branch), cargo (default), apt
  (default), git/unified.
- [x] 2.4 Delete `internal/provider/wrap.go`'s `WrapExecPassthrough`
  function (and its test if any). Project rule: no compat shim.
- [x] 2.5 `task test:unit` passes; integration tests untouched (they
  exercise the real exec path).
- [x] 2.6 Atomic commit: `feat(provider): add Passthrough primitive
  with DryRun support and DI seam`.

## 3. storeinit fine-grained DI seams

- [x] 3.1 Replace `var ExecGitInit = defaultExecGitInit` and
  `var GoGitInit = defaultGoGitInit` + `const gitInitTimeout` in
  `internal/storeinit/storeinit.go` with:
  `var LookPathGit = func() (string, error) { return exec.LookPath("git") }`,
  `var ExecCommandContext = exec.CommandContext`,
  `var GitInitTimeout = 30 * time.Second`.
- [x] 3.2 Refactor `defaultExecGitInit` body to use the new seams.
  Keep `defaultGoGitInit` as the go-git fallback (uses
  `go-git/v5`'s `git.PlainInit`).
- [x] 3.3 Update `internal/storeinit/storeinit_test.go` to swap the
  new seams (LookPathGit returns ErrNotFound to force fallback;
  ExecCommandContext returns a stubbed cmd to assert behaviour;
  GitInitTimeout shortened in tests).
- [x] 3.4 `task test:unit` passes; **apt E0 integration test runs
  unchanged** and still asserts the "bundled go-git" stderr line.
- [x] 3.5 Atomic commit: `refactor(storeinit): replace coarse function
  seams with fine-grained LookPathGit/ExecCommandContext/GitInitTimeout`.

## 4. Spec updates and verification

- [x] 4.1 Wrote `specs/provider-system/spec.md` delta with SHALLs:
  "Provider passthrough flow MUST honour `--dry-run`",
  "Package-class providers MUST use `baseprovider.*` helpers
  for hamsfile/config resolution".
- [x] 4.2 Wrote `specs/cli-architecture/spec.md` delta with SHALL:
  "`internal/storeinit` MUST expose `LookPathGit`,
  `ExecCommandContext`, `GitInitTimeout` as package-level
  variables for unit-test substitution".
- [ ] 4.3 `task check` passes end-to-end (fmt + lint + unit +
  integration + e2e).
- [ ] 4.4 Archive 2026-04-19-shared-abstraction-fanout after
  task check is green.
