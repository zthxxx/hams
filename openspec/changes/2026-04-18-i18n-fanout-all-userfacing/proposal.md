# Proposal: Fan-out i18n coverage to every user-facing string

## Why

Dev shipped the i18n infrastructure (`internal/i18n/i18n.go`, `locales/{en,zh-CN}.yaml`, typed constants in `internal/i18n/keys.go`), but only ~15 call-sites actually route through it. An audit shows ~50 `hamserr.NewUserError(...)` primary messages and 100+ `fmt.Print*` user-facing prose strings still hard-coded in English. `openspec/specs/cli-architecture/spec.md` "Internationalization" mandates that **every** user-facing string goes through the i18n catalog — the current state is a visible gap between spec and code.

The typed-keys change (`2026-04-18-typed-i18n-keys`) locked in the compile-time discipline for EXISTING translated strings; this change finishes the job by translating the remaining call-sites.

## What changes

1. **Audit** every `hamserr.NewUserError(...)` in `internal/` (excluding `*_test.go`) and route the primary message (first string after the exit code) through `i18n.T(...)` or `i18n.Tf(...)`. Suggestion strings in the same call may stay English today; keys added as they land.
2. **Audit** every `fmt.Print*`, `fmt.Fprint*` call-site in `internal/` that writes user-facing prose (stdout/stderr, not slog/debug) and route through `i18n.T(...)` or `i18n.Tf(...)`.
3. **Extend the typed-key catalog** (`internal/i18n/keys.go` + `locales/{en,zh-CN}.yaml`) with one key per unique message pattern. Template variables use Go template syntax (`{{.Pkg}}`, `{{.Path}}`) to keep strings translatable when they contain dynamic values.
4. **Extend the catalog-coherence test** (`internal/i18n/i18n_test.go::TestCatalogCoherence_EveryTypedKeyResolves`) — every new key added to both `en.yaml` and `zh-CN.yaml`, test invariant holds.
5. **Key naming convention** stays consistent: `<capability>.<component>.<short-id>` — per keys.go header. New families: `provider.err.*` for shared provider-level errors, `apply.status.*`, `refresh.status.*`, `store.status.*`, `config.status.*`, `version.status.*`, `upgrade.status.*`, `cli.err.*`.

Skipped (per scope): test files, `log/slog` calls, `internal/logging/**`, TUI internal rendering (`internal/tui/collapsible.go`, `internal/tui/picker.go`, `internal/tui/popup.go` — internal widgets, not direct CLI output; only `internal/tui/tui.go`'s `WarnNoTTY` is user-facing), debug-only traces, embedded code/command text (`brew`, `git`, etc.), and dry-run diagnostic lines that echo commands verbatim.

## Impact

- **Capability `cli-architecture` / "Internationalization"** — the requirement is now enforced in code: every user-facing string has a typed key with entries in both locales.
- **Users** — `LANG=zh_CN.UTF-8 hams apply` now shows Chinese for ALL prose, not just the ~15 already-translated messages.
- **Translators** — the catalog is the single source of truth; the coherence test blocks CI when a key is added without a translation.
- **Developers** — new user-facing prose MUST go through i18n via a typed key. This is reinforced by the existing pattern and by the large catalog already in place.

## Scope caveat

A handful of error messages embed dynamic values that are awkward to template (e.g., exit-specific context, multi-line shell commands). When the template complexity is unreasonable, a `// TODO(i18n):` marker is left and the call-site deferred. Those are noted in `tasks.md` and represent follow-up work, not correctness gaps — the fallback (English literal via `T()` passthrough) keeps UX working.
