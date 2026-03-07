## ADDED Requirements

### Requirement: OTel SDK initialization

The CLI architecture SHALL initialize the OpenTelemetry SDK during Fx bootstrap in `cmd/hams/main.go`. The SDK MUST configure a `TracerProvider` and a `MeterProvider`, both using the local file exporter. The SDK MUST NOT configure an OTel Logs SDK. The resource attributes MUST include `service.name=hams`, `service.version=<build-version>`, and `host.name=<machine-id>`.

#### Scenario: SDK bootstrap on hams apply

- **WHEN** the user runs `hams apply`
- **THEN** Fx initializes the OTel `TracerProvider` and `MeterProvider` with the local file exporter before any provider executes

#### Scenario: SDK bootstrap on hams refresh

- **WHEN** the user runs `hams refresh`
- **THEN** Fx initializes the OTel `TracerProvider` and `MeterProvider` identically to `hams apply`

#### Scenario: OTel disabled for non-instrumented commands

- **WHEN** the user runs a command that does not perform apply or refresh (e.g., `hams version`, `hams config`)
- **THEN** the OTel SDK MUST NOT be initialized and no trace or metric data SHALL be emitted

### Requirement: Root span per invocation

Each `hams apply` or `hams refresh` invocation SHALL create exactly one root span. The root span MUST have the name `hams.apply` or `hams.refresh` respectively. The root span MUST carry the attributes: `hams.profile` (active profile tag), `hams.providers.count` (number of providers executed), and `hams.result` (`success`, `partial-failure`, or `failure`).

#### Scenario: Successful apply creates root span

- **WHEN** `hams apply` completes with all resources in `ok` state
- **THEN** a root span named `hams.apply` is emitted with `hams.result=success` and `otel.status_code=OK`

#### Scenario: Partial failure sets result attribute

- **WHEN** `hams apply` completes with some resources in `failed` state and some in `ok`
- **THEN** the root span has `hams.result=partial-failure` and `otel.status_code=ERROR`

### Requirement: Provider child spans

For each provider executed during apply or refresh, the system SHALL create a child span under the root span. The span name MUST be `hams.provider.<provider-name>` (e.g., `hams.provider.homebrew`). Each provider span MUST carry attributes: `hams.provider.name`, `hams.provider.resource_count` (total resources in this provider), and `hams.provider.failed_count`.

#### Scenario: Multiple providers create sibling spans

- **WHEN** `hams apply` executes Homebrew and pnpm providers sequentially
- **THEN** two child spans `hams.provider.homebrew` and `hams.provider.pnpm` appear under the root span, each with their own timing and attributes

### Requirement: Resource operation grandchild spans

For each resource operation (probe, install, or remove) within a provider, the system SHALL create a grandchild span under the provider span. The span name MUST be `hams.resource.<action>` where `<action>` is one of `probe`, `install`, or `remove`. Each resource span MUST carry attributes: `hams.resource.id` (package name or URN), `hams.resource.action`, `hams.resource.result` (`ok`, `failed`, or `skipped`), and `hams.provider.name`.

#### Scenario: Probe span for a package resource

- **WHEN** the Homebrew provider probes the `git` package during refresh
- **THEN** a span `hams.resource.probe` is emitted under `hams.provider.homebrew` with `hams.resource.id=git` and `hams.resource.action=probe`

#### Scenario: Failed install records error on span

- **WHEN** a resource install fails with an error
- **THEN** the resource span has `hams.resource.result=failed`, `otel.status_code=ERROR`, and the span records the error message via `span.RecordError`

#### Scenario: Skipped resource records skip reason

- **WHEN** a resource is skipped because it is already in `ok` state
- **THEN** the resource span has `hams.resource.result=skipped` and the span duration is near-zero

### Requirement: Apply duration metric

The system SHALL record a histogram metric `hams.apply.duration` (unit: milliseconds) measuring the total wall-clock time of each `hams apply` or `hams refresh` invocation. The metric MUST have attribute `hams.command` (`apply` or `refresh`) and `hams.result`.

#### Scenario: Duration recorded after apply

- **WHEN** `hams apply` completes (success or failure)
- **THEN** a `hams.apply.duration` histogram data point is recorded with the elapsed time in milliseconds

### Requirement: Provider failure rate metric

The system SHALL record a counter metric `hams.provider.failures` incremented once per provider that has at least one failed resource. The metric MUST have attribute `hams.provider.name`.

#### Scenario: Provider with failures increments counter

- **WHEN** the Homebrew provider finishes with 2 failed resources out of 10
- **THEN** `hams.provider.failures` is incremented by 1 with `hams.provider.name=homebrew`

#### Scenario: Provider with no failures does not increment

- **WHEN** the pnpm provider finishes with all resources in `ok` state
- **THEN** `hams.provider.failures` is not incremented for `hams.provider.name=pnpm`

### Requirement: Resource count metrics

The system SHALL record a counter metric `hams.resources.total` with attributes `hams.resource.result` (`ok`, `failed`, `skipped`) and `hams.provider.name`, incremented by the count of resources in each result category per provider.

#### Scenario: Resource counts after mixed apply

