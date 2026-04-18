# Tasks: `storeinit` Package with `go-git` Fallback

## 1. Create `internal/storeinit/` package

- [x] 1.1 Add `internal/storeinit/doc.go` with the package-level doc comment.
- [x] 1.2 Move embedded templates from `internal/cli/template/store/` to `internal/storeinit/template/`.
- [x] 1.3 Add `internal/storeinit/storeinit.go` exposing `Bootstrap`, `Bootstrapped`, `DefaultLocation`, plus the `ExecGitInit` / `GoGitInit` DI seams.
- [x] 1.4 Port the existing `scaffold.go` behaviour byte-for-byte: dry-run short-circuit, idempotent template writes, `WriteConfigKey(store_path)` persistence, `seedIfMissing(profile_tag)` + `seedIfMissing(machine_id)` seeding.
- [x] 1.5 Implement the `go-git` fallback: `exec.LookPath("git")` miss → `gogit.PlainInit`.
- [x] 1.6 Emit the `INFO`-level "bundled go-git" log line when the fallback fires.
- [x] 1.7 Propagate non-`ErrNotFound` exec errors unchanged (no silent retry via fallback).

## 2. Unit tests for `internal/storeinit/`

- [x] 2.1 Port `TestEnsureStoreScaffolded_CreatesDirAndTemplates` → `TestBootstrap_CreatesDirAndTemplates`.
- [x] 2.2 Port `TestEnsureStoreScaffolded_Idempotent` → `TestBootstrap_Idempotent`.
- [x] 2.3 Add `TestBootstrap_FallsBackToGoGit_WhenGitMissing` — rebind `ExecGitInit` to return `exec.ErrNotFound`, assert `.git/HEAD` exists + log contains `"bundled go-git"`.
- [x] 2.4 Add `TestBootstrap_PropagatesNonNotFoundExecErrors` — rebind `ExecGitInit` to return a generic error, assert it propagates unchanged AND `GoGitInit` is NOT called.
- [x] 2.5 Add `TestBootstrap_SeedsIdentityDefaults` — start with empty config home, run `Bootstrap`, assert `profile_tag` and `machine_id` are seeded.
- [x] 2.6 Add `TestBootstrap_DoesNotClobberUserSetIdentity` — pre-write `profile_tag: macOS`, run `Bootstrap`, assert the value is untouched.
- [x] 2.7 Add a property-based idempotency test using `pgregory.net/rapid`: any sequence of `Bootstrap(t.TempDir())` leaves the templates unchanged across runs.

## 3. Rewire CLI callers

- [x] 3.1 Replace `internal/cli/scaffold.go`'s body with a thin delegator to `storeinit.Bootstrap` (or delete the file and inline the call in the 3 callers — pick whichever minimises diff churn).
- [x] 3.2 Update `internal/cli/apply.go:runApply` — the "auto-init store" branch calls `storeinit.Bootstrap` with the right args.
- [x] 3.3 Update `internal/cli/provider_cmd.go` — first-run provider wrapping calls `storeinit.Bootstrap`.
- [x] 3.4 Update `internal/cli/commands.go` / `internal/cli/register.go` if they reference the old scaffold package.
- [x] 3.5 Delete `internal/cli/template/store/` (it has been moved to `storeinit`).
- [x] 3.6 Delete or migrate `internal/cli/scaffold_test.go` — the new tests live under `internal/storeinit/`.

## 4. Integration test: git-less container

- [x] 4.1 Pick the smallest provider integration scaffold (`internal/provider/builtin/apt/integration/`) and parameterise its Dockerfile to optionally `apt remove --purge git`.
- [x] 4.2 Add an integration test case that asserts: on a container with no `git` binary, `hams apt install htop` still succeeds; the scaffolded `.git/HEAD` exists; the hams log (stderr) contains the string "bundled go-git".
- [x] 4.3 Ensure the existing "with git installed" apt integration path still passes (no regression).
- [x] 4.4 Gate the new test behind the standard integration harness so local `task test:itest:one PROVIDER=apt` exercises both legs.

## 5. Verification

- [x] 5.1 `go build ./...` passes with zero warnings.
- [x] 5.2 `task lint` passes (both `lint:go` and `lint:md`).
- [x] 5.3 `task test` passes (unit + integration, race detector on).
- [x] 5.4 `task check` as a whole passes.
- [x] 5.5 Manual smoke: on a throwaway debian:slim container without `git`, `hams apt install htop` scaffolds the store and records the install — verified by a shell one-liner or the new integration test above.
- [x] 5.6 OpenSpec `/opsx:verify` passes for this change (validates all artifacts cross-reference correctly).
- [x] 5.7 Archive the change via `/opsx:archive`.
