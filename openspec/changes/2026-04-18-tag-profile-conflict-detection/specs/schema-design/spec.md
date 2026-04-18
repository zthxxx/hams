# Spec delta: schema-design — DefaultProfileTag constant

## ADDED Requirement: DefaultProfileTag exported constant

`internal/config` SHALL expose `const DefaultProfileTag = "default"` as the canonical string returned when no CLI flag, config value, env var, or other source supplies a profile tag. Scaffolders (first-run store init, auto-init) SHALL seed this literal into freshly-written global configs so the on-disk value and the runtime fallback stay in lock-step.

`ResolveActiveTag(cfg, cliTag, cliProfile)` SHALL compose the CLI override (via `ResolveCLITagOverride`) with the merged config value with `DefaultProfileTag` as the lowest-precedence fallback.

`DeriveMachineID()` SHALL return a sanitized machine id:

1. `$HAMS_MACHINE_ID` env var if non-empty (sanitized).
2. `os.Hostname()` if the lookup succeeds and returns non-empty (sanitized).
3. `DefaultProfileTag` ("default") as the final fallback.

The `HostnameLookup` package-level `var` SHALL be the `os.Hostname` seam that unit tests swap with deterministic fakes.

#### Scenario: hostname lookup fails

- **Given** `os.Hostname()` errors (e.g., an incorrectly configured container)
- **When** `DeriveMachineID()` runs with `HAMS_MACHINE_ID` unset
- **Then** the return value is `DefaultProfileTag` (`"default"`).
