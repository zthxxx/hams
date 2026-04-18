# Proposal: Route Builtin-Provider User-Facing Strings Through the i18n Catalogue

## Why

`openspec/specs/cli-architecture/spec.md:103-116` mandates:

> "The i18n module SHALL provide a message catalog interface that all
> user-facing strings (errors, help text, prompts) go through."

Today, `rg 'i18n\.' internal/provider/builtin/` returns zero results.
Every `hamserr.NewUserError` call, every dry-run preview line, every
usage hint in every builtin provider is a hardcoded English string.
Non-English users cannot see a localized message from a provider even
when `LANG=zh_CN.UTF-8` is set — directly contradicting the SHALL.

Docs/notes/dev-vs-loop-gap-analysis.md #10 spelled this out:

> "Both branches violate the SHALL. The fix touches every provider and
> is not a pick-one-from-the-other port; both need new code."

## What Changes

- Extend `internal/i18n/keys.go` with:
  - A small set of **parameterised shared keys** that the
    consistent-pattern usage errors share across providers:
    `provider.err.requires-resource`, `provider.err.requires-at-least-one`,
    `provider.usage.basic`, `provider.dry-run.would-run`.
  - Per-provider specialised keys for any message that cannot be
    expressed via the shared templates.
- Fill `internal/i18n/locales/en.yaml` (canonical) and `zh-CN.yaml`
  (complete translations) with the new keys.
- Route every `hamserr.NewUserError(...)` call in every builtin provider
  through `i18n.T(...)` / `i18n.Tf(...)`.
- Route every obvious dry-run preview line (`fmt.Fprintf(stdout, "[dry-run]
  Would install: …")`) through `i18n.Tf`.
- Keep `slog.*` log records in English; logs are not user-facing prose
  for the purposes of the SHALL.

## Capabilities

### New Capabilities

None — the i18n capability already exists; this change expands its
reach.

### Modified Capabilities

- `cli-architecture` — clarify that the "go through the catalog" SHALL
  is enforceable for builtin providers (not just the CLI dispatcher).
  Delta under `specs/cli-architecture/spec.md`.

## Impact

- Affected code:
  - `internal/i18n/keys.go` — ~15 new exported string constants.
  - `internal/i18n/locales/en.yaml` — new entries.
  - `internal/i18n/locales/zh-CN.yaml` — new entries.
  - `internal/provider/builtin/*/*.go` — every `NewUserError` + dry-run
    site rewritten.
- Affected tests:
  - Per-provider unit tests that assert error text update to use
    `i18n.T(...)` or `i18n.Tf(...)` for the expected string.
  - NEW: `internal/i18n/i18n_providers_test.go` sanity-checks that
    every provider key resolves (no missing yaml entries).
- No CLI behaviour change for `LANG=en_US` users; `LANG=zh_CN.UTF-8`
  users get localized errors.
