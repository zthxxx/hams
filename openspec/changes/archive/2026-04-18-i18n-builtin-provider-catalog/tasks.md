# Tasks: Builtin-Provider i18n Catalogue

## 1. Extend the catalogue

- [x] 1.1 Add shared parameterised keys to `internal/i18n/keys.go`: `ProviderErrRequiresResource`, `ProviderErrRequiresAtLeastOne`, `ProviderUsageBasic`, `ProviderDryRunWouldRun`, `ProviderDryRunWouldInstall`, `ProviderDryRunWouldRemove`.
- [x] 1.2 Add provider-specific keys (apt simulate warning, git clone/config usage errors, homebrew list outputs, etc.) as each wiring step uncovers a message that doesn't fit the shared template.
- [x] 1.3 Mirror every new key into `internal/i18n/locales/en.yaml` (canonical) and `internal/i18n/locales/zh-CN.yaml` (complete Chinese translation).

## 2. Wire providers

- [x] 2.1 Wire `apt` — 5 `NewUserError` sites + dry-run preview.
- [x] 2.2 Wire `bash` — 2 sites.
- [x] 2.3 Wire `homebrew` — 11 sites + dry-run lines + tap/list prose.
- [x] 2.4 Wire `cargo` — 5 sites.
- [x] 2.5 Wire `goinstall` — 3 sites.
- [x] 2.6 Wire `mas` — 5 sites.
- [x] 2.7 Wire `npm` — 5 sites.
- [x] 2.8 Wire `pnpm` — 5 sites.
- [x] 2.9 Wire `uv` — 5 sites.
- [x] 2.10 Wire `vscodeext` (exposed as `code`) — 5 sites.
- [x] 2.11 Wire `duti` — 3 sites.
- [x] 2.12 Wire `defaults` — 4 sites.
- [x] 2.13 Wire `ansible` — 4 sites.
- [x] 2.14 Wire `git` — unified dispatcher + git/config + git/clone internals all routed through i18n. `rg 'NewUserError' internal/provider/builtin/git/ -g '!*_test.go'` shows every site followed by an `i18n.T` / `i18n.Tf` call; no literal English remains.
- [x] 2.14.a `git/clone.go` + `git/git.go` internal NewUserError + dry-run sites wired in a follow-up commit. Catalog gained ~30 new keys under `provider.git.config.*` and `provider.git.clone.*` covering set/remove usage errors, the multi-line usage bundles, the "target not a git repo" explanation + both hints, the exact-one-remote guard, the "no tracked resource" error, and `--hams-path` validation.

## 3. Test coverage

- [x] 3.1 Add `internal/i18n/i18n_providers_test.go` with `TestProviderKeysResolveInEnglish` / `TestProviderKeysResolveInChinese` iterating every exported `Provider*` constant and asserting non-empty, non-key-literal resolution.
- [x] 3.2 Update any provider unit tests that assert specific English error text to use the i18n-resolved form.

## 4. Verification

- [x] 4.1 `rg 'NewUserError\(hamserr\.' internal/provider/builtin/ | rg -v 'i18n\.'` returns zero lines — every NewUserError site routes through i18n.
- [x] 4.2 `go build ./...` passes.
- [x] 4.3 `task lint` passes.
- [x] 4.4 `task test` passes (unit + integration, race detector on).
- [x] 4.5 Manual: run `LANG=zh_CN.UTF-8 ./bin/hams apt install` with zero args and observe the Chinese rendering; same for 2 other providers (spot check).
- [x] 4.6 `openspec validate i18n-builtin-provider-catalog` passes.
- [x] 4.7 Archive the change.
