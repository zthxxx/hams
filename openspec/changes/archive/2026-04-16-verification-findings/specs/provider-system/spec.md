# Delta — provider-system

## REMOVED

### Requirement: CLIHandler Optional Interface

The `CLIHandler` interface (with `HandleCLI(ctx, args)` method) declared in `internal/provider/provider.go` SHALL be removed. It was never implemented by any provider, never documented in any spec, and never referenced at any call site.

The actual CLI-dispatch contract — `ProviderHandler` with `HandleCommand(ctx, args, hamsFlags, flags)` — lives at `internal/cli/provider_cmd.go` and belongs to the `cli` package (not the core `provider` package), because the four-arg signature carries CLI-layer concerns (`hamsFlags`, `GlobalFlags`) that SHALL NOT leak into the domain-layer provider contract.

#### Scenario: Dead-code removal verified

- **WHEN** `rg CLIHandler` is run across the repository after this change
- **THEN** zero matches SHALL be returned
- **AND** `rg HandleCLI` SHALL also return zero matches

## MODIFIED

### Requirement: Provider Runtime Auto-Bootstrap — Multi-Provider DAG-Level Ordering

After investigation via `TestResolveDAG_ZeroIndegreePriority` at `internal/provider/dag_test.go`, the shipped ordering behavior is:

1. **Cross-DAG-level ordering** SHALL follow topological sort (dependencies always precede dependents).
2. **Same-DAG-level tie-breaking** SHALL be alphabetical by provider name (via `sort.Strings` at `internal/provider/dag.go:53`). The `provider_priority` list from `internal/config/config.go` `DefaultProviderPriority` (and user overrides via `provider_priority:` YAML key) SHALL NOT affect execution order for root-level (zero-dependency) providers.

The `provider_priority` list DOES affect:

- Display order in `registry.Ordered()` output (used by list commands and initial provider enumeration).
- Order of dependent-queue population when multiple providers become ready at the same DAG level after their dependency completes (subsequent additions in `dag.go:61-66` follow `dependents[]` slice order, which comes from the provider-iteration order in the initial DAG build, which comes from `Ordered()` output). This is a subtle residual effect — not a contract users should rely on.

The `provider_priority` list DOES NOT affect:

- Root-level (zero-dependency) provider execution order. Always alphabetical.

This is a deliberate architectural choice: alphabetical tiebreaking is deterministic without requiring any user configuration, while the priority list remains useful for display and as a hint to the DAG builder without being load-bearing on execution order.

#### Scenario: Multiple root-level providers ordered alphabetically

- **WHEN** `bash`, `apt`, and `cargo` are all registered with no `DependsOn` entries
- **AND** a user's `provider_priority: [bash, apt, cargo]` YAML override is active
- **THEN** the execution order SHALL be `apt, bash, cargo` (alphabetical), NOT the priority-list order
- **AND** a unit test SHALL exist that asserts this behavior so future changes cannot silently revert it

#### Scenario: Dependencies always precede dependents

- **WHEN** `vscode-ext` declares `DependsOn: [{Provider: "brew"}]` and `brew` declares `DependsOn: [{Provider: "bash", Script: "install brew"}]`
- **THEN** the execution order SHALL be `bash, brew, vscode-ext` regardless of alphabetical tiebreaking
- **AND** this ordering SHALL hold even if the `provider_priority` list is empty or contradictory
