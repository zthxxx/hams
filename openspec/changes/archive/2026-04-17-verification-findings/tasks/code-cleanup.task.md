# Task: Remove Dead CLIHandler Interface

## Finding

`internal/provider/provider.go:169-173` defines:

```go
// CLIHandler is an optional interface for providers that handle CLI subcommands.
type CLIHandler interface {
    // HandleCLI processes the raw CLI args after the provider name.
    HandleCLI(ctx context.Context, args []string) error
}
```

A `rg CLIHandler` across the whole repo returns only the definition — nothing references it, no provider implements `HandleCLI`, no `interface{}` type-assertion checks for it, no call site dispatches through it.

The interface actually used at runtime is `ProviderHandler` in `internal/cli/provider_cmd.go:13-20`:

```go
type ProviderHandler interface {
    Name() string
    DisplayName() string
    HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error
}
```

All 15 builtin providers implement `HandleCommand` (with the four-arg signature), and `routeToProvider` in `provider_cmd.go:42` dispatches through it.

## Why This Matters

Dead interface in a foundational `internal/provider` package confuses contributors who read the package trying to understand the provider contract — they see `CLIHandler` documented as "optional", try to implement `HandleCLI`, and discover only after compilation that the real contract is elsewhere.

## Decision (Architect View)

Remove `CLIHandler` entirely. The repository principle (`CLAUDE.md` → "Don't add features, refactor code, or make improvements beyond what was asked") does not apply to deleting verifiably dead code — the agent-behavior rule explicitly says: "If you are certain that something is unused, you can delete it completely."

Do **not** rename `ProviderHandler` to `CLIHandler` or move it into the provider package. Reasoning:

- `ProviderHandler` has a `Name()` + `DisplayName()` contract because the CLI router needs to advertise providers by name in help output and dispatch by name in command routing. Those responsibilities belong to the `cli` package.
- `HandleCommand` takes `hamsFlags` + `*GlobalFlags` — these are CLI-layer concerns (hams-prefix extraction, global flag parsing). Exposing them in `internal/provider` would leak CLI concerns into the domain layer.
- The current split is actually correct: `internal/provider` defines the domain contract (Manifest, Bootstrap, Probe, Plan, Apply, Remove, List, Enricher); `internal/cli` defines the delivery contract (ProviderHandler).

## Subtasks

- [x] Remove lines 169-173 from `internal/provider/provider.go`.
- [ ] Update `openspec/specs/provider-system/spec.md` if it documents the `HandleCLI` signature — replace with a pointer to `internal/cli/provider_cmd.go` `ProviderHandler` as the CLI-dispatch contract, and clarify that CLI dispatch is an `internal/cli`-layer concern not a core-provider-interface concern. (Grep'd — spec never referenced `HandleCLI` or `CLIHandler`, so no spec update needed.)
- [x] Run `task check` — must pass with zero new issues.
- [x] `rg HandleCLI` across the repo must return zero hits.
- [x] `rg CLIHandler` across the repo must return zero hits.
