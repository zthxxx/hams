# Spec delta: provider-system — CLI verb ≡ Manifest.Name invariant

## ADDED Requirement: CLI verb and Manifest.Name SHALL be equal

For every builtin provider that exposes a single CLI verb, `Manifest().Name` (registry key, `apply --only=` filter, state file prefix, and default provider-priority entry) SHALL equal the CLI verb that users type. Exceptions are allowed only when the CLI verb is an aggregator that multiplexes two or more underlying providers, in which case the aggregator:

- SHALL be registered in `cliOnlyHandlers` as a `ProviderHandler`, and
- SHALL NOT have a `Manifest().Name` of its own (it is not a `Provider`; it does not participate in apply/refresh).
- The underlying providers MAY keep their original `Manifest().Name` values; the aggregator is responsible for mapping CLI args onto the right underlying provider.

#### Scenario: unified `hams git` aggregator

- **Given** `hams git clone …` and `hams git config …` exist
- **When** the CLI dispatches
- **Then** `hams git` is a `ProviderHandler` registered in `cliOnlyHandlers`; it routes to the two `Provider` implementations whose `Manifest().Name` values remain `"git-clone"` and `"git-config"` (because they are separate apply/refresh resources with distinct hamsfile + state files).

#### Scenario: single-provider `hams code`

- **Given** VS Code extensions is ONE underlying provider
- **When** the provider is registered
- **Then** `Manifest().Name` is `"code"`, the provider is in `cliProviders` directly, and no wrapper handler is needed.

## MODIFIED Requirement: Integration test MANIFEST_NAME override

The `MANIFEST_NAME` environment variable in `e2e/base/lib/provider_flow.sh::standard_cli_flow` SHALL be required only when an integration test exercises a provider whose CLI verb is an aggregator (currently `git`). For single-provider CLI verbs, the default (`MANIFEST_NAME=$provider`) SHALL apply; no per-test override is necessary.
