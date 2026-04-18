# Spec delta — provider-system (provider-shared-abstraction-adoption)

## ADDED Requirements

### Requirement: Package-class providers route install/remove through the shared dispatcher

Package-class builtin providers (resource class 1 in `provider-system/spec.md`) SHALL route their CLI-first install and remove flows through `internal/provider.AutoRecordInstall` and `internal/provider.AutoRecordRemove` unless they document an explicit exemption in their own spec delta. The shared dispatcher encapsulates the "validate args → dry-run short-circuit → acquire single-writer lock → loop runner.Install → load hamsfile+state → append → save" sequence that every Package-class provider duplicates today.

Rationale: `CLAUDE.md` → Current Tasks requires that "extending with a new provider is a matter of filling in a well-defined template, not reimplementing the pattern from scratch." Before this requirement, the dispatcher existed (190 LoC) but had zero adopters — 14 provider packages each maintained their own near-identical install/uninstall bookkeeping. A template with zero adopters is speculation, not engineering.

Exemption process: a provider whose runner signature does not match `PackageInstaller` (`Install(ctx, pkg string) error`, `Uninstall(ctx, pkg string) error`) — for example, apt (batch install via a slice) or brew (install carries an `isCask` bool) — SHALL document the mismatch in its provider's spec delta AND the provider's package doc comment. A future change MAY extend the dispatcher with a second variant (e.g., `BatchPackageInstaller`) to reabsorb exempt providers.

#### Scenario: Reference adoption — cargo

- **GIVEN** the cargo provider (`internal/provider/builtin/cargo/`)
- **WHEN** `hams cargo install ripgrep` executes
- **THEN** `handleInstall` SHALL delegate the install loop + record flow to `provider.AutoRecordInstall`
- **AND** no inline `for _, crate := range crates { runner.Install(…) }` + hamsfile/state write loop SHALL remain in `cargo.go`

#### Scenario: Exempt provider documents the mismatch

- **GIVEN** a provider like `apt` whose `runner.Install(ctx, args []string)` doesn't fit `PackageInstaller`
- **WHEN** the provider ships
- **THEN** the provider's package doc (`apt/doc.go` or the top of its main file) SHALL carry a one-line note referencing this SHALL and the reason for the exemption
- **AND** the provider's spec delta SHALL list the exemption in its "Impact" section

#### Scenario: New Package-class provider uses the dispatcher by default

- **GIVEN** a new Package-class provider being added post-adoption
- **WHEN** the provider's runner signature matches `PackageInstaller`
- **THEN** the provider SHALL call `AutoRecordInstall` / `AutoRecordRemove` from its CLI handlers
- **AND** the provider SHALL NOT reimplement the lock + exec + record loop

#### Scenario: Dispatcher user-facing strings route through i18n

- **WHEN** `AutoRecordInstall` is invoked with zero packages
- **THEN** the returned `UserFacingError` SHALL have its message and usage-hint strings routed through `i18n.Tf` via `provider.UsageRequiresAtLeastOne`
- **AND** the returned error SHALL render in Chinese when `LANG=zh_CN.UTF-8` is set
