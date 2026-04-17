# 2026-04-17-onboarding-auto-init

## Why

A first-time user trying hams today hits two paper cuts that the
"CLI-first, auto-record" core philosophy implicitly promises away:

### Friction 1: `hams brew install jq` on a clean machine errors out

Running any provider before `hams store init` fails with:

```text
Error: no store directory configured
  → Set store_path in ~/.config/hams/hams.config.yaml
  → Or initialize one: hams store init
```

This contradicts the philosophy in `CLAUDE.md`:

> **CLI-first, auto-record** — `hams brew install git` installs AND records.
> No hand-editing config first.

To "not hand-edit config first" the binary must accept the bare provider
verb on a clean machine. Today it does not.

### Friction 2: `hams apply` on a fresh machine has no single-command form

The closest path to one-liner restoration is:

```bash
hams apply --from-repo=user/hams-store --profile=macOS
```

Three problems:

1. The flag is `--profile`. The natural English word is `--tag`
   (matching the schema's "profile-as-directory" terminology and the
   CLAUDE.md task-list wording: `default tag: default`).
2. If no `~/.config/hams/hams.config.yaml` exists, `hams apply` errors
   out before it ever gets to the `--from-repo` path because earlier
   guards expect a config file to be readable.
3. There is no built-in default tag — users on machines with no
   per-machine `profile_tag` get prompted interactively or hard-fail
   in non-TTY environments.

### Consolidated user impact

A new contributor cloning `hams-store.git` and running
`curl ... | bash && hams apply --from-repo=mine --tag=macOS` on a brand
new VM expects ONE command to bring the machine up. Today they need:

```bash
hams store init                                     # Pre-create store
hams config set profile_tag macOS                   # Pre-set profile
hams config set machine_id $(hostname)              # Pre-set machine
hams apply --from-repo=user/hams-store              # Finally restore
```

Four commands instead of one. This change closes both gaps.

## What Changes

### Code changes

#### CLI surface

1. **Add `--tag <name>` flag to `hams apply` and `hams refresh`.**
   Aliases `--profile` for back-compat. Precedence chain:
   `--tag` > config `tag:` > config `profile_tag:` > `"default"`.

2. **Auto-create the global config** at `${HAMS_CONFIG_HOME}/hams.config.yaml`
   when missing on first invocation of any command that needs it.
   Initial contents are sourced from a `//go:embed`-bundled template
   (see "Bundled template" below). No interactive prompt — the auto-
   created file uses `tag: default` and `machine_id: <hostname>`.

3. **Auto-create the default store** at `${HAMS_DATA_HOME}/store/` when
   `cfg.StorePath == ""` after config load AND no `--store` /
   `--from-repo` was passed. Auto-created stores are pre-initialized
   via:
   - `git init` (real `git` binary, falls back to in-process go-git
     when `git` is missing from PATH so the path also works in
     fresh-machine container scenarios).
   - Write `.gitignore` from bundled template.
   - Write `hams.config.yaml` from bundled template.
   - Create `default/` profile directory.
   - Update the global config's `store_path` to the auto-created
     location so subsequent runs find it.

4. **Default `tag` becomes `"default"`** when neither `--tag`, the
   config-file `tag:` field, nor the legacy `profile_tag:` field is
   set. Removes the interactive `promptProfileInit` fallback for the
   common path; the prompt is reserved for `hams store init` and is
   gated behind explicit user opt-in.

#### Schema additions

- `Config.Tag` (alias for `ProfileTag`): YAML key `tag`. Reading
  honors both `tag:` and the legacy `profile_tag:` (last-wins on
  collision). Writing always emits `tag:` going forward.

#### Bundled templates

- `internal/storeinit/template/.gitignore` — embedded via
  `//go:embed`. Contents: `.state/\n*.local.*\n`.
- `internal/storeinit/template/hams.config.yaml` — embedded.
  Contents: `# hams store project-level config\n# Per-machine
  fields belong in ~/.config/hams/hams.config.yaml, not here.\n`.
- A new package `internal/storeinit/` exposes `Bootstrap(path string)
  error` that walks the embedded template and writes each file
  idempotently.

### Spec deltas

- `cli-architecture/spec.md` — new requirements:
  - "Apply accepts --tag flag with documented precedence chain"
  - "First-run auto-creates global config when missing"
  - "First-run auto-initializes default store when cfg.StorePath empty"
- `schema-design/spec.md` — new requirements:
  - "Config.Tag is the canonical YAML key; profile_tag is a recognized alias"
  - "Default store location is ${HAMS_DATA_HOME}/store/"
  - "Auto-init writes embedded template + .gitignore + git init"
- `builtin-providers/spec.md` — new requirement:
  - "Provider CLI invocations auto-init the default store when none is configured"

### Docs updates

- `docs/content/en/docs/quickstart.mdx` — replace the four-command
  bootstrap with one-command form.
- `docs/content/en/docs/cli/apply.mdx` — document `--tag`.
- `docs/content/zh-CN/...` — mirror.

## Impact

- **Affected packages**: `internal/cli/` (apply, refresh, root, commands),
  `internal/config/` (Tag alias + Default tag fallback), new
  `internal/storeinit/`, `internal/provider/builtin/` (every provider's
  `effectiveConfig` adds an auto-init fall-through).
- **Affected tests**: every test that asserts the "no store directory
  configured" error needs a new fixture (set `HAMS_CONFIG_HOME` to a
  read-only path or pre-create a config to keep the negative case
  meaningful).
- **Risk**: medium. Auto-init has filesystem side effects on first
  run; tests must use `t.TempDir()`-overridden `HAMS_CONFIG_HOME` /
  `HAMS_DATA_HOME` to avoid touching the developer machine.
- **Migration**: existing users with a configured `profile_tag` keep
  working unchanged. New users get the one-command flow.

## Implementation sequencing

1. New `internal/storeinit/` package with embedded templates + tests.
2. Config layer: add `Tag` field, alias parsing, default fallback.
3. CLI: add `--tag` flag wiring + alias for `--profile` on apply/refresh.
4. Auto-init wiring: shared helper called from apply's pre-store-load
   guard and from every provider's `loadOrCreateHamsfile` path.
5. Update existing tests that asserted the "no store" error path.
6. Add new tests for one-command flow + override flow.
7. Docs sync.
8. Integration test: real `hams brew install jq` inside a Docker
   container starting from `$HOME` empty — exercises auto-init end to
   end.