- **WHEN** the Homebrew provider processes 8 ok, 1 failed, and 3 skipped resources
- **THEN** `hams.resources.total` is incremented by 8 (result=ok), 1 (result=failed), and 3 (result=skipped), all with `hams.provider.name=homebrew`

### Requirement: Probe duration metric

The system SHALL record a histogram metric `hams.probe.duration` (unit: milliseconds) measuring the wall-clock time of each provider's probe phase. The metric MUST have attribute `hams.provider.name`.

#### Scenario: Probe duration per provider

- **WHEN** the Homebrew provider's probe phase takes 1200ms
- **THEN** a `hams.probe.duration` data point of 1200 is recorded with `hams.provider.name=homebrew`

### Requirement: Local file exporter

The system SHALL export traces and metrics to the local filesystem at `${HAMS_DATA_HOME}/otel/`. Trace data MUST be written to `${HAMS_DATA_HOME}/otel/traces/` and metric data to `${HAMS_DATA_HOME}/otel/metrics/`. Each file MUST be named with the pattern `<YYYY-MM-DDTHHmmss>-<trace-id-prefix>.json`. The exporter MUST write JSON-encoded OTLP protobuf data (i.e., the standard OTLP JSON serialization format). The system MUST NOT support OTLP network endpoints in v1.

#### Scenario: Trace file written after apply

- **WHEN** `hams apply` completes and the OTel SDK shuts down
- **THEN** a JSON file appears in `${HAMS_DATA_HOME}/otel/traces/` containing the root span and all child/grandchild spans for that invocation

#### Scenario: Metric file written after apply

- **WHEN** `hams apply` completes and the OTel SDK shuts down
- **THEN** a JSON file appears in `${HAMS_DATA_HOME}/otel/metrics/` containing all histogram and counter data points recorded during the invocation

#### Scenario: Data directory created automatically

- **WHEN** `${HAMS_DATA_HOME}/otel/traces/` or `${HAMS_DATA_HOME}/otel/metrics/` does not exist
- **THEN** the exporter MUST create the directories with mode 0700 before writing

### Requirement: Exporter interface for future extensibility

The OTel integration MUST use an internal `Exporter` interface that abstracts the export destination. The local file exporter SHALL be the only implementation in v1. The interface MUST accept the standard OTel SDK `SpanExporter` and `MetricReader` contracts so that an OTLP gRPC/HTTP exporter can be added in a future version by providing an alternative implementation selectable via `hams.config.yaml`.

#### Scenario: Exporter resolved from config

- **WHEN** `hams.config.yaml` does not contain an `otel.exporter` field (or it is set to `file`)
- **THEN** the system uses the local file exporter

#### Scenario: Unknown exporter value rejected

- **WHEN** `hams.config.yaml` contains `otel.exporter: otlp`
- **THEN** the system exits with a clear error message indicating that OTLP export is not supported in this version

### Requirement: Graceful shutdown and flush

The OTel SDK MUST flush all pending spans and metrics before the process exits. The shutdown MUST be triggered during Fx lifecycle `OnStop`. The shutdown MUST enforce a hard timeout of 5 seconds to prevent hanging on exit.

#### Scenario: Clean shutdown on successful apply

- **WHEN** `hams apply` completes and the process exits normally
- **THEN** all spans and metrics are flushed to disk before the process terminates

#### Scenario: Shutdown on SIGINT

- **WHEN** the user presses Ctrl+C during `hams apply`
- **THEN** the Fx shutdown sequence fires, the OTel SDK flushes within 5 seconds, and the process exits

#### Scenario: Shutdown timeout prevents hang

- **WHEN** the file exporter is blocked (e.g., disk full)
- **THEN** the OTel shutdown completes within 5 seconds and the process exits with an error logged (not a fatal crash)

### Requirement: Negligible performance overhead

The OTel instrumentation MUST NOT add more than 5ms of overhead to the total apply/refresh wall-clock time under normal conditions (fewer than 500 resources). Span creation and attribute setting MUST use synchronous in-process calls with no network I/O. File writes MUST occur only at shutdown (batch export), not per-span.

#### Scenario: Overhead within budget for typical apply

- **WHEN** `hams apply` processes 50 resources across 5 providers
- **THEN** the OTel overhead (span creation + attribute setting + shutdown flush) is less than 5ms

### Requirement: Sampling for large applies

When the number of resource operations in a single invocation exceeds 200, the system SHALL switch to a tail-sampling strategy that keeps all failed/errored spans but samples successful resource-level spans at a 1-in-10 rate. Provider-level and root spans MUST always be retained. The sampling threshold MUST be configurable via `hams.config.yaml` field `otel.sampling-threshold` (default: 200).

#### Scenario: All spans retained below threshold

- **WHEN** `hams apply` processes 150 resources
- **THEN** all spans (root, provider, resource) are exported without sampling

#### Scenario: Successful spans sampled above threshold

- **WHEN** `hams apply` processes 500 resources with 480 successful and 20 failed
- **THEN** all 20 failed resource spans are exported, approximately 48 successful resource spans are exported (1-in-10), and all provider and root spans are exported

#### Scenario: Custom threshold from config

- **WHEN** `hams.config.yaml` contains `otel.sampling-threshold: 100`
- **THEN** sampling activates when resource operations exceed 100 instead of the default 200
