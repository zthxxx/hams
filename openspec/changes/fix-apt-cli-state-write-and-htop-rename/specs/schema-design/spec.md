# Schema Design — Spec Delta

## MODIFIED Requirements

### Requirement: Resource lifecycle timestamps

The state file SHALL track the lifecycle of a resource using three timestamp fields with strict update semantics: `first_install_at`, `updated_at`, and `removed_at`.

#### Scenario: Remove transitions record removed_at

- **WHEN** the user runs `hams apt remove htop` (or any equivalent provider remove) for a resource with `first_install_at: T0`
- **THEN** the state entry SHALL have `state: removed`, `first_install_at: T0` (unchanged), `updated_at: T1`, and `removed_at: T1` where T1 is the current timestamp.
- **AND** the entry SHALL NOT be deleted from the state file — it remains for audit.
