# builtin-providers — Spec Delta

## MODIFIED Requirements

### Requirement: apt Provider

The apt provider SHALL wrap `apt-get install`, `apt-get remove`, and `dpkg -s` for Debian/Ubuntu-based Linux systems. The CLI handlers SHALL also write `<store>/.state/<machine-id>/apt.state.yaml` directly after each successful install / remove (in addition to mutating the hamsfile), so imperative actions produce a state-file audit trail without requiring a follow-up `hams apply`.

The auto-record path SHALL accept three install-args shapes:

1. **Bare name** — `hams apt install nginx` records `{app: nginx}` and probes `dpkg -s nginx` for the observed version. (Current behavior; unchanged.)
2. **Version pinned** — `hams apt install nginx=1.24.0` records `{app: nginx, version: "1.24.0"}` and probes `dpkg -s nginx`. The recorded `version` is the user's REQUESTED pin; the observed version (which may differ if apt resolved an exact alternative) goes into state's `version` field. Plan SHALL re-install when observed != requested.
3. **Release pinned** — `hams apt install nginx/bookworm-backports` records `{app: nginx, source: "bookworm-backports"}` and probes `dpkg -s nginx`. The release pin SHALL be replayed verbatim by the executor's install path on `hams apply`.

Dry-run flags (`--download-only`, `--simulate`, `-s`, `--just-print`, `--no-act`, `--recon`) MUST still trigger the "complex invocation: do not record" short-circuit — they are correctly unrecordable because no host state change occurred this invocation.

The `apt` Provider has the same shape as `homebrew`, with Debian/Ubuntu-specific commands. (The remainder of the existing apt Provider requirement — Provider metadata, command boundary, probe implementation, apply flow, remove flow, CLI wrapping, stdout/stderr policy, state ownership, complex invocation scope, LLM enrichment — is preserved verbatim except for the modifications described above.)

#### Scenario: Version-pinned install records structured entry

- **WHEN** the user runs `hams apt install nginx=1.24.0` on a Debian system
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])` so apt-get installs nginx pinned to version 1.24.0
- **AND** on success, SHALL append `{app: nginx, version: "1.24.0"}` to `apt.hams.yaml`
- **AND** on success, SHALL write `apt.state.yaml.resources.nginx` with `state=ok`, `version` populated from `dpkg -s nginx`, AND a `requested_version` field equal to `"1.24.0"`
- **AND** SHALL NOT emit the legacy "complex invocation; not auto-recorded" warning.

#### Scenario: Release-pinned install records structured entry

- **WHEN** the user runs `hams apt install nginx/bookworm-backports`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["nginx/bookworm-backports"])` so apt-get installs nginx from the bookworm-backports release
- **AND** on success, SHALL append `{app: nginx, source: "bookworm-backports"}` to `apt.hams.yaml`
- **AND** on success, SHALL record `apt.state.yaml.resources.nginx` with `state=ok` and the `source` field replicated.

#### Scenario: Plan re-installs when host version differs from pin

- **WHEN** `apt.hams.yaml` declares `{app: nginx, version: "1.24.0"}` and `apt.state.yaml.resources.nginx.version == "1.22.1"` (host was upgraded out of band)
- **THEN** Plan SHALL emit an `update` action for nginx
- **AND** Execute SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])` to re-pin.

#### Scenario: Dry-run flag still short-circuits auto-record

- **WHEN** the user runs `hams apt install --download-only nginx=1.24.0`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["--download-only", "nginx=1.24.0"])` so apt-get downloads but does not install
- **AND** SHALL emit the "complex invocation; not auto-recorded" warning
- **AND** SHALL NOT mutate `apt.hams.yaml` or `apt.state.yaml`. Dry-run wins over version-pinning recording — the host did not change, so no record is appropriate.

## ADDED Requirements

### Requirement: apt resource schema fields for version and release pinning

The hamsfile per-package entry for apt SHALL accept two optional fields beyond `app`:

- `version: "<spec>"` — the version specifier the user wants apt-get to pin. Forwarded verbatim to apt-get as `<app>=<version>` on install.
- `source: "<release>"` — the release/suite the user wants apt-get to install from. Forwarded verbatim to apt-get as `<app>/<release>`.

The state file's per-resource entry SHALL accept the symmetric `requested_version` and `requested_source` fields so refresh/probe can detect host drift away from the pin.

Both new hamsfile fields and both new state fields SHALL be optional and omitempty. Existing bare-name entries SHALL continue to round-trip without modification.

#### Scenario: Bare-name entry round-trips without new fields appearing

- **WHEN** the hamsfile contains `{app: htop}` and is loaded, mutated (e.g., a comment added), and saved
- **THEN** the persisted YAML SHALL still be `{app: htop}` — no spurious `version: ""` or `source: ""` keys SHALL appear.

#### Scenario: Version-pinned entry round-trips through hamsfile + state

- **WHEN** the hamsfile contains `{app: nginx, version: "1.24.0"}` and `hams apply` runs
- **THEN** the executor SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])`
- **AND** the resulting state row SHALL carry `requested_version: "1.24.0"` AND `version: "<observed>"` populated from `dpkg -s nginx`.
