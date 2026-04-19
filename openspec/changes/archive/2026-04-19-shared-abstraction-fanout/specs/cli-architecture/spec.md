# CLI Architecture — shared-abstraction-fanout deltas

## ADDED Requirements

### Requirement: storeinit MUST expose fine-grained DI seams

The `internal/storeinit` package MUST expose `LookPathGit`,
`ExecCommandContext`, and `GitInitTimeout` as package-level
variables (not constants) so unit tests can simulate "git not on
PATH", record the spawn argv, or shrink the timeout independently,
without having to swap the entire `ExecGitInit` function.

The function-level `ExecGitInit` and `GoGitInit` seams from the
2026-04-18 storeinit-package change SHALL be retained for the
existing whole-flow swap-out tests; the new fine-grained seams
SHALL compose into the default `ExecGitInit` body so production
behaviour is unchanged.

#### Scenario: Unit test simulates missing git binary

WHEN a unit test needs to exercise the go-git fallback branch in
`storeinit.ensureGitRepo` without modifying the host PATH
THEN it MAY rebind `storeinit.LookPathGit` to return
`("", exec.ErrNotFound)` (with `t.Cleanup` restore) and assert
the fallback branch fires without touching the real `exec.LookPath`.

#### Scenario: Unit test asserts spawn argv

WHEN a unit test needs to assert that `git init --quiet <dir>` is
the canonical spawn shape
THEN it MAY rebind `storeinit.ExecCommandContext` to a recorder
that captures (name, args) and returns a no-op `exec.Cmd`
(`exec.CommandContext(ctx, "true")`).

#### Scenario: apt E0 integration test still uses real git path

WHEN the apt E0 container scenario runs `hams apt install htop`
on a machine where `/usr/bin/git` has been moved aside
THEN the storeinit fallback path SHALL still fire via the real
`LookPathGit` binding (production behaviour) and the "bundled
go-git" log line SHALL appear on stderr.
