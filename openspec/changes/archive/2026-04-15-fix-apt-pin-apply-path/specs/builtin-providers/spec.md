# builtin-providers — Spec Delta

## MODIFIED Requirements

### Requirement: apt Provider

The apt provider SHALL wrap `apt-get install`, `apt-get remove`, and `dpkg -s` for Debian/Ubuntu-based Linux systems. The CLI handlers SHALL also write `<store>/.state/<machine-id>/apt.state.yaml` directly after each successful install / remove (in addition to mutating the hamsfile), so imperative actions produce a state-file audit trail without requiring a follow-up `hams apply`.

The auto-record path SHALL accept three install-args shapes (bare, version pinned, release pinned). The recording loop SHALL upgrade an existing bare entry to pinned in-place when the user re-runs the install with a pin. Bookkeeping SHALL call the hamsfile's structured-fields helper unconditionally (not just on absent entries) — the helper merges the new fields into the existing mapping node when an entry already exists under any tag.

`Plan` SHALL inspect each declared app's structured fields directly from the hamsfile (not just the app name) so that a hamsfile authored or restored on a fresh machine — including the `apply --from-repo=...` bootstrap path — replays the user's pinned versions on first install. Drift detection (observed dpkg version differs from the requested pin) SHALL emit an Update action whose `ID` remains the bare package name AND whose `Resource` field carries the install-token form (`pkg=version` / `pkg/source`). The executor's `Provider.Apply` SHALL prefer `action.Resource` over `action.ID` when invoking the runner so that state stays keyed on the canonical bare name (no `nginx=1.24.0` orphan rows).

(The remainder of the existing apt Provider requirement is preserved verbatim except for the modifications described above.)

#### Scenario: Version-pinned install records structured entry

- **WHEN** the user runs `hams apt install nginx=1.24.0` on a Debian system
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])` so apt-get installs nginx pinned to version 1.24.0
- **AND** on success, SHALL append `{app: nginx, version: "1.24.0"}` to `apt.hams.yaml`
- **AND** on success, SHALL write `apt.state.yaml.resources.nginx` with `state=ok`, `version` populated from `dpkg -s nginx`, AND a `requested_version` field equal to `"1.24.0"`
- **AND** SHALL NOT emit the legacy "complex invocation; not auto-recorded" warning.

#### Scenario: Existing bare entry is upgraded to pinned in-place

- **WHEN** `apt.hams.yaml` already contains a bare entry `{app: nginx}` AND the user runs `hams apt install nginx=1.24.0`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])`
- **AND** SHALL update the existing nginx entry in `apt.hams.yaml` IN PLACE to `{app: nginx, version: "1.24.0"}` (NOT add a duplicate entry, NOT leave the bare entry unchanged)
- **AND** SHALL set `apt.state.yaml.resources.nginx.requested_version = "1.24.0"`.

#### Scenario: Plan replays hamsfile-declared pin on fresh state

- **WHEN** `apt.hams.yaml` declares `{app: nginx, version: "1.24.0"}` AND `apt.state.yaml` has no entry for nginx (fresh machine OR restore path)
- **THEN** `Plan` SHALL emit an Install action with `ID = "nginx"` AND `Resource = "nginx=1.24.0"`
- **AND** Execute SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])`
- **AND** the resulting state row SHALL be keyed on the bare name `nginx` AND carry `requested_version: "1.24.0"`.

#### Scenario: Plan re-installs when host version differs from pin (state-key invariant)

- **WHEN** `apt.hams.yaml` declares `{app: nginx, version: "1.24.0"}` and `apt.state.yaml.resources.nginx` has `version: "1.22.1"` (host was upgraded out of band)
- **THEN** `Plan` SHALL emit an Update action with `ID = "nginx"` AND `Resource = "nginx=1.24.0"`
- **AND** Execute SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])` to re-pin
- **AND** the state file SHALL retain a SINGLE row for nginx, keyed on the bare name (no duplicate `resources["nginx=1.24.0"]` orphan).

#### Scenario: Release-pinned install records structured entry

- **WHEN** the user runs `hams apt install nginx/bookworm-backports`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["nginx/bookworm-backports"])` so apt-get installs nginx from the bookworm-backports release
- **AND** on success, SHALL ensure `apt.hams.yaml` contains `{app: nginx, source: "bookworm-backports"}` (whether by appending a new entry or upgrading an existing bare one in place)
- **AND** on success, SHALL record `apt.state.yaml.resources.nginx` with `state=ok` and the `source` field replicated.

#### Scenario: Dry-run flag still short-circuits auto-record

- **WHEN** the user runs `hams apt install --download-only nginx=1.24.0`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["--download-only", "nginx=1.24.0"])` so apt-get downloads but does not install
- **AND** SHALL emit the "complex invocation; not auto-recorded" warning
- **AND** SHALL NOT mutate `apt.hams.yaml` or `apt.state.yaml`. Dry-run wins over version-pinning recording — the host did not change, so no record is appropriate.

#### Scenario: Benign passthrough flag still auto-records

- **WHEN** the user runs `hams apt install --no-install-recommends htop`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["--no-install-recommends", "htop"])` and continue into the auto-record path
- **AND** SHALL append `{app: htop}` to `apt.hams.yaml`
- **AND** SHALL write `apt.state.yaml.resources.htop.state = ok`. Benign flags (those that do not pin versions, pin releases, or short-circuit installation) preserve the auto-record contract.

## ADDED Requirements

### Requirement: Hamsfile structured-fields read API

The hamsfile package SHALL expose `(*File).AppFields(appName string) map[string]string` returning the structured per-app fields (e.g., `version`, `source`) for the entry whose `app` value matches `appName`. The `app` and `intro` fields SHALL be omitted from the result; only the optional structured fields are returned. When no entry matches, SHALL return nil.

This API is the read-side counterpart to `AddAppWithFields`: callers that need to consult per-app structured fields (e.g., apt's `Plan` reading version/source pins from a hamsfile) SHALL use this helper rather than re-walking the YAML node tree at every call site.

#### Scenario: AppFields returns recorded structured fields

- **WHEN** the hamsfile contains `{app: nginx, version: "1.24.0", source: "bookworm-backports"}`
- **AND** a caller invokes `f.AppFields("nginx")`
- **THEN** the result SHALL be a map equal to `{"version": "1.24.0", "source": "bookworm-backports"}` (NO `app` key, NO `intro` key).

#### Scenario: AppFields returns nil for unknown apps

- **WHEN** the hamsfile does not contain an entry for `nginx`
- **AND** a caller invokes `f.AppFields("nginx")`
- **THEN** the result SHALL be nil (NOT an empty non-nil map).

#### Scenario: AppFields returns nil for bare entries

- **WHEN** the hamsfile contains `{app: htop}` (no extra fields)
- **AND** a caller invokes `f.AppFields("htop")`
- **THEN** the result SHALL be nil OR an empty map (callers MUST NOT distinguish between the two; both signal "no structured fields recorded").
