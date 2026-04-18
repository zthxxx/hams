# Design: Provider-Wide i18n Catalogue

## Context

Zero `i18n.T` calls exist in any `internal/provider/builtin/` package
today. The SHALL ("all user-facing strings go through the catalogue")
spans 14 providers × ~5 sites each ≈ 70 call sites. Implementing them
as 70 individual keys would make the catalogue large and noisy; using
5 well-designed parameterised keys collapses that to a single shared
template for the dominant pattern.

## Goals

1. Satisfy the SHALL: every `hamserr.NewUserError` + every dry-run
   preview line in every builtin provider routes through `i18n.T` /
   `i18n.Tf`.
2. Minimise catalogue entry count via shared parameterised keys for
   the common "<provider> <verb> requires a <resource>" / "Usage:
   hams <provider> <verb> <placeholder>" / "[dry-run] Would <verb>: …"
   patterns.
3. Keep translator ergonomics: a new locale needs one yaml file that
   names every key. Go code does not embed literal English strings for
   these sites.

## Non-Goals

- Translating `slog.*` log records — those are English-only by design
  (they are consumed by log scrapers, not end-users).
- Translating package names, app identifiers, or technical tokens.
- Translating `showProviderHelp` (top-level `--help` text) —
  out-of-scope for this change, deferred to a follow-up if the
  SHALL re-reading requires it.

## Shared Key Templates

```yaml
- id: provider.err.requires-resource
  other: "{{.Provider}} {{.Verb}} requires a {{.Resource}}"

- id: provider.err.requires-at-least-one
  other: "{{.Provider}} {{.Verb}} requires at least one {{.Resource}}"

- id: provider.usage.basic
  other: "Usage: hams {{.Provider}} {{.Verb}} <{{.Placeholder}}>"

- id: provider.dry-run.would-run
  other: "[dry-run] Would run: {{.Cmd}}"

- id: provider.dry-run.would-install
  other: "[dry-run] Would install: {{.Cmd}}"

- id: provider.dry-run.would-remove
  other: "[dry-run] Would remove: {{.Cmd}}"
```

Call-site shape (apt, install, zero args):

```go
return hamserr.NewUserError(hamserr.ExitUsageError,
    i18n.Tf(i18n.ProviderErrRequiresResource, map[string]any{
        "Provider": "apt", "Verb": "install", "Resource": "package name",
    }),
    i18n.Tf(i18n.ProviderUsageBasic, map[string]any{
        "Provider": "apt", "Verb": "install", "Placeholder": "package",
    }),
)
```

This is 3-4 extra lines per call site — higher than the current
one-liner, but mechanical and local. Translators see one template key
regardless of provider/verb combination.

## Provider-Specific Keys

Not every error fits the shared template. Provider-specific keys live
under `provider.<name>.<verb>.<slot>`:

- `provider.apt.install.simulate-warning` — apt's
  "complex-invocation auto-record skipped" warning.
- `provider.git.clone.requires-remote` — git's clone-without-remote
  usage error.
- `provider.git.config.path-required` — git-config without a flag/path.
- `provider.homebrew.list.no-entries` — homebrew list's "nothing
  managed" line.

These are added as needed; the shared templates cover ~80% of sites.

## Rollout

Single atomic commit touches:

1. `internal/i18n/keys.go` — exported constants.
2. `internal/i18n/locales/en.yaml` — canonical entries.
3. `internal/i18n/locales/zh-CN.yaml` — translations.
4. Every file with `NewUserError` under `internal/provider/builtin/`.
5. Existing provider unit tests that assert error text update to use
   the i18n key lookup or its English rendering.

`task check` must pass before the commit lands. A dedicated test in
`internal/i18n/i18n_providers_test.go` iterates every exported
`Provider*` key and asserts `i18n.T(key) != key` for the English
locale, catching missing yaml entries at compile-test time.

## Test Plan

1. Unit test `TestProviderKeysResolveInEnglish` iterates every
   `provider.*` key in `keys.go` and asserts `i18n.T(key)` returns a
   non-empty string that is NOT the key itself (which signals a miss
   in the lookup).
2. Unit test `TestProviderKeysResolveInChinese` does the same for
   `zh-CN` — catches missing translations.
3. Update provider-package tests that match on error text: change
   string equality / `strings.Contains` checks to look for the
   translated form (either English or key-based).
4. Manual E2E check (doc only — no script change): set
   `LANG=zh_CN.UTF-8` and invoke each provider's install/remove error
   path, observe the Chinese text. Recorded in the archived change's
   `tasks.md` as a completed check.

## Alternatives Considered

**A. Per-provider specialised keys only (`provider.apt.install.requires-name`).**
Simple, no template engine. But yields ~70 keys across 14 providers,
much yaml noise, and translators see many near-identical strings. Rejected.

**B. Shared templates only, no provider-specific keys.**
Collapses too aggressively — apt's "complex-invocation simulate"
warning has no clean shared template. Rejected.

**Chosen: mix** — shared templates for the dominant pattern,
provider-specific keys for outliers.
