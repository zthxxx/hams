# Tasks — 2026-04-18-provider-shared-abstractions

## 1. Helper file

- [x] 1.1 Create `internal/provider/package_dispatcher.go`.
- [x] 1.2 Define `PackageInstaller` interface
      (`Install(ctx, pkg) / Uninstall(ctx, pkg) error`) as the
      narrow contract the helpers need from a provider's CmdRunner.
- [x] 1.3 Define `PackageDispatchOpts` struct carrying `CLIName`,
      `InstallVerb`, `RemoveVerb`, `HamsTag` — everything the
      helpers need to format error messages, lock labels, and
      hamsfile records.
- [x] 1.4 Implement `AutoRecordInstall(ctx, runner, pkgs, cfg,
      flags, hfPath, statePath, opts)` — five-step flow (validate
      / dry-run / lock / exec / write).
- [x] 1.5 Implement `AutoRecordRemove` mirroring the install flow
      for the uninstall side.

## 2. Documentation

- [x] 2.1 File-level comment in `package_dispatcher.go` walks
      through the canonical flow step-by-step and names what the
      caller still owns (arg extraction, complex-invocation
      detection, post-install probes).
- [x] 2.2 Proposal document in
      `openspec/changes/2026-04-18-provider-shared-abstractions/proposal.md`
      records the design rationale, rejected alternatives, and the
      incremental migration path.

## 3. Migration pilot (deferred to a follow-up change)

- [ ] 3.1 Cargo pilot: swap
      `internal/provider/builtin/cargo/cargo.go` handleInstall /
      handleRemove to call the helpers. Existing U1-U7 tests keep
      passing (pattern-preserving).
- [ ] 3.2 npm / pnpm / uv / goinstall / mas follow-up migrations,
      one commit each.
- [ ] 3.3 vscodeext follow-up; needs the version-pin-strip hook
      but otherwise matches.
- [ ] 3.4 Document why brew + apt remain inline (tap detection +
      --cask branches; --simulate detection + post-install version
      probe) inside each provider's file-level comment.

## 4. Verification

- [x] 4.1 `go build ./...` passes (helper compiles).
- [x] 4.2 `go test -race ./...` passes (existing providers
      untouched, zero behavior change).
- [x] 4.3 `task check` passes green through lint + unit +
      integration (e2e via act fails on artifact upload — known
      act limitation, unrelated to this change).
