# cli-architecture — Spec Delta

## MODIFIED Requirements

### Requirement: `--hams-lucky` Flag Behavior — Deferred to v1.1

The `--hams-lucky` flag and the LLM-driven enrichment chain that consumes it SHALL be marked as **scaffolded but not user-functional** in v1. The flag parses without error but is silently dropped at the provider boundary; no LLM call is made; no tags are auto-recorded; no TUI prompt is suppressed (because no provider invokes the TUI prompt to begin with).

This SHALL be acknowledged honestly in the user-facing spec until at least one provider implements the `provider.Enricher` interface (`Enrich(ctx, resourceID) error`) and consumes `hamsFlags["lucky"]`.

#### Current shipped behavior (as of v1)

- `splitHamsFlags()` extracts `--hams-lucky` into `hamsFlags["lucky"]`. ✓
- `RunTagPicker(llmTags, existingTags []string, lucky bool)` accepts the flag and short-circuits when `lucky == true`. ✓
- `Enricher` interface is defined at `internal/provider/provider.go`. ✓
- `runEnrichPhase` in `internal/cli/apply.go` iterates registered providers and type-asserts to `Enricher`. ✓
- **Zero of 15 builtin providers implement `Enrich`.** The type assertion always fails; the loop body never executes.
- **Zero providers read `hamsFlags["lucky"]`.** The flag never affects behavior.
- **`llm.Recommend()` has zero production callers.** Configuring `llm_cli` via `hams config set llm_cli claude` writes the value but no code path reads it.

#### Scenario: --hams-lucky flag is parsed without error

- **WHEN** the user runs `hams brew install git --hams-lucky`
- **THEN** the CLI SHALL extract the `lucky` flag into the per-command `hamsFlags` map without warning or error
- **AND** the install SHALL proceed as if the flag were absent (provider does not yet read the flag).

#### Scenario: LLM enrichment scaffolding does not invoke LLM in v1

- **WHEN** any user runs any `hams <provider> install ...` command in v1
- **THEN** no LLM subprocess SHALL be spawned regardless of `--hams-lucky` or `llm_cli` config
- **AND** no tags or intro fields SHALL be auto-populated in the resulting hamsfile entry.

#### Scenario: Spec promises scenarios deferred to v1.1

- **WHEN** a v1.1 release wires at least one provider's `Enrich` method to call `llm.Recommend()` and read `hamsFlags["lucky"]`
- **THEN** the v1.1 spec SHALL re-enable the original `--hams-lucky skips tag picker` and `--hams-lucky with no LLM configured` scenarios as user-facing requirements
- **AND** until then, those scenarios SHALL be considered DEFERRED, not failing, requirements.

## Why deferred (not removed)

- The plumbing (`Enricher` interface, `RunTagPicker(lucky)`, `EnrichAsync`, `EnrichCollector`, `runEnrichPhase`, `llm.Recommend`, config field `LLMCLI`) is end-to-end correct and ready for a provider implementation to plug in.
- Removing the scaffolding (~250 lines across `internal/llm/`, `internal/tui/`, `internal/provider/`, `internal/cli/`) would force re-implementation in v1.1 with identical structure.
- The honest architectural call is "documented gap, not dead code" — distinct from the `CLIHandler` interface (which had no plumbing reachable from any production code path and was removed in commit `10de4bd`).
