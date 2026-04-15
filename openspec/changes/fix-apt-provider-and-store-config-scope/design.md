# Design: fix-apt-provider-and-store-config-scope

## Context

The apt builtin provider was scaffolded but its CLI handlers (`hams apt install`, `hams apt remove`) never evolved past passthrough to `apt-get`. They never mutate the hamsfile or the state file, silence stdout/stderr, and — because the `apt-get` command boundary is not isolated via DI — the bug is invisible to unit tests (no test can observe "command ran but state untouched" without shelling out to real apt, which would trash the host).

At the same time, three adjacent correctness gaps surfaced on the same store file:

1. The state struct lacks a `removed_at` timestamp — `SetResource(id, StateRemoved)` only bumps `updated_at`, losing the signal of "when was this removed".
2. `install_at` semantics are ambiguous — the field is set on first install and then never touched, but the field name implies "the install time", which could reasonably be interpreted as re-install time too. Naming drives behavior.
3. `<store>/hams.config.yaml` silently accepts `profile_tag` and `machine_id`, which are machine-scope fields. A shared store repo with those keys committed overwrites every collaborator's machine identity when they clone.

And the CI workflow inlines raw commands (`go test -race`, `golangci-lint run`, `docker build`), which drifts from `task <name>` used locally — a Local/CI isomorphism violation already called out in `.claude/rules/development-process.md`.

Current state snapshot (verified by exploration):

- `internal/provider/builtin/apt/apt.go:80-86` — `Apply` runs apt-get, sets `cmd.Stdout = nil, cmd.Stderr = nil` (silencing).
- `internal/provider/builtin/apt/apt.go:102-129` — `HandleCommand` for `install`/`remove` runs apt-get directly, no hamsfile mutation.
- `internal/provider/builtin/homebrew/homebrew.go:292-337` — `handleInstall` shows the correct pattern: run `brew install`, then load hamsfile, call `hf.AddApp`, write back.
- `internal/state/state.go:44-54` — `Resource` has `InstallAt`, `UpdatedAt`, `CheckedAt` as `string` (format `20060102T150405`), no `RemovedAt`. `SchemaVersion = 1` hard-coded.
- `internal/state/state.go:96-126` — `SetResource`: `StateOK` sets `InstallAt` only if empty (idempotent), always bumps `UpdatedAt`; `StateRemoved` only bumps `UpdatedAt`.
- `internal/config/merge.go:27-31` — `mergeConfig` unconditionally merges `ProfileTag` and `MachineID` from store-level into base.
- `examples/basic-debian/store/hams.config.yaml` + multiple `e2e/fixtures/*/hams.config.yaml` — carry `profile_tag` and `machine_id` at store level, baking the bug into templates.
- `.github/workflows/ci.yml` — inline commands for build/test/lint/integration/e2e steps.

Stakeholders: any user of `hams apt`, any user whose store repo is shared with teammates, anyone inspecting state files for audit, and any contributor who expects `task ci:*` to match what CI runs.

## Goals / Non-Goals

**Goals:**

- `hams apt install <pkg>` and `hams apt remove <pkg>` achieve end-to-end correctness: real `apt-get` runs, user sees its stdout/stderr, hamsfile is updated, `hams apply` then reconciles state with `state: ok` / `state: removed` and proper timestamps.
- State schema captures install identity (`first_install_at`, immutable) distinct from last-mutation time (`updated_at`) and removal time (`removed_at`). Old state files migrate forward automatically.
- Store-level config files fail loudly (not silently) when they contain machine-scope fields. Error messages are actionable.
- Apt's `apt-get`/`dpkg` command boundary is DI-isolated so unit tests catch "command ran but state untouched" class bugs without Docker.
- Debian E2E has end-to-end scenarios that exercise the full install → remove → re-install → migration cycle against real `apt-get`.
- `.github/workflows/ci.yml` is the thin glue — every build/test/lint step calls a named Taskfile task via `arduino/setup-task@v2`. Local `task <name>` and CI `task <name>` are byte-for-byte identical.

**Non-Goals:**

