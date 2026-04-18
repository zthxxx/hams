# Proposal: `hams git` passthrough for unhandled subcommands

## Why

CLAUDE.md §Current Tasks states: *"Provider wrapped commands MUST behave exactly like the original command, at least at the first-level command entry point."* Before this change, dev's `hams git <unknown-subcommand>` surfaced a usage error — `hams git log`, `hams git pull`, `hams git status` all failed with "unknown subcommand". A user who aliases `git=hams git` to get auto-record for free found half their muscle memory broken.

Additionally, the natural `hams git clone <remote> <path>` form did NOT fold the positional `<path>` into the CloneProvider's internal `--hams-path` DSL, so the CLI grammar that mirrors real git did not work end-to-end for cloning.

## What changes

1. `internal/provider/builtin/git/unified.go`:
   - `HandleCommand`'s `default:` branch now invokes a new `passthrough(ctx, args, flags)` helper instead of surfacing a UFE. The passthrough runs the real `git` binary with stdin/stdout/stderr + exit code preserved.
   - `handleClone` translates `hams git clone <url> <path>` into `{"add", url}` plus `hamsFlags["path"] = path` so the natural git grammar records correctly.
   - Unforwarded git flags (`--depth`, `--branch`, `--recurse-submodules`, …) are rejected with an actionable UFE — silently dropping them could cause surprises (e.g., a `--depth=1` CI clone silently becoming a full clone).
   - `flags.DryRun` on the passthrough prints `[dry-run] Would run: git <args>` and returns without exec.
   - New `passthroughExec` package-level var is the DI seam for unit tests.

2. `internal/provider/builtin/git/unified_test.go`:
   - Replaces the old `TestUnifiedHandler_RejectsUnknownSubcommand` (behavior changed) with:
     - `TestUnifiedHandler_PassesUnknownSubcommandThroughToGit` — passthrough invokes the exec seam with the exact args.
     - `TestUnifiedHandler_PassthroughPropagatesExecError` — non-zero exit bubbles up.
     - `TestUnifiedHandler_PassthroughDryRunSkipsExec` — dry-run prints preview, skips exec, captures stdout via the new `flags.Out` seam.
     - `TestUnifiedHandler_CloneNaturalFormTranslatesPath` — positional path folds into hamsFlags.
     - `TestUnifiedHandler_CloneRejectsUnknownGitFlag` — `--depth=1` rejected with UFE.
     - `TestUnifiedHandler_CloneRequiresRemote` — bare `hams git clone` errors.

## Impact

- **Capability `provider-system`** — adds the "Passthrough for Unhandled Subcommands" requirement that was previously implicit in CLAUDE.md but missing from the spec.
- **Capability `builtin-providers`** — the VS Code extensions provider already inherits the pattern (see `vscodeext.Provider.HandleCommand` which delegates unknown verbs via `provider.WrapExecPassthrough`), so the `git` unified handler's new passthrough behavior is consistent with the rest of the registry.
- **User-visible:**
  - `hams git pull origin main` runs real git.
  - `hams git log --oneline` runs real git.
  - `hams git clone https://example.com/repo.git /tmp/repo` records the clone under `git-clone.hams.yaml`.
  - `hams git clone https://example.com/repo.git /tmp/repo --depth=1` fails with an actionable error naming the flag.
- **Back-compat:** the legacy `--hams-path=<path>` form continues to work. `hams git config ...` and `hams git clone remove/list` routes are unchanged.
