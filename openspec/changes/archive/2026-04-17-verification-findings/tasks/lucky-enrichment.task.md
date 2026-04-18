# Task: Wire `--hams-lucky` Through Enrichment Flow

## Finding

`openspec/specs/cli-architecture/spec.md` (the `--hams-lucky` section) says the flag SHALL:

1. Be extracted by `splitHamsFlags()` into the `hamsFlags` map. **✓ Implemented** at `internal/cli/flags.go:5-43`, test at `internal/cli/flags_test.go:43-50`.
2. Be propagated to the provider's `HandleCommand()`. **✓ Implemented** — `hamsFlags` is passed as the third argument.
3. Be forwarded from `HandleCommand()` into the enrichment flow. **✗ Not wired.** No provider reads `hamsFlags["lucky"]` and no enrichment call site consumes it.
4. When `true`, `RunTagPicker()` SHALL return LLM-recommended tags immediately without showing the TUI. **✓ Signature supports it** — `RunTagPicker(lucky bool, ...)` in `internal/tui/run.go`. But the caller always passes `false`.
5. `EnrichAsync()` SHALL auto-accept the LLM-generated intro when lucky mode is active. **✗ Not implemented.**

So the flag is parseable but inert — the user can type `hams brew install htop --hams-lucky` and nothing changes versus `hams brew install htop`.

## User-Workflow Impact

The intent of `--hams-lucky` is for CI/automation contexts ("I don't want to sit through a TUI picker"). Without wiring, AI agents and CI pipelines get the TUI prompt regardless of the flag — or hang when no TTY is attached.

## Architect Decision

Wire the flag through the two enrichment seams. Prefer a context-based propagation over a parameter-explosion approach:

```go
// internal/provider/enrichment.go (new or existing file)
type luckyKey struct{}

func WithLucky(ctx context.Context, lucky bool) context.Context {
    return context.WithValue(ctx, luckyKey{}, lucky)
}

func IsLucky(ctx context.Context) bool {
    v, _ := ctx.Value(luckyKey{}).(bool)
    return v
}
```

Then:

- `routeToProvider` reads `hamsFlags["lucky"]`, calls `provider.WithLucky(ctx, ...)`, passes augmented context to `HandleCommand`.
- `RunTagPicker` checks `IsLucky(ctx)` and short-circuits.
- `EnrichAsync` checks `IsLucky(ctx)` and auto-accepts.

Why context over parameter:

- Avoids a breaking signature change to `Enricher.Enrich(ctx, resourceID)`.
- Applies to the `hams apply` path too: apply-time enrichment can read lucky from context (set globally when user runs `hams apply --hams-lucky` as a hypothetical future flag).
- Matches the repo's existing `WithBootstrapAllowed` / `BootstrapAllowed` pattern in `internal/provider/bootstrap.go:45-62` — consistency with the domain idiom.

## Subtasks

- [ ] Define `WithLucky(ctx, bool)` and `IsLucky(ctx) bool` in `internal/provider/lucky.go` (or append to an existing small-context-helpers file if one exists).
- [ ] In `internal/cli/provider_cmd.go:42` (`routeToProvider`), extract `hamsFlags["lucky"]`, augment context before calling `handler.HandleCommand(ctx, ...)`.
- [ ] In `internal/tui/run.go`, add a path in `RunTagPicker` that checks `provider.IsLucky(ctx)` and returns LLM-recommended tags without rendering the TUI (requires threading ctx into `RunTagPicker` if not already present).
- [ ] In `internal/llm/enrich.go` (or wherever `EnrichAsync` lives), check `provider.IsLucky(ctx)` before soliciting user confirmation; auto-accept if true.
- [ ] Unit test: `hams brew install foo --hams-lucky` on a non-TTY stdin completes without hanging, records LLM-picked tags in the hamsfile, and exits 0.
- [ ] Unit test: without `--hams-lucky`, the same invocation on a non-TTY stdin either prompts (if supported) or fails with a clear "non-interactive requires --hams-lucky" error.