- Not changing any other builtin provider's install/remove behavior. Other providers already follow the correct pattern (brew) or have a separate bug that belongs to a separate change. The `install_at` → `first_install_at` rename touches every provider's state reads mechanically, but behavior changes apply to apt only.
- Not introducing a general "config field scoping" framework. Only the two known machine-scope fields (`profile_tag`, `machine_id`) are validated in this change. A generalized scope-annotation system on `Config` struct tags is future work.
- Not changing the `sudo.CmdBuilder` abstraction. The new apt `CmdRunner` sits above it, not replacing it.
- Not introducing dynamic-mode dispatch in CI (e.g., matrix → different Taskfile tasks). Workflow still enumerates its `task <name>` steps; just no longer inlines raw commands.
- Not altering the provider plugin system (`hashicorp/go-plugin` boundary). External providers are out of scope.
- Not changing the store repo layout or profile directory semantics.

## Decisions

### D1. Apt CLI handlers mirror brew's pattern, not a "direct state write" shortcut

**Decision:** `hams apt install <pkg>` runs `apt-get install -y <pkg>`, then loads the relevant `apt.hams.yaml` (+ local override), adds `{app: <pkg>}` if absent, writes back. Remove mirrors it. State file is NOT written by the CLI path — only by the executor on `hams apply`.

**Alternatives considered:**

- Direct state write (CLI handler writes `state: ok` into `apt.state.yaml`). Rejected: introduces a second writer to the state file, breaking the "executor is the single state mutator" invariant that makes refresh-then-diff reasoning tractable.
- Trigger an immediate apply inside the CLI handler (so one command = reconciled). Rejected: doubles command latency, and users expect `hams apt install foo` to be fast (it already has). `hams apply` is idiomatic and users invoke it routinely.

**Why:** Symmetry with brew. The executor is already the single writer for state; CLI just declares intent via the hamsfile. This keeps the mental model uniform across all 15 package providers.

### D2. `CmdRunner` interface sits above `sudo.CmdBuilder`, not replaces it

**Decision:** Apt gains a new `internal/provider/builtin/apt/command.go`:

```go
type CmdRunner interface {
    Install(ctx context.Context, pkg string) error
    Remove(ctx context.Context, pkg string) error
    IsInstalled(ctx context.Context, pkg string) (installed bool, version string, err error)
}
```

Real impl `realCmdRunner{sudo sudo.CmdBuilder}` uses the existing `sudo.CmdBuilder` internally, streams stdout/stderr to `os.Stdout`/`os.Stderr`. Fake impl (in `command_fake.go`) maintains an `map[string]bool` installed-set + records all calls.

**Alternatives considered:**

- Wrap at the `sudo.CmdBuilder` layer globally. Rejected: `sudo.CmdBuilder` is an abstraction over sudo privilege escalation shared across providers; replacing it affects every provider's test surface and bleeds concerns. Better to add a narrower, apt-owned seam.
- Use `os/exec` function variables (e.g., `var execCommand = exec.Command`). Rejected: stringly-typed, fragile, doesn't enforce the three-verb surface (Install/Remove/IsInstalled).
- Reuse `provider.WrapExecPassthrough` (used at apt.go:127 for unknown verbs). Rejected: it's a generic shell-out helper, not a typed boundary. Tests would still have to stub out `exec`.

**Why:** The DI seam must be narrow, typed, and owned by the provider. That's the only way unit tests can assert "`runner.Install` was called once with `bat`" AND "the hamsfile was modified" as separate, composable checks.

### D3. `RemovedAt` is a `string`, not `*time.Time`

**Decision:** Add `RemovedAt string \`yaml:"removed_at,omitempty"\`` to `state.Resource`. Stored as the same `20060102T150405` format used by the existing fields (see `state.go:104`).

**Alternatives considered:**

- `*time.Time` with custom YAML marshalling. Rejected: inconsistent with `InstallAt`, `UpdatedAt`, `CheckedAt`, which are all `string`. Introducing a single `*time.Time` field would mean either migrating all fields (out of scope) or living with inconsistency.

**Why:** Consistency with existing fields. Empty string + `omitempty` gives the same YAML absence behavior as `*time.Time` + `omitempty` would. No functional loss.

### D4. `FirstInstallAt` immutability enforced in `SetResource`, not at struct level

**Decision:** `SetResource(id, StateOK)` logic:

```
r := f.Resources[id] (or new)
if r.FirstInstallAt == "" { r.FirstInstallAt = now }  // only on brand-new records
r.State = StateOK
r.RemovedAt = ""                                       // clear on re-install
r.UpdatedAt = now
r.LastError = ""
```

