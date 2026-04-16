# builtin-providers — Spec Delta

## ADDED Requirements

### Requirement: pnpm Bootstrap signals consent required when missing

The pnpm provider's `Bootstrap(ctx)` SHALL return a
`*provider.BootstrapRequiredError` (wrapping `provider.ErrBootstrapRequired`)
when `pnpm` is not on `$PATH`. The error SHALL carry:

- `Provider: "pnpm"`
- `Binary: "pnpm"`
- `Script: "npm install -g pnpm"`

The pnpm manifest's `DependsOn[0]` SHALL declare `Script: "npm install -g pnpm"`
so that `provider.RunBootstrap` can execute it under user consent
(via the `--bootstrap` flag or TTY `[y/N/s]` prompt).

When pnpm IS on PATH, `Bootstrap` SHALL return nil as before.

#### Scenario: pnpm missing produces structured error

- **WHEN** pnpm is not on `$PATH`
- **THEN** `Bootstrap` SHALL return `*provider.BootstrapRequiredError` with `Binary: "pnpm"` and `Script: "npm install -g pnpm"`
- **AND** the returned error SHALL unwrap to `provider.ErrBootstrapRequired`.

#### Scenario: pnpm --bootstrap delegates via npm

- **WHEN** the user runs `hams apply --bootstrap` on a machine with npm but no pnpm
- **THEN** the pnpm provider's `Bootstrap` signals `BootstrapRequiredError`
- **AND** apply delegates the script `npm install -g pnpm` via the bash provider
- **AND** retries `pnpm.Bootstrap`, which now returns nil since pnpm is on PATH.

### Requirement: duti Bootstrap signals consent required when missing

The duti provider's `Bootstrap(ctx)` SHALL return a
`*provider.BootstrapRequiredError` when `duti` is not on `$PATH`.
The error SHALL carry:

- `Provider: "duti"`
- `Binary: "duti"`
- `Script: "brew install duti"`

The duti manifest's `DependsOn` SHALL include `{Provider: "brew", Script: "brew install duti"}`.

#### Scenario: duti missing produces structured error

- **WHEN** duti is not on `$PATH` (macOS)
- **THEN** `Bootstrap` SHALL return `*provider.BootstrapRequiredError` with `Binary: "duti"` and `Script: "brew install duti"`.

### Requirement: mas Bootstrap signals consent required when missing

The mas provider's `Bootstrap(ctx)` SHALL return a
`*provider.BootstrapRequiredError` when `mas` is not on `$PATH`.
The error SHALL carry:

- `Provider: "mas"`
- `Binary: "mas"`
- `Script: "brew install mas"`

The mas manifest's `DependsOn` SHALL include `{Provider: "brew", Script: "brew install mas"}`.

#### Scenario: mas missing produces structured error

- **WHEN** mas is not on `$PATH` (macOS)
- **THEN** `Bootstrap` SHALL return `*provider.BootstrapRequiredError` with `Binary: "mas"` and `Script: "brew install mas"`.

### Requirement: ansible Bootstrap signals consent required when missing

The ansible provider's `Bootstrap(ctx)` SHALL return a
`*provider.BootstrapRequiredError` when `ansible-playbook` is not on
`$PATH`. The error SHALL carry:

- `Provider: "ansible"`
- `Binary: "ansible-playbook"`
- `Script: "pipx install --include-deps ansible"`

The ansible manifest's `DependsOn` SHALL include
`{Provider: "bash", Script: "pipx install --include-deps ansible"}`.

The error body SHALL note the pipx prerequisite so users without
pipx see a clear chain: "this requires pipx; install via `apt
install pipx` (Debian) or `brew install pipx` (macOS) first."

#### Scenario: ansible missing produces structured error with pipx note

- **WHEN** ansible-playbook is not on `$PATH`
- **THEN** `Bootstrap` SHALL return `*provider.BootstrapRequiredError` with `Binary: "ansible-playbook"` and `Script: "pipx install --include-deps ansible"`
- **AND** the surrounding error path SHALL include the pipx prerequisite note in suggestions.

### Requirement: Explicit skip-list for bootstrap-unsafe providers

The following providers SHALL NOT adopt the `BootstrapRequiredError`
pattern. Their `Bootstrap()` SHALL continue returning plain error
strings. Rationale is recorded here so future maintainers inherit
an auditable "here's why we didn't extend this one":

- `npm`, `cargo`, `goinstall`, `uv`: language runtime install is a
  user-owned decision (nvm/fnm/n/volta for node; rustup/distro for
  Rust; official installer/distro for Go; pipx/pip for Python
  tooling). hams does not have the right abstraction to pick a
  runtime installer on behalf of the user.
- `vscodeext`: requires a GUI app install (Visual Studio Code itself,
  not the tunnel-CLI). No reliable one-liner; the integration test
  already uses the Microsoft apt repo + root-safe wrapper.
- `apt`: platform-gated (`runtime.GOOS == "linux"`). Converting the
  error shape would mislead — it's not a missing-binary signal.
- `defaults`: platform-gated (macOS-only). Same reasoning as apt.
- `git`: git is pre-installed on any machine that ran the hams
  curl-installer (the installer uses git itself). No plausible
  fresh-machine case where git is missing.
- `bash`: always present; `Bootstrap` is already a no-op.

This skip-list is explicit scope; it SHALL be reconsidered only when
a concrete user story or support incident demonstrates one of these
providers as a blocker on a fresh-machine restore path.

#### Scenario: skipped providers retain plain-string errors

- **WHEN** an npm / cargo / goinstall / uv / vscodeext / apt / defaults / git provider is needed but its prerequisite binary is missing
- **THEN** `Bootstrap` SHALL return a plain `fmt.Errorf` string as today
- **AND** the CLI bootstrap loop SHALL surface that error via the existing "bootstrap failed for providers with hamsfiles" path (NOT the `BootstrapRequiredError` consent flow).
