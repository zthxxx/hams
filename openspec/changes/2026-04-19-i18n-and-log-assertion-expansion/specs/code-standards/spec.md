# Code Standards — i18n-and-log-assertion-expansion deltas

## ADDED Requirements

### Requirement: Every typed i18n constant MUST resolve in every locale

Every typed message-key constant in `internal/i18n/keys.go` MUST resolve to a non-key, non-empty string in every locale file under `internal/i18n/locales/*.yaml`.

The exhaustive form: every typed message-key constant declared in
`internal/i18n/keys.go` MUST resolve to a non-key, non-empty
string in EVERY locale file under `internal/i18n/locales/*.yaml`.

The contract is enforced by three layered tests:

1. `TestCatalogCoherence_EveryTypedKeyResolves`
   (`internal/i18n/catalog_coherence_test.go`) hand-maintains a
   slice of every exported constant in `keys.go` and asserts
   each appears as `id: <key>` in both locale files. Catches
   "added const, forgot YAML".
2. `TestLocalesAreInParity`
   (`internal/i18n/locale_parity_test.go`) dynamically diffs
   every non-English locale against `en.yaml` and fails on any
   missing or extra key. Catches "added en, forgot zh-CN".
3. `TestProviderKeysResolve{English,Chinese}`
   (`internal/i18n/i18n_providers_test.go`) iterates the
   `Provider*` constants and asserts `Tf` produces a
   non-key string with realistic template data. Catches
   unresolved `{{.Placeholder}}` regressions.

Together the three tests give bidirectional, interpolation-
aware coverage. Adding a new typed constant without translations,
or removing a YAML key without removing its constant, fails CI
before the regression lands.

#### Scenario: New typed constant gates the test

WHEN a developer adds a new constant to `keys.go` (e.g.,
`AutoInitDryRunIdentitySeed = "autoinit.dry_run.identity_seed"`)
WITHOUT adding the matching `id:` entries to `en.yaml` and
`zh-CN.yaml`
THEN `go test ./internal/i18n/...` SHALL fail with a "missing
translation" error citing the locale path and key name.

#### Scenario: Orphaned YAML key fails parity

WHEN a developer removes a constant from `keys.go` but leaves
the corresponding `id:` entry in the locale files
THEN the catalog-coherence test SHALL pass (no constant to
verify) but the parity test SHALL flag the orphan because
en.yaml's key set differs from the now-shrunk constant set.
(In practice the orphan is benign — the YAML entry just sits
there unused — so parity catches it as a hygiene issue, not a
correctness one.)

### Requirement: Integration tests MUST verify file-based slog emission

Every package-class provider's integration test MUST verify
that the rolling slog log file under
`${HAMS_DATA_HOME}/<YYYY-MM>/hams.YYYYMM.log` actually receives
the per-invocation session-start line and the provider-specific
slog records. The file-based check is in addition to (not in
place of) the existing stderr-based assertion in
`standard_cli_flow`.

Rationale: stderr captures what the user sees in one invocation;
the rolling log file captures what `hams logs` reads back later.
A regression where stderr emits but the file write silently
fails is invisible to stderr-only assertions.

The required helpers `assert_log_contains` and
`assert_log_records_session` live in `e2e/base/lib/assertions.sh`.

#### Scenario: apt integration test exercises both gates

WHEN `task ci:itest:run PROVIDER=apt` runs the apt container
test
THEN the script SHALL:

1. Set `HAMS_DATA_HOME=/tmp/test-apt-data` near the env block to
   isolate the rolling log directory.
2. After `standard_cli_flow apt install ...` (which already
   carries the stderr gate), trigger
   `hams --store=$HAMS_STORE apply --only=apt` to fire
   SetupLogging.
3. Call `assert_log_records_session "apt integration"` to
   verify the bootstrap record landed.
4. Call `assert_log_contains "apt provider records applied
   actions" "apt"` to verify provider-specific records landed.

#### Scenario: New provider integration test inherits the requirement

WHEN a new package-class provider's integration test is added
THEN the script SHALL call `assert_log_records_session` and
`assert_log_contains` with substrings unique to its provider
identity (matching the provider's slog.Info attributes).