`SetResource(id, StateRemoved)` logic:

```
r.State = StateRemoved
r.RemovedAt = now
r.UpdatedAt = now
// FirstInstallAt untouched
```

**Alternatives considered:**

- Enforce immutability via an unexported setter + factory. Rejected: `Resource` is a plain struct marshalled to YAML; public fields are an API constraint already. The `SetResource` convention (single mutation entrypoint) is already established.
- Introduce a separate `ReInstallAt` field. Rejected: the user explicitly chose "only update `UpdatedAt` on re-install" — `UpdatedAt` already carries that signal, adding a third timestamp field is noise.

**Why:** `SetResource` is the single code path for state transitions today (verified in `state.go:96-126`); extending its switch statement is less invasive than adding new types.

### D5. Schema migration is loader-only and one-way (v1 → v2)

**Decision:** On `state.Load`, after `yaml.Unmarshal`, check `f.SchemaVersion`. If 0 or 1, run an in-place migration:

- For each `Resource` r: if `r.InstallAt != ""` AND `r.FirstInstallAt == ""`, move `r.InstallAt` → `r.FirstInstallAt`.
- Set `f.SchemaVersion = 2`.
- The struct still has `InstallAt` as a read-only field during migration (tag `yaml:"install_at,omitempty"`), cleared after copy so it doesn't re-serialize.

On `Save`, the file is written with `schema_version: 2` and `first_install_at`, never `install_at`.

No `v2 → v1` downgrade path.

**Alternatives considered:**

- Keep both `install_at` and `first_install_at` indefinitely. Rejected: confusing, doubles struct surface, tempts future code to pick the "wrong" field.
- Require a manual migration command (`hams migrate`). Rejected: adds user friction for a trivial key rename; auto-migration is the Terraform-style norm and users expect `.state/` to be hams-managed.
- Version-specific loaders (`LoadV1`, `LoadV2`). Rejected: overkill for a single-field rename.

**Why:** State files are per-machine-per-store, not distributed; a forward-only migration matches the single-writer ownership model. Downgrade-compatibility would require dual-writing both keys, which is worse than just bumping the version.

### D6. Store-level validation happens before merge, with full file path in the error

**Decision:** New `internal/config/config.go:validateStoreScope(cfg *Config, filePath string) error`. Called immediately after `yaml.Unmarshal` of any store-level config file, before `mergeConfig`. Fails on non-empty `cfg.ProfileTag` or `cfg.MachineID` with:

```
%s: field %q is machine-scoped and must not be set at store level. Move it to ~/.config/hams/hams.config.yaml (or hams.config.local.yaml for untracked per-machine overrides).
```

Validation applies to both `<store>/hams.config.yaml` (git-tracked) AND `<store>/hams.config.local.yaml` (gitignored) — the user confirmed symmetric strictness in brainstorming.

**Alternatives considered:**

- Post-merge validation (check values on merged config). Rejected: can't tell whether the offending value came from global or store file after merge.
- Warn-only. Rejected per brainstorming Q2 — hard-fail chosen because IaC tools that silently misconfigure bite hard on fresh-machine restore.
- Custom YAML decoder that rejects fields at parse time. Rejected: YAML v3 supports UnmarshalStrict but not per-caller-scoped field rejection. Post-unmarshal check is simpler.

**Why:** Clear, targeted error at the right moment in the pipeline. Matches the user's "hard fail with actionable message pointing to the global path" requirement.

### D7. `.github/workflows/ci.yml` is thin glue; Taskfile is the single source of truth

**Decision:** Every step in `ci.yml` uses `arduino/setup-task@v2` to install `task`, then calls `run: task <name>`. Raw `go`, `go-task`, `golangci-lint`, `docker`, `pnpm`, and shell commands disappear from workflow YAML. Missing `ci:*` task compositions are added to `Taskfile.yml` first.

**Alternatives considered:**

- Keep raw commands, rely on discipline. Rejected per `.claude/rules/development-process.md` newly-added "GitHub Actions invoke Taskfile tasks, never raw commands" rule (which this change itself adds to the ruleset).
- Use GitHub Actions reusable workflows instead of tasks. Rejected: reusable workflows don't run locally; Taskfile tasks run identically under `act` and on dev machines.
- Shell script wrappers instead of tasks. Rejected: Taskfile already exists and handles deps, platform detection, and verbose output.

