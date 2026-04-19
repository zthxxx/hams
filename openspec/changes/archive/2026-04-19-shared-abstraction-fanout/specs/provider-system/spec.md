# Provider System — shared-abstraction-fanout deltas

## ADDED Requirements

### Requirement: Passthrough flow MUST honour DryRun

Every provider's CLI passthrough flow MUST route through
`provider.Passthrough(ctx, tool, args, flags)` so that
`flags.DryRun` produces a `[dry-run] Would run: <tool> <args>`
preview on `flags.Stdout()` instead of executing the wrapped
binary. Direct calls to `exec.CommandContext` for the
passthrough purpose are forbidden.

#### Scenario: Builtin provider passthrough honours --dry-run

WHEN a user runs `hams brew tap homebrew/cask-fonts --dry-run`
THEN `homebrew.HandleCommand` SHALL call `provider.Passthrough`
which prints `[dry-run] Would run: brew tap homebrew/cask-fonts`
to stdout and returns nil without exec'ing brew.

#### Scenario: PassthroughExec seam is swappable in unit tests

WHEN a unit test in any provider package needs to assert the
spawn argv for the passthrough branch
THEN it MAY rebind `provider.PassthroughExec` to a fake recorder
inside the test (with `t.Cleanup` restore) and observe the
captured (tool, args) without spawning a real process.

### Requirement: Package-class providers MUST use baseprovider helpers

Package-class builtin providers (resource class 1) MUST route
their hamsfile / config resolution through
`internal/provider/baseprovider`'s `EffectiveConfig`,
`HamsfilePath`, and `LoadOrCreateHamsfile` helpers rather than
inlining duplicate per-provider implementations. The
`baseprovider` package SHALL be the single source of truth for
how `--store`, `--profile`/`--tag`, and `--hams-local` flags
overlay onto a provider's effective config.

A provider MAY keep an inlined helper when it needs additional
behaviour beyond the shared shape; in that case the deviation
MUST be documented in a block comment at the top of the
provider package, mirroring the apt dispatcher exemption process.

#### Scenario: New package-class provider uses baseprovider

WHEN a new package-class provider is added (e.g., a future
`hams winget` for Windows)
THEN its `effectiveConfig`, `hamsfilePath`, and
`loadOrCreateHamsfile` helpers SHALL delegate to `baseprovider`
in one-line method bodies rather than re-implementing the
overlay/path/load logic from scratch.

#### Scenario: Test for shared overlay rules lives once

WHEN a developer needs to test that `--store` overrides
`cfg.StorePath` and `--profile` overrides `cfg.ProfileTag`
THEN the test SHALL live in
`internal/provider/baseprovider/baseprovider_test.go` and cover
every adopting provider transitively, instead of being copied
into each provider's test suite.
