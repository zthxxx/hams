# 2026-04-16-package-provider-auto-record-gap

## Why

The 2026-04-16 verification cycle discovered that 7 of the 9 Package-class providers silently violate the core "auto-record" philosophy — they invoke the underlying package manager but do NOT append the installed resource to the hamsfile, meaning the install leaves no serialized record for later `hams apply` replay.

**Core philosophy (CLAUDE.md)**:

> **CLI-first, auto-record** — `hams brew install git` installs AND records. No hand-editing config first.

**CP-1 spec (`openspec/specs/builtin-providers/spec.md:41-71`)**:

> All Package-class providers (homebrew, apt, pnpm, npm, uv, goinstall, cargo, code-ext, mas) share these behaviors:
> **Apply**: For each entry in Hamsfile not present in state (or state = `failed`/`pending`), run the provider's install command. Write state on success.
> **CLI wrapping**: Provider recognizes `install`, `remove`, and `list` subcommands.

### Implementation survey (grep `AddApp` / `hf.Write` in `internal/provider/builtin/`)

| Provider | CLI install records hamsfile? | CLI install records state? | Tests present? |
|----------|------------------------------|----------------------------|----------------|
| **homebrew** | ✅ (handleInstall `hf.AddApp` + `hf.Write`) | ❌ | ❌ tests on CLI path |
| **apt** | ✅ (U1-U35 HandleCommand tests) | ✅ (U12-U15) | ✅ comprehensive |
| **cargo** | ❌ (returns after passthrough) | ❌ | ❌ |
| **npm** | ❌ | ❌ | ❌ |
| **pnpm** | ❌ | ❌ | ❌ |
| **uv** | ❌ | ❌ | ❌ |
| **goinstall** | ❌ | ❌ | ❌ |
| **mas** | ❌ | ❌ | ❌ |
| **vscodeext (code-ext)** | ❌ | ❌ | ❌ |

Only `apt` fully satisfies the CP-1 contract. `homebrew` records the hamsfile but not state. The remaining 7 do neither.

### User impact

- `hams cargo install ripgrep` installs the binary but does not persist the intent → `hams apply --from-repo=…` on a new machine re-installs nothing because the hamsfile is empty.
- Users assume the philosophy matches `hams apt install git`'s behavior — there is no warning that these 7 providers silently drop the record.
- The install mechanically succeeds, so exit code is 0, which breaks the user's mental model ("if it exited cleanly, hams should be able to restore it on another machine").

### Why this wasn't caught

- No HandleCommand-level tests for the 7 affected providers. The `*_lifecycle_test.go` files cover `Apply`/`Probe`/`Remove` via DI runner — that's the apply-from-hamsfile path, not the CLI-first path.
- Coverage reports show 60-70% for each provider, which looks healthy — but the uncovered branches are exactly the CLI-first auto-record lines that don't exist yet.

## What Changes

### Code changes (this change)

For each of the 7 providers — cargo, npm, pnpm, uv, goinstall, mas, vscodeext — apply the apt/homebrew pattern:

1. Add `cfg *config.Config` field to the `Provider` struct (cargo/npm/pnpm/uv/goinstall/mas/vscodeext currently have only `runner CmdRunner`).
2. Add `loadOrCreateHamsfile(hamsFlags, flags)` helper — one per provider, following `internal/provider/builtin/apt/hamsfile.go` exactly.
3. Update `handleInstall`: after successful passthrough, call `loadOrCreateHamsfile` → `hf.AddApp(<tag>, <pkg>, "")` per resource → `hf.Write()`.
4. Update `handleRemove`: after successful passthrough, call `loadOrCreateHamsfile` → `hf.RemoveApp(<pkg>)` per resource → `hf.Write()`.
5. Update `New()` signatures to accept `cfg *config.Config`. Update `internal/cli/register.go` to pass `builtinCfg` into each.
6. Add `TestHandleCommand_U*` tests matching apt's U1-U5 coverage: install-adds-to-hamsfile, install-is-idempotent, install-failure-leaves-hamsfile-untouched, remove-deletes-from-hamsfile, remove-failure-leaves-hamsfile-untouched, dry-run-does-nothing.

State-file writes were originally deferred to a separate change. In practice the hamsfile-only fix surfaced the follow-on bug immediately: `hams list --only=<provider>` reads the state file only, so right after a successful CLI install the list was empty until the user ran `hams refresh`. Cycles 96 (homebrew) and 202–208 (mas, cargo, npm, pnpm, uv, goinstall, vscodeext) therefore added the state-write half in the CLI handler too, matching apt's U12–U15 behavior. The spec delta and scenarios have been updated to describe both writes as part of the single auto-record contract.

### Spec delta (this change)

- `openspec/specs/builtin-providers/spec.md` — add a new Requirement under CP-1 mandating that every Package-class provider's CLI `install` and `remove` verbs MUST auto-record the hamsfile after a successful passthrough. Scenarios enumerate: install-records, install-failure-does-not-record, remove-unrecords, dry-run-does-not-record.

### Docs updates (this change)

- None. The core philosophy already documents auto-record; we are catching up the implementation to match documented behavior.

## Impact

- **Affected packages**: `internal/provider/builtin/{cargo,npm,pnpm,uv,goinstall,mas,vscodeext}`.
- **Affected CLI entry points**: `internal/cli/register.go` (constructor signatures).
- **Risk**: Low. The CP-1 pattern is well-established in `apt`/`homebrew`. Each fix is bounded to one provider package.
- **Migration**: None — existing hamsfiles remain valid; new ones will be populated automatically.

## Implementation sequencing

One provider per atomic commit + cycle. Pilot with `cargo` (smallest surface), then `npm` → `pnpm` → `uv` → `goinstall` → `mas` → `vscodeext`. Close with a final integration-test pass.