**Why:** One command to run, one place to change it. Any drift between `task ci:integration` locally and CI becomes impossible by construction.

### D8. New capability slots

**Decision:** This change does not introduce a new OpenSpec capability. All deltas land in existing specs:

- `schema-design` — state schema evolution + store-level field scoping rule.
- `builtin-providers` — apt-specific MODIFIED requirements.
- `project-structure` — GitHub Actions workflow step composition rule.

**Alternatives considered:**

- New `config-scope-validation` capability. Rejected: one rule over two fields doesn't warrant a capability boundary; it fits naturally under `schema-design` which already owns the store config schema (the `schema-design` spec at line 17+ currently lists `profile_tag` and `machine_id` as global-only fields without a scoping enforcement requirement — this change fills that gap).

**Why:** YAGNI. Capability boundaries exist to isolate independent spec evolution; this rule coevolves with the config schema itself.

## Risks / Trade-offs

- **[State schema migration is one-way]** → A user who rolls back to an older hams binary after running the new one will hit "unknown field first_install_at" errors. Mitigation: call out in CHANGELOG as a breaking state-file change. Risk is low because `.state/` is a local directory, not distributed, and users rolling back can `rm -rf .state/<machine-id>/` and re-run `hams apply` to rebuild.

- **[Rename touches every provider's state reads]** → `grep -w InstallAt` across the codebase will catch struct field accesses, but any external plugin binary compiled against the old struct type breaks. Mitigation: the state struct is not in `pkg/`, only `internal/` — external plugins never see this field directly; they receive state through the gRPC provider boundary which marshals YAML, and YAML migration handles them.

- **[Hard-fail on store-level `profile_tag` breaks existing user repos]** → Any user who committed `profile_tag: macOS` into their store repo's `hams.config.yaml` will see `hams apply` fail on next update. Mitigation: the error message tells them exactly what to do. This is preferable to silent override — discovering it at first use of a second machine is much worse. Fixing the bundled example templates and E2E fixtures ships with this change.

- **[DI fake must stay faithful to real apt behavior]** → If the fake `CmdRunner` diverges from what `apt-get` actually does (e.g., exit codes, idempotency of re-install), unit tests could pass while E2E fails. Mitigation: debian E2E scenarios E1–E5 are required — they exercise the identical orchestration paths against real `apt-get`. Unit tests are for the orchestration logic (did we call the runner? did we write the hamsfile?), not the runner's fidelity.

- **[`arduino/setup-task@v2` as a new workflow dependency]** → Action could be deprecated or change major versions. Mitigation: pin to `@v2`, not `@latest`. Setup-task is actively maintained (Arduino Fork of the original go-task maintainer's action) with a stable 1.0-style API.

- **[`yq` in debian integration Dockerfile]** → Adds a dependency to the container image. Mitigation: `yq` is a tiny Go binary installed via a release download or `apt-get install yq`. Alternative (no dependency): use `grep`+`awk` for structural YAML assertions, but that's fragile. Prefer the explicit tool.

- **[`arduino/setup-task@v2` requires network access in CI]** → Setup action downloads `task` binary. In offline CI, this would fail. Mitigation: mainstream CI (GitHub Actions hosted runners) has internet. Air-gapped CI is not a current requirement. If it becomes one, vendor `task` into `bin/` and skip the action.

## Migration Plan

1. **Ship the state schema change behind the automatic v1→v2 migration** so existing `.state/<machine-id>/*.state.yaml` files upgrade on first read. No user action required; first run of the new binary rewrites the file on next save.
2. **Ship the config-scope validation at the same release.** Users with `profile_tag`/`machine_id` in store-level config get an immediate error with remediation instructions.
3. **Update bundled `examples/.template/` and `examples/basic-debian/` first.** Users who `--from-repo=.template` on a fresh machine never see the bug again.
4. **Add CHANGELOG entry** under "Breaking changes" for the state schema v1→v2 auto-migration AND for the store-level config rejection.
5. **No rollback plan** for state schema (one-way). For config-scope rejection: users can re-add `profile_tag`/`machine_id` to store-level if they revert hams; the rejection only activates on the new binary.

## Open Questions

None. All design decisions above are derived from the brainstorming Q1–Q5 resolutions in the preceding conversation; no pending ambiguities.
