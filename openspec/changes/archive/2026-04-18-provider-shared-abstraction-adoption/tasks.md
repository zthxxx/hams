# Tasks: Shared Package-Dispatcher Adoption

## 1. Dispatcher i18n hardening

- [x] 1.1 Route `AutoRecordInstall`'s "requires at least one package" error through `provider.UsageRequiresAtLeastOne` (i18n-wired).
- [x] 1.2 Route `AutoRecordRemove`'s "requires at least one package" error through `provider.UsageRequiresAtLeastOne`.
- [x] 1.3 Route the dry-run preview through `provider.DryRunInstall` / `provider.DryRunRemove` for install and remove respectively.
- [x] 1.4 Drop the now-unused `hamserr` import.

## 2. Cargo reference migration

- [x] 2.1 Rewrite `cargo.handleInstall` to delegate to `provider.AutoRecordInstall`.
- [x] 2.2 Rewrite `cargo.handleRemove` to delegate to `provider.AutoRecordRemove`.
- [x] 2.3 Remove unused helpers (`loadOrCreateHamsfile`, `loadOrCreateStateFile`).
- [x] 2.4 Prune now-unused imports.

## 3. Spec delta

- [x] 3.1 Document the SHALL in `provider-system/spec.md` (via delta file).
- [x] 3.2 Document the exemption process for providers whose runner signatures don't match.

## 4. Verification

- [x] 4.1 `go test -race ./internal/provider/builtin/cargo/...` passes.
- [x] 4.2 `task lint` passes.
- [x] 4.3 Provider-system spec delta validates via `openspec validate`.
- [x] 4.4 Archive the change.

## 5. Follow-up (tracked, not in this change)

- [x] 5.1 Migrate `npm` onto `AutoRecordInstall` / `AutoRecordRemove`.
- [x] 5.2 Migrate `pnpm`.
- [x] 5.3 Migrate `uv`.
- [ ] 5.4 Migrate `goinstall` — blocked: runner has no `Uninstall` method (go install has no uninstall verb). Needs either a no-op `Uninstall` on the runner or a dispatcher variant that handles install-only providers. Tracked for a future change.
- [x] 5.5 Migrate `mas`.
- [x] 5.6 Migrate `vscodeext`.
- [ ] 5.7 Design a second dispatcher variant for the batch-install shape (apt).
- [ ] 5.8 Design a third dispatcher variant for the extra-arg shape (brew's `isCask`).
