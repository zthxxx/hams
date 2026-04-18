# Tasks: Fan-out i18n coverage to every user-facing string

## 1. Catalog groundwork

- [x] Add `provider.err.*` keys for shared "no store configured" + "install requires a package name" style messages that every provider duplicates.
- [x] Add `apply.*` keys for apply-specific status lines (execution order, summary, dry-run per-provider preview, etc.).
- [x] Add `refresh.*` keys for refresh-specific status lines.
- [x] Add `store.*` / `config.*` keys for `hams store status` and `hams config get/set/unset` outputs.
- [x] Add `version.*` and `upgrade.*` keys for `hams version` and `hams self-upgrade` outputs.
- [x] Add `cli.err.*` keys for framework-level usage errors (mutually-exclusive flags, positional-args-not-allowed, etc.).
- [x] Add `git.*` / `git-clone.*` keys for dry-run lines and list headers not already covered.
- [x] Update `internal/i18n/locales/en.yaml` with every new key's English text.
- [x] Update `internal/i18n/locales/zh-CN.yaml` with every new key's Chinese translation.
- [x] Update `internal/i18n/keys.go` with exported `const` declarations for every new key.
- [x] Extend `TestCatalogCoherence_EveryTypedKeyResolves` to include every new constant.

## 2. Refactor user-facing call-sites

- [x] `internal/config/config.go` — wire explicit-config-missing error through i18n.
- [x] `internal/provider/baseprovider/baseprovider.go` — route shared "no store directory configured" error through i18n.
- [x] `internal/provider/builtin/<provider>/hamsfile.go` — route each provider's no-store error through `provider.err.no-store`. Providers: apt, bash, cargo, defaults, duti, git, goinstall, mas, npm, pnpm, uv, vscodeext.
- [x] `internal/provider/builtin/<provider>/<provider>.go` — route "install/remove requires a package name" primary messages through i18n. Providers: apt, cargo, duti, goinstall, mas, npm, pnpm, uv, vscodeext, homebrew.
- [x] `internal/provider/builtin/<provider>/<provider>.go` — route dry-run echo lines through i18n (`provider.status.dry-run-install` + `provider.status.dry-run-remove`). Scope to `[dry-run] Would install:` / `[dry-run] Would remove:` style lines, skip bespoke lines.
- [x] `internal/cli/apply.go` — route all `fmt.Println`/`fmt.Printf` user-facing prose + NewUserError primary messages through i18n (dry-run header, no-providers-match variants, summary, warnings, exit messages, profile-init guidance).
- [x] `internal/cli/commands.go` — route `refresh` summary lines, `config` output lines, `store` status/init lines, and all NewUserError primary messages through i18n.
- [x] `internal/cli/provider_cmd.go` — route `showProviderHelp` lines through i18n.
- [x] `internal/cli/errors.go` — route `Error:` prefix and `suggestion:` prefix through i18n.
- [x] `internal/cli/bootstrap.go` — route download/cloning status lines through i18n.
- [x] `internal/provider/cli_list.go` / `cli_lock.go` — route NewUserError primary messages through i18n.
- [x] `internal/sudo/sudo.go` — route "hams needs sudo" prompt through i18n.
- [x] `internal/tui/tui.go` — route `WarnNoTTY` through i18n.
- [x] `internal/provider/builtin/git/unified.go` — route dry-run "Would run: git ..." through i18n.
- [x] `internal/provider/builtin/git/git.go` / `git/clone.go` — route "git config/clone dry-run" lines + "managed repositories" listing header through i18n.
- [x] `internal/provider/builtin/homebrew/homebrew.go` — route `[dry-run] Would run: brew untap` / `brew tap` prose + "Homebrew managed packages:" header.
- [x] `internal/provider/builtin/bash/bash.go` — route NewUserError primary messages through i18n.
- [x] `internal/provider/builtin/ansible/ansible.go` — route dry-run + NewUserError primary messages through i18n.
- [x] `internal/provider/builtin/defaults/defaults.go` — route dry-run + NewUserError primary messages through i18n.

## 3. Verification

- [x] `task fmt` — zero diff.
- [x] `task lint` — pass.
- [x] `task test:unit` — pass (including `TestCatalogCoherence_EveryTypedKeyResolves`).
- [x] Manual smoke — `LANG=zh_CN.UTF-8 ./bin/hams apply --dry-run` shows Chinese prose where keys exist; English where deferred.
- [x] Count check — total i18n-covered call-sites ≥ 60 (up from 13), total typed keys ≥ 60 (up from 19).

## 4. Follow-up / deferred

Call-sites left with `// TODO(i18n):` markers, typically because the string embeds multi-line shell examples or complex structured text. These represent tech-debt that can land in a future cycle:

- Profile-init non-TTY multi-line guidance (`internal/cli/apply.go::ensureProfileConfigured`) — the three-line example block with `hams config set ...` literals is best kept as verbatim shell.
- `--from-repo` vs `--store` mutual-exclusion detail suggestions — multi-clause prose with embedded path syntax.
- Apply bootstrap-consent prompt flow (`internal/cli/bootstrap_consent.go`) — deferred because the prompt is itself a structured TTY interaction; covered by a follow-up task.
