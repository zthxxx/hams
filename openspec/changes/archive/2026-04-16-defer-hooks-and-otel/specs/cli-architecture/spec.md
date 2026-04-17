# cli-architecture — Spec Delta (OTel CLI-integration defer)

## MODIFIED Requirements

### Requirement: OTel Observability — CLI Integration Deferred to v1.1

The `internal/otel/` package provides `Session`, `Span`, `LocalFileExporter` with trace/metric file output to `${HAMS_DATA_HOME}/otel/`. The executor accepts an optional `otelSession ...*otel.Session` variadic. But:

- **Every caller of `provider.Execute` passes no session.** `internal/cli/apply.go:444` is the sole caller; `provider.Execute(ctx, p, actions, sf)` — the variadic arg is empty.
- **No root `hams.apply` / `hams.refresh` spans are created.** `internal/cli/apply.go` never calls `otel.NewSession()`.
- **Sampling (>200 resources) is not implemented.** `internal/otel/otel.go` has no threshold check.

Therefore: in v1, configuring OTel has no user-visible effect; no traces or metrics are written to `${HAMS_DATA_HOME}/otel/`. The package is ready scaffolding for v1.1.

#### Scenario: v1 does not produce OTel output

- **WHEN** a user runs `hams apply` in v1 (regardless of env vars like `HAMS_OTEL_*`)
- **THEN** no file SHALL appear under `${HAMS_DATA_HOME}/otel/traces/` or `${HAMS_DATA_HOME}/otel/metrics/`
- **AND** no error is emitted about missing OTel — the absence is silent.

#### Scenario: OTel scaffolding preserved for v1.1

- **WHEN** a v1.1 release wires `otel.NewSession()` into the `runApply` and `runRefresh` entry points and passes sessions through to `provider.Execute(..., session)`
- **THEN** the existing `internal/otel/otel.go` `LocalFileExporter` SHALL write JSON trace + metric records without further refactoring
- **AND** the executor's per-action `StartSpan`/`EndSpan` calls (already in `executor.go:100-150`) SHALL populate those records.

## Why deferred (not removed)

- `internal/otel/` is ~300 lines of working exporter + span-tracking code.
- Removing would force re-implementation in v1.1 with identical structure.
- Documenting the gap is the honest architectural move — matches the precedent set by `--hams-lucky` deferral (commit `f4c0f20`).
