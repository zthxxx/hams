## MODIFIED Requirements

### Requirement: Internationalization via locale environment variables

The system SHALL detect the user's locale by reading environment variables in priority order: `LC_ALL`, `LC_CTYPE`, `LANG`. The locale string SHALL be parsed by stripping the encoding suffix (e.g., `.UTF-8`, `.ISO-8859-1`) first, then extracting the language and region components (e.g., `zh_TW.UTF-8` yields language `zh`, region `TW`). The system always enforces UTF-8 internally; the encoding suffix is ignored. If no supported locale is detected or the variables are unset, the system SHALL default to `en_US`. The i18n module SHALL provide a message catalog interface that all user-facing strings (errors, help text, prompts) go through. The v1 release SHALL ship with `en_US` and `zh_CN` translations; the architecture SHALL support additional locales without code changes.

Every user-facing string emitted from the CLI — including the primary message of every `hamserr.NewUserError(...)` call (first string argument after the exit code) and every `fmt.Print*` / `fmt.Fprint*` call that writes prose to stdout or stderr — SHALL be routed through the i18n catalog via a typed constant declared in `internal/i18n/keys.go`. Log records emitted via `log/slog` do NOT require i18n because they are operator-facing, not user-facing. Suggestion strings attached to `NewUserError` calls SHOULD go through the catalog; when a specific call-site would be awkward to template (e.g., embedded multi-line shell examples), a `// TODO(i18n):` marker SHALL be left and tracked as follow-up, but the primary message MUST still use a typed key.

#### Locale file resolution chain

The English locale file (`en.yaml`) SHALL always be loaded as the base. For non-English locales, the system SHALL resolve a single locale file using the following fallback chain (first match wins):

1. **Exact match**: `<lang>-<REGION>.yaml` (e.g., `zh-TW.yaml` for locale `zh_TW`)
2. **Base language**: `<lang>.yaml` (e.g., `zh.yaml`)
3. **Sibling match**: the first `<lang>-*.yaml` file alphabetically (e.g., `zh-CN.yaml` when only `zh-CN.yaml` exists and the locale is `zh_TW`)
4. **No match**: no additional locale file is loaded; rely on English only

At most one non-English locale file SHALL be loaded. The system loads either a full locale resource file or nothing — partial file loading is not supported.

#### i18n key fallback

For any i18n key lookup, if the key is missing from the loaded non-English locale, the system SHALL fall back to the English locale. If the key is also missing from English, the system SHALL return the key string itself as the message (fail-open for robustness).

#### Catalog coherence invariant

`internal/i18n/keys.go` SHALL declare every user-facing message ID as an exported `const`. The unit test `TestCatalogCoherence_EveryTypedKeyResolves` SHALL assert that every declared constant has corresponding entries in both `locales/en.yaml` and `locales/zh-CN.yaml`. Adding a new typed constant without a matching translation in BOTH locales SHALL fail CI.

#### Scenario: English locale detected

- **WHEN** the user's environment has `LANG=en_US.UTF-8`
- **THEN** all CLI output SHALL use English message strings

#### Scenario: Unsupported locale falls back to English

- **WHEN** the user's environment has `LANG=ja_JP.UTF-8` and no Japanese translation exists
- **THEN** the system SHALL fall back to `en_US` and all CLI output SHALL use English

#### Scenario: LC_ALL overrides LANG

- **WHEN** `LC_ALL=zh_CN.UTF-8` and `LANG=en_US.UTF-8`
- **THEN** the system SHALL use the `zh_CN` locale (if supported), as `LC_ALL` takes precedence

#### Scenario: No locale environment variables

- **WHEN** `LC_ALL`, `LC_CTYPE`, and `LANG` are all unset
- **THEN** the system SHALL default to `en_US`

#### Scenario: Non-CN Chinese locale falls back to zh-CN

- **WHEN** the user's environment has `LANG=zh_TW.UTF-8`
- **AND** no `zh-TW.yaml` or `zh.yaml` locale file exists
- **AND** `zh-CN.yaml` exists as the only `zh-*.yaml` file
- **THEN** the system SHALL load `zh-CN.yaml` via the sibling match rule and use Chinese translations

#### Scenario: Encoding suffix is stripped

- **WHEN** the user's environment has `LANG=zh_CN.UTF-8` or `LANG=zh_CN.GB2312`
- **THEN** the system SHALL strip the encoding suffix, parse `zh_CN`, and load `zh-CN.yaml` regardless of the original encoding

#### Scenario: All NewUserError primary messages go through the catalog

- **WHEN** any call to `hamserr.NewUserError` is compiled and executed under `LANG=zh_CN.UTF-8`
- **THEN** the primary message (first string after the exit code) SHALL be a `i18n.T(...)` / `i18n.Tf(...)` lookup against a typed `const` from `internal/i18n/keys.go`
- **AND** the Chinese translation SHALL be emitted on stderr (not the English source literal)

#### Scenario: All user-facing fmt.Print* output goes through the catalog

- **WHEN** any `fmt.Print*` or `fmt.Fprint*` call writes prose to stdout or stderr in `internal/**` (excluding `internal/logging/**`, `_test.go`, and `slog`-wrapping helpers) and the process runs under `LANG=zh_CN.UTF-8`
- **THEN** the string SHALL be a `i18n.T(...)` / `i18n.Tf(...)` lookup returning the Chinese translation

#### Scenario: Missing translation fails CI

- **WHEN** a developer adds a new typed constant to `internal/i18n/keys.go` without a corresponding `id:` entry in both `locales/en.yaml` and `locales/zh-CN.yaml`
- **THEN** `go test ./internal/i18n/...` SHALL fail in `TestCatalogCoherence_EveryTypedKeyResolves` naming the missing key + locale
