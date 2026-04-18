# Proposal: docs sync to `dev` state after the 2026-04-18 feature batch

## Why

Seven changes shipped on 2026-04-18 — `ci-act-opt-in`, `code-provider-full-rename`,
`tag-profile-conflict-detection`, `typed-i18n-keys`, `auto-init-ux-hardening`,
`integration-log-assertion-fanout`, `git-passthrough-and-spec`. Each updated code,
tests, and (where relevant) its own delta spec. Docs were NOT swept as part of the
individual changes, so:

- `docs/content/{en,zh-CN}/**` references exist for `--profile` as-canonical, for the
  auto-init flow without the new dry-run / ctx-timeout / identity-seeding semantics,
  and for `hams git <verb>` without the new passthrough guarantee.
- `README.md` / `README.zh-CN.md` user-facing summaries mostly already landed during
  `code-provider-full-rename`, but the `--tag / --profile` conflict guarantee + the
  git passthrough note are missing.
- `openspec/specs/**` were already reconciled during each individual change's own
  delta (every 2026-04-18 change wrote spec deltas). The sweep re-validates that.

## What changes

Documentation only. No code, no CLI behavior, no schema. Sweep targets:

1. **`--tag` as canonical, `--profile` as legacy alias, conflict-detection**
   - `docs/content/{en,zh-CN}/docs/cli/index.mdx` — replace `--profile=<tag>` row with
     `--tag=<name>` + "`--profile` alias" note.
   - `docs/content/{en,zh-CN}/docs/cli/apply.mdx` — add a line documenting the conflict
     failure: `hams apply --tag=macOS --profile=linux` → exit code 2 (USAGE).
   - `docs/content/{en,zh-CN}/docs/cli/global-flags.mdx` — same conflict note under the
     `--tag` row.

2. **Auto-init flow — dry-run honored, ctx timeout, identity seeded**
   - `docs/content/{en,zh-CN}/docs/cli/global-flags.mdx` — extend the auto-init section:
     "honors `--dry-run` (preview, zero writes)", "30s timeout on the bundled git init",
     "seeds `profile_tag: default` + hostname-derived `machine_id` when absent".
   - `docs/content/{en,zh-CN}/docs/quickstart.mdx` — mirror the "seeds `profile_tag` +
     `machine_id`" note in the first-run narrative.

3. **`hams git` passthrough**
   - `docs/content/{en,zh-CN}/docs/providers/git.mdx` — add "### Passthrough for unmanaged
     subcommands" subsection with an example `hams git log --oneline` call.
   - `README.md` / `README.zh-CN.md` — one-liner in the features list that mirrors the
     passthrough contract.

4. **`openspec/specs/**` grep sanity check**
   - The 2026-04-18 changes already wrote their own `openspec/changes/*/specs/` deltas.
     No spec files under `openspec/specs/` need additional edits for this sweep — the
     deltas merge on archive. This proposal carries ZERO spec deltas of its own.

## Impact

- User-visible behavior: **no change**. This is documentation reconciliation only.
- `task fmt` + `bun markdownlint-cli2` pass.
- `task check` unchanged (docs changes do not affect Go / lint / test).
- Archival stays clean because no `openspec/specs/**` files move.
