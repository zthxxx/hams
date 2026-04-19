# CLI Architecture — i18n-and-log-assertion-expansion deltas

## ADDED Requirements

### Requirement: i18n catalog MUST cover the full CLI lifecycle

The `internal/i18n/keys.go` catalog MUST cover every user-facing
string emitted by the `internal/cli/**` packages and the builtin
provider command handlers, including (but not limited to):

- App metadata (`app.title`, `version.info`).
- Auto-init lifecycle (`autoinit.global_config_created`,
  `autoinit.store_created`, plus dry-run variants).
- User-facing error family (`ufe.no_store_configured` and its
  five `suggest_*` / `opt_out` variants).
- Apply / refresh status, summary, dry-run preview, and
  partial-failure error rendering.
- Store commands (`store.init.*`, `store.pull.*`,
  `store.commit.*`, `store.status.*`).
- Config commands (`config.set.*`, `config.unset.*`,
  `config.open.*`, `config.line.*`).
- List command (`list.no-resources-*`, `list.group-header`).
- Self-upgrade (`upgrade.brew.*`, `upgrade.dry-run`,
  `upgrade.success`).
- Sudo prompt (`sudo.prompt`).
- TUI fallbacks (`tui.warn-no-tty`).
- Bootstrap repo clone (`bootstrap.clone-dry-run`,
  `bootstrap.downloading`, `bootstrap.download-success`,
  `bootstrap.profile-now`).
- Errors rendering (`errors.prefix`,
  `errors.suggestion-prefix`).
- Provider help (`provider.help.*`).
- Git dispatcher (`git.usage.*`, `git.unknown_subcommand`).

Translations MUST exist in every `internal/i18n/locales/*.yaml`
file (currently `en.yaml` and `zh-CN.yaml`). New CLI commands MUST
add their user-facing strings to the catalog before merging.

#### Scenario: Adding a new CLI command requires catalog entries

WHEN a developer adds a new subcommand under `hams config` (e.g.,
`hams config edit`)
THEN every user-facing line the command prints SHALL go through
`i18n.T` / `i18n.Tf` with a typed constant declared in `keys.go`,
and corresponding entries SHALL be added to both `en.yaml` and
`zh-CN.yaml` before the change can pass CI.

#### Scenario: Hardcoded English strings fail review

WHEN a code review encounters `fmt.Println("…")` or
`fmt.Fprintln(os.Stderr, "…")` with English literal user-facing
text in `internal/cli/**` or `internal/provider/builtin/**`
THEN the review SHALL block the change and request the literal
be replaced with `i18n.T(<TypedKey>)` / `i18n.Tf(<TypedKey>,
data)`. Operator-only log records (`slog.Info`, `slog.Warn`)
remain in English by design — they are not user-facing.
