## Context

Before this change, `hams` had two sudo-related code paths that were both fused to host state:

- `internal/cli/apply.go::runApply` called `sudo.NewManager().Acquire(ctx)` unconditionally. Every unit test that exercised `runApply` either prompted for a real `Password:` on the developer's terminal or silently blocked in CI.
- `internal/provider/builtin/apt` invoked a package-level `sudo.RunWithSudo(...)` helper that looked at `os.Getuid()` and shelled out to `/usr/bin/sudo`. There was no seam for tests.

The `isRoot` sentinel in `sudo.go` was already a `var` that tests monkey-patched — a hint that the design was pushing toward DI but hadn't closed the loop. Meanwhile `internal/sudo/sudo_test.go` could exercise the `isRoot` branch in isolation, but the call-site tests (`internal/cli/apply_test.go`, `internal/provider/builtin/apt/apt_test.go`) had no way to stub out the privilege-escalation boundary.

That boundary is exactly the kind of external-world touch-point that the project's testing philosophy says MUST be DI-isolated: "unit tests inject mock boundary-layer services and run without side effects on the host" (CLAUDE.md, `.claude/rules/code-conventions.md`). Real sudo behavior belongs in a Docker container, not on a developer laptop.

## Goals / Non-Goals

**Goals:**

- Eliminate any path where `go test ./...` can fire `/usr/bin/sudo` against the host.
- Split the sudo concern into two orthogonal interfaces — *acquire credentials* vs. *wrap a command* — so callers only depend on what they use.
- Provide first-class test doubles (`NoopAcquirer`, `DirectBuilder`, `SpyAcquirer`, `RecordingBuilder`) inside the `sudo` package so consumers don't reinvent mocks.
- Retain a clear, isolated target that exercises *real* sudo behavior — root-skips, NOPASSWD-passes, no-sudoers-fails — inside Docker, gated by the `sudo` build tag so `go test` never sees it.
- Make the CI job byte-for-byte reproducible locally (via `act`) and keep its image cache keyed on Dockerfile + `go.mod`/`go.sum` content hash.

**Non-Goals:**

- Sudo password *entry* from tests. The real-password path remains unreachable in automation; the Docker target substitutes NOPASSWD for "entered correct password" and an empty-sudoers user for "entered wrong password."
- Replacing urfave/cli's flag plumbing or restructuring command wiring beyond what sudo DI requires.
- A generic exec.Cmd DI framework. `CmdBuilder` is scoped to sudo-wrapping; other exec sites keep using `exec.CommandContext` directly until there is a concrete test-isolation need.
- Adopting a DI container. This was originally designed on Uber Fx but was rolled back to direct construction in `Execute()` (see Decision 3).

## Decisions

### Decision 1: Split `Acquirer` and `CmdBuilder` instead of one god-interface

Two distinct responsibilities live in `internal/sudo`:

- **Lifecycle** — `Acquire(ctx)` primes sudo credentials for the apply session; `Stop()` cancels the heartbeat goroutine. Only `runApply` needs this.
- **Construction** — `Command(ctx, name, args...) *exec.Cmd` returns a command that is either bare or sudo-prefixed based on `isRoot()`. Providers (currently apt) need this per-invocation.

Making these two interfaces means:

- `apt.Provider` depends on `CmdBuilder` only — it never sees `Acquire`. Test doubles for apt don't need to implement an acquisition no-op.
- `runApply` depends on `Acquirer` only — it never sees `Command`. Its tests don't need a command builder.
- Production `Manager` and `Builder` are independent structs; the heartbeat goroutine stays scoped to `Manager` without polluting the builder.

**Alternatives considered:** A single `Sudo` interface with both methods. Rejected because it forces every consumer to depend on both concerns and forces every test double to implement both (ISP violation).

### Decision 2: Ship production + test doubles in the same package

`internal/sudo/noop.go` ships `NoopAcquirer`, `SpyAcquirer`, `DirectBuilder`, `RecordingBuilder` alongside production `Manager` and `Builder`.

Keeping doubles in-package:

- Avoids a `sudotest` subpackage and the cyclical import dance that comes with it.
- Lets provider tests say `sudo.DirectBuilder{}` — same import path as the real code.
- Makes the double discoverable next to the production code it shadows.

The cost is a tiny amount of test-only code compiled into the production binary. Benchmarks show this is noise (<1KB); the doubles are plain structs with no init cost.

**Alternatives considered:** A `sudotest` package. Rejected — the import cycle is not a real risk here (no sudo-package consumer imports its own tests), and the extra package adds friction for every test site.

### Decision 3: Direct construction in `Execute()`, not Uber Fx

The original plan (in `tasks.md`) wired sudo via a `sudo.Module` Fx module. During implementation we reverted to a plain three-liner in `Execute()`:

