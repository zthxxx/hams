# 2026-04-18-apply-tag-and-auto-init

## Why

A user sitting down at a fresh machine with a store repo should be
able to restore their whole environment with ONE command, without
touching `~/.config/hams/hams.config.yaml` first:

```bash
curl … | bash     # install hams
hams apply --from-repo=zthxxx/hams-store --tag=macOS
```

Today this partially works — `--from-repo` clones correctly — but the
flow is rough on two points that the Current Tasks list in CLAUDE.md
flags explicitly:

1. There is no `--tag` flag on `hams apply`. To pick a profile the
   user must type `--profile=macOS`. "Profile" is a correct noun
   for an isolated environment, but "tag" is the shorter, more
   natural English word (and the YAML config key is `profile_tag`,
   so "tag" is already the second half of that compound noun).
   The CLI ergonomics ask favors `--tag`.

2. If `~/.config/hams/hams.config.yaml` does not exist, `hams apply`
   without a TTY errors out with a spec-correct but unhelpful
   "profile_tag and machine_id not configured" message (see
   `ensureProfileConfigured` at internal/cli/apply.go:1429). The
   right behavior on a fresh non-interactive machine (cloud-init,
   dotfiles script, Dockerfile RUN line) is to auto-create the
   config from whatever the user supplied on the command line
   (`--tag=<t>`) plus a sane default `machine_id` (hostname), then
   continue.

These two gaps are small individually but the user-workflow cost is
large: today you cannot truly one-shot a fresh machine without an
interactive prompt or a pre-seeded config.

## What Changes

Two user-facing surface changes, one internal cleanup:

### 1. `--tag=<t>` global flag

- Add `--tag` to the global flag list (alongside `--debug`, `--json`,
  `--profile`, etc.). It is a first-class global, usable on every
  subcommand — not just `hams apply` — so that `hams --tag=macOS
  list` and `hams --tag=macOS brew install htop` also respect it.
- `--tag` and `--profile` are aliases. Supplying both with
  different values is a usage error: `--tag and --profile are
  aliases; pass only one`.
- Resolution precedence for the active tag (highest wins):
  1. `--tag=<t>` / `--profile=<t>` on the command line.
  2. `profile_tag:` in `${HAMS_CONFIG_HOME}/hams.config.yaml`
     (and `.local.yaml` overlay, merged per existing rules).
  3. Hardcoded default `"default"` (matches
     `config.sanitizePathSegment`'s existing fallback).

No spec changes to the store-layout; the resolved tag still picks
the `<store>/<tag>/` directory on disk.

### 2. Auto-init config on `hams apply`

- When `hams apply` runs and `${HAMS_CONFIG_HOME}/hams.config.yaml`
  does not exist, hams SHALL create it with whatever it can infer:
  - `profile_tag`: from `--tag` / `--profile` if given; else the
    interactive prompt (TTY); else the literal `"default"`.
  - `machine_id`: from `$HAMS_MACHINE_ID` env var if set; else
    `hostname -s` (sanitized via `sanitizePathSegment`); else the
    interactive prompt (TTY); else the literal `"default"`.
  - `store_repo` and `store_path`: unchanged — `--from-repo=<x>`
    persists as `store_repo` on the first successful clone so the
    next `hams apply` works with no args.
- Auto-init fires EXACTLY ONCE on first apply. Subsequent apply runs
  on the same host read the persisted config and skip the
  auto-init path entirely.
- Auto-init is lossless: if `--tag=macOS` is supplied but
  `profile_tag:` was already present in a pre-existing config with
  a different value, the CLI flag wins for this invocation but
  does NOT rewrite the persisted config. (Avoids clobbering a
  user's carefully maintained config from a one-off CI run.)

### 3. Internal cleanup

- Extract tag/profile resolution into `config.ResolveActiveTag(cfg,
  flags)` so every CLI command (`apply`, `refresh`, `list`,
  `config`, per-provider `handleList`) reaches for the same logic
  instead of re-implementing the overlay.
- `config.Load` keeps its current `profileTag` parameter but
  callers now populate it from `ResolveActiveTag` instead of the
  raw `flags.Profile` field. Same behavior, different call path.

## User Workflow — After This Change

### One-shot restore on a fresh machine

```bash
hams apply --from-repo=zthxxx/hams-store --tag=macOS
# → clones the store under ${HAMS_DATA_HOME}/repo/zthxxx/hams-store/
# → writes ~/.config/hams/hams.config.yaml with:
#     profile_tag: macOS
#     machine_id: <hostname>
#     store_repo: zthxxx/hams-store
# → runs the full apply (install/remove/update per declared state)
```

### Day-2: apply is a no-arg command

```bash
hams apply          # reads persisted config, applies everything
```

### Ad-hoc profile switch

```bash
hams --tag=linux apply          # one-off; does NOT rewrite config
hams --profile=linux apply       # equivalent (alias)
```

## Out of Scope

- Per-provider `--tag` (hamsfile inline tags like `cli:`,
  `development-tool:`) — that is a hamsfile-level concept, not a
  CLI flag concept. No conflation intended.
- Renaming the config key `profile_tag` → `tag`. The field name
  stays as-is to avoid a schema migration; only the CLI flag is
  shortened.
- Removing the `--profile` flag. It stays as a forever-alias of
  `--tag`; users who scripted `--profile=X` keep working.

## Verification

- Unit tests in `internal/config/` cover `ResolveActiveTag` for all
  four precedence levels and the `--tag` + `--profile` ambiguity
  error.
- Unit tests in `internal/cli/` cover `hams apply` auto-init with
  and without a TTY, with and without `--tag`, and drive a fake
  hostname resolver so the machine_id defaulting is deterministic.
- An integration test under `e2e/integration/` runs
  `hams apply --from-repo=<fixture-repo> --tag=<t>` against a
  pre-committed local test store; asserts that the config file is
  materialized and the subsequent `hams apply` (no args) reads it.
