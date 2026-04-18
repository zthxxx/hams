# Spec delta — cli-architecture (i18n-builtin-provider-catalog)

## ADDED Requirements

### Requirement: Builtin providers route user-facing strings through i18n

Every builtin provider in `internal/provider/builtin/` SHALL route its user-facing strings — `hamserr.NewUserError` error messages, dry-run preview lines, and other prose shown to end-users — through the i18n catalogue via `i18n.T` or `i18n.Tf`. Hardcoded English literals in these sites SHALL NOT remain after this change ships.

Log statements (`slog.*`) are EXCLUDED from this requirement because slog records are consumed by log scrapers, not end-users; they remain English-only.

Help text rendered by `cli/provider_cmd.go:showProviderHelp` is EXCLUDED from the scope of this delta; its localisation is tracked separately.

Rationale: `cli-architecture/spec.md` already establishes that "all user-facing strings (errors, help text, prompts) go through" the catalogue. Enumerating the provider case explicitly closes a gap where the SHALL was not verifiable — previously a grep for `i18n\.` across `internal/provider/builtin/` returned zero results.

#### Scenario: Provider errors render in the active locale

- **GIVEN** the user's environment has `LANG=zh_CN.UTF-8`
- **AND** the `zh-CN.yaml` catalogue has translations for every `provider.*` key
- **WHEN** the user runs `hams apt install` with no package argument
- **THEN** the error message SHALL be rendered in Chinese
- **AND** no English literal from the Go source SHALL appear in the output

#### Scenario: English remains the fallback

- **GIVEN** the user's environment has no `LANG` or an unsupported locale
- **WHEN** the user runs `hams apt install` with no package argument
- **THEN** the error message SHALL be rendered in English via the canonical `en.yaml` catalogue
- **AND** the rendered English SHALL match what the pre-i18n hardcoded string produced (no user-visible regression for English speakers)

#### Scenario: Catalogue is enumerable via keys.go

- **WHEN** a translator lists the strings this project exposes
- **THEN** they SHALL find every provider key declared in `internal/i18n/keys.go`
- **AND** every such key SHALL have corresponding entries in `en.yaml` and `zh-CN.yaml`
- **AND** the test `TestProviderKeysResolveInEnglish` / `TestProviderKeysResolveInChinese` SHALL assert the resolution end-to-end

#### Scenario: Missing yaml entry fails the build

- **GIVEN** a developer adds a new `Provider*` constant to `internal/i18n/keys.go`
- **AND** forgets to add the entry to `en.yaml` or `zh-CN.yaml`
- **WHEN** `task check` runs
- **THEN** the provider-keys resolution test SHALL fail with a clear "missing translation for key X" message
- **AND** the failing test SHALL identify which key is missing from which locale