```go
registry := provider.NewRegistry()
registerBuiltins(registry, &sudo.Builder{})
app := NewApp(registry, sudo.NewManager())
```

Fx's value is graph-based wiring for services with non-trivial dependencies or lifecycle hooks. `sudo` has neither: `Manager` and `Builder` are zero-arg constructors, and the CLI is short-lived. Adding `fx.New` / `fx.Populate` only hid the wiring behind indirection without buying anything.

**Alternatives considered:** Keeping Fx as originally planned. Rejected because it added ceremony without simplifying testing — tests already construct their own `sudo.NoopAcquirer{}` and `sudo.DirectBuilder{}` literals. `registerBuiltins` keeps `sudo.CmdBuilder` as a parameter so future callers could swap it out, which is the only flexibility we actually need.

### Decision 4: Overridable `isRoot` var, not `os.Getuid` at the call site

`internal/sudo/sudo.go` keeps:

```go
var isRoot = func() bool { return os.Getuid() == 0 }
```

Tests that need to exercise the non-root branch *without* running as non-root swap this var under `t.Cleanup(func() { isRoot = orig })`. This is the only in-package seam; call-site tests use the DI doubles instead.

**Alternatives considered:** Making `isRoot` a field on `Builder` / `Manager`. Rejected — doing so would force every production construction site to pass `os.Getuid`, and every unit test would need to construct a whole Builder just to verify arg shapes. The package-level var is limited to the sudo package and never leaks past the boundary.

### Decision 5: `//go:build sudo` tag for real-sudo tests, Docker for execution

Tests that exercise real sudo — `TestAcquire_AsRoot_Succeeds`, `TestAcquire_AsNonRoot_WithNOPASSWD_Succeeds`, `TestAcquire_AsNonRoot_WithoutSudo_Fails`, `TestBuilder_AsRoot_SkipsSudo`, `TestBuilder_AsNonRoot_PrependsSudo` — live in `internal/sudo/sudo_sudo_test.go` behind `//go:build sudo`.

Consequences:

- `go test ./...` in any environment (CI, laptop, container) never sees these tests. The host is immune.
- `go test -tags=sudo ./internal/sudo/...` runs them. The Dockerfile creates three users — root, `testuser` with NOPASSWD, and `nosudouser` with no sudoers entry — and `run-tests.sh` dispatches the right subset to each via `su -c`.
- `e2e/sudo/Dockerfile` pins `golang:1.25-bookworm` and bind-mounts the source at `/src`, so local and CI runs see the same binary layout.

**Alternatives considered:** Using `testing.Short()` or an env var. Rejected — build tags are the only mechanism that makes the source file literally invisible to `go test ./...`, which is the strongest possible guarantee.

### Decision 6: CI caches the sudo image by Dockerfile+go.mod+go.sum hash

`.github/workflows/ci.yml` hashes `e2e/sudo/Dockerfile`, `go.mod`, `go.sum` and uses the 12-char prefix as the image tag. If the image already exists, build is skipped; older tagged images are pruned. This keeps the `sudo:` job under ~30s on a warm runner while avoiding Docker-layer-cache corner cases on self-hosted runners.

**Alternatives considered:** `actions/cache` on the Docker layer store. Rejected — the cache round-trip is slower than a rebuild on `ubuntu-latest` for a ~250MB bookworm image, and tag-based caching is zero-config when using a shared daemon.

## Risks / Trade-offs

- **Risk:** A future provider forgets to take `CmdBuilder` as a constructor parameter and calls `exec.Command("sudo", ...)` directly. → **Mitigation:** the `sudo.RunWithSudo` escape hatch was removed, so anyone reaching for sudo has to go through `CmdBuilder`. Add a lint rule later if a grep-based CI check flags `"sudo"` literals outside `internal/sudo`.
- **Risk:** The Docker sudo job drifts from the laptop environment (different sudo version, different PAM config). → **Mitigation:** the Dockerfile pins `golang:1.25-bookworm`; the container is the authoritative environment. Developers run `act push -j sudo` locally via the `test:sudo` Taskfile entry.
- **Risk:** `isRoot` being a package-level var is shared mutable state. Parallel tests that flip it can race. → **Mitigation:** the two call-site tests (`TestBuilder_Root_SkipsSudo`, `TestBuilder_NonRoot_WrapsSudo`) restore the original value in `t.Cleanup` and are in the same package, so `go test -race` has surfaced no issues. If we add more branches, move to `t.Setenv`-style scoping or make it a field.
- **Trade-off:** Shipping test doubles in the production package adds a few hundred lines to `internal/sudo`. Worth it for the one-import ergonomics; negligible binary-size impact.
