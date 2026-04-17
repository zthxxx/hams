# 2026-04-18-provider-autoscaffold-store

## Why

The day-one workflow for a new hams user is:

```bash
hams brew install htop
```

They want that one command to install htop AND record it into a
git-tracked store so it can be replayed on the next machine. Today
this fails with "no store directory configured" because the store
hasn't been scaffolded yet — the user has to run `hams store init`
first, or edit `~/.config/hams/hams.config.yaml` by hand to set
`store_path`.

That friction kills the pitch. CLAUDE.md's Current Tasks makes this
explicit: "When running `hams <provider> ...`, if no hams config
file repo exists yet, auto-create one at the default location,
pre-initialized with `git init` (via command invocation) and a
`.gitignore` (from a template file), then write the hams config file
(create if missing)."

## What Changes

### 1. Auto-scaffold hook in routeToProvider

Before dispatching to the provider handler, `routeToProvider` calls
a new `EnsureStoreScaffolded(ctx, paths, flags)` helper that:

1. Resolves the target store path via precedence:
   - `flags.Store` (explicit --store flag).
   - `cfg.StorePath` from the currently-loaded config (if any).
   - `$HAMS_STORE` env var.
   - Default: `${HAMS_DATA_HOME}/store` (so the scaffolded store
     lives alongside logs and OTel traces — users who want it
     elsewhere set `$HAMS_STORE` or `--store` up front).
2. Creates the directory if missing.
3. Runs `git init <store-path>` via an injectable exec seam if
   `.git` is absent. The store is meant to be git-tracked; requiring
   the user to `git init` after the fact would defeat the
   auto-scaffold's pitch.
4. Writes `<store>/.gitignore` from the embedded template if
   missing. Template lives at
   `internal/cli/template/store/gitignore` and is bundled via
   `go:embed template/store`. File is named `gitignore` (not
   `.gitignore`) so Go's embed picks it up — dotfiles are excluded
   by default. Scaffolder renames on write.
5. Writes `<store>/hams.config.yaml` from the embedded template if
   missing. Minimal comment-only seed, matching the `hams store
   init` shape.
6. Persists `store_path: <resolved-path>` into the global config
   so subsequent invocations find it without re-scaffolding.

All steps are idempotent: scaffolding twice leaves the user's
hand-edited `.gitignore` / config untouched. That is the "mechanical
undo guarantee" — running the same command twice produces the same
state, so a user inspecting the scaffold to verify what hams did
can safely re-run it.

### 2. Embedded templates

Introduce `internal/cli/template/store/` with two files:

- `gitignore` (renamed to `.gitignore` at write time).
- `hams.config.yaml` with a commented header pointing at the schema
  spec.

Templates are embedded via `//go:embed template/store` so anyone
editing the scaffolded content touches a real file — no code change
needed. Mirrors the "bundle its structure + file contents into the
binary for ease of development and maintenance" requirement.

### 3. `--hams-no-scaffold` escape hatch

Users (or tests) that want the pre-change "fail loud when no store"
behavior can pass `--hams-no-scaffold`. The flag is documented in
the provider help output and consumed by the same `--hams-` prefix
path as the other hams flags. No spec change to the provider
interface — scaffolding is orthogonal to the provider's HandleCommand.

### 4. --tag / --profile collapse

Collapse `flags.Tag` and `flags.Profile` into a single effective
value before `handler.HandleCommand` runs, with the
`ResolveCLITagOverride` helper from the sibling change
(`2026-04-18-apply-tag-and-auto-init`). Every provider's existing
`effectiveConfig` overlay reads `flags.Profile` only, so aliasing
here means providers pick up `--tag=macOS` without touching 15
individual implementations. The collapse also fails fast on the
ambiguity case (`--tag=macOS --profile=linux`) with a clear usage
error.

## User Workflow — After This Change

### First provider command on a pristine machine

```bash
hams brew install htop
# INFO initialized store git repo path=/home/user/.local/share/hams/store
# INFO scaffolded store file  path=/home/user/.local/share/hams/store/.gitignore
# INFO scaffolded store file  path=/home/user/.local/share/hams/store/hams.config.yaml
# INFO brew install package=htop cask=false
# … hams records htop in <store>/default/Homebrew.hams.yaml …
```

The user's next command (`hams pnpm install -g serve`, `hams apply`,
whatever) reuses the same scaffolded store — second scaffold call
is a no-op.

### Opting out

```bash
hams --hams-no-scaffold brew install htop
# ERROR: no store directory configured (pre-change behavior preserved)
```

## Out of Scope

- Auto-scaffolding at `hams apply` time. Apply's --from-repo flow
  already handles the clone-a-remote-store path; `hams apply`
  without any hints on a pristine machine is not a supported
  workflow. The Current Tasks list specifically asks for the
  provider-invocation scaffold.
- Prompting for `profile_tag` during scaffold. `hams
  <provider>` commands don't take a `--tag` unless explicit;
  undefined tag falls through to `"default"` via sanitizePathSegment
  (existing behavior).
- Populating `machine_id` in the store-level config. `machine_id`
  is machine-scoped and MUST live in the global config, which
  `ensureProfileConfigured` seeds separately (see sibling change).

## Verification

- Unit tests in `internal/cli/scaffold_test.go`:
  - `TestEnsureStoreScaffolded_CreatesDirAndTemplates` — pristine
    paths + empty flags produce a correctly-scaffolded store.
  - `TestEnsureStoreScaffolded_Idempotent` — second call does NOT
    clobber user edits to `.gitignore`; `git init` runs exactly
    once.
  - `TestEnsureStoreScaffolded_RespectsHamsStoreEnv` — env override
    beats the default `$HAMS_DATA_HOME/store` placement.
- `go build ./...` and `go test -race ./...` pass green.
- Manual smoke in the dev sandbox: `task dev EXAMPLE=basic-debian`,
  inside the container run `hams brew install htop` (with brew
  available) — see the scaffold fire and htop recorded.
