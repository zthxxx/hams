# Proposal: Typed i18n message-key catalog

## Why

Every `i18n.T(...)` / `i18n.Tf(...)` call-site passes a string literal message ID. Typos compile silently (the missing-key fallback returns the key verbatim at runtime), translators discover missing entries only after shipping, and renaming a key means grepping every call-site by hand. `origin/local/loop`'s `internal/i18n/keys.go` (commit `2ccd9aa`) solves this by declaring every message ID as a Go `const` with doc comments; call-sites reference the constant (`i18n.T(i18n.GitUsageHeader)`), making typos compile errors and the catalog discoverable via IDE go-to-definition.

## What changes

1. `internal/i18n/keys.go` — new file declaring every message ID currently in `locales/*.yaml` as an exported `const`. Doc comments describe the call-site context for translators.
2. Every existing `i18n.T("literal")` / `i18n.Tf("literal", ...)` call-site in `internal/**` rewritten to use the typed constant (`i18n.T(i18n.<KeyName>)`).
3. `internal/i18n/i18n_test.go` — new `TestCatalogCoherence_EveryTypedKeyResolves` reads both `en.yaml` and `zh-CN.yaml` directly and asserts every typed constant has a declared translation. A missing entry in either locale fails the test — which means "added a new `const` but forgot to update zh-CN.yaml" now fails CI.

## Impact

- **Capability `cli-architecture` / "Internationalization"** — the i18n module's message-key surface is now a typed catalog rather than a set of stringly-typed identifiers.
- **Developer experience** — typos are compile errors; `go to definition` on the key name jumps to the doc comment; `rg '\bGitUsageHeader\b'` finds every call-site instantly.
- **Translator experience** — the catalog-coherence test flags any translation gap immediately.
- **Behavior** — unchanged (runtime lookup goes through the same T/Tf path).
