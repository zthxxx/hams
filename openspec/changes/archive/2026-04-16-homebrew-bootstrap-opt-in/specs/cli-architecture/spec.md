# cli-architecture — Spec Delta

## ADDED Requirements

### Requirement: Apply with `--bootstrap` / `--no-bootstrap` flags

The `hams apply` command SHALL accept `--bootstrap` (boolean, default
`false`) and `--no-bootstrap` (boolean, default `false`) flags. The
two flags SHALL be mutually exclusive; specifying both is a usage
error (exit code 2).

Semantics:

- `--bootstrap` SHALL set a value on the command's `context.Context`
  (via `provider.WithBootstrapAllowed(ctx, true)`) that providers'
  `Bootstrap` methods query before executing any `DependsOn[].Script`.
  This is the non-interactive (CI / script) consent path.
- `--no-bootstrap` SHALL set the same context value to `false` AND
  SHALL suppress any interactive TTY prompt for consent — apply fails
  fast with an actionable error when a provider's prerequisite is
  missing.
- Without either flag, apply SHALL check whether stdin is a TTY: if so,
  providers that signal `ErrBootstrapRequired` from `Bootstrap` SHALL
  trigger an interactive `[y/N/s]` prompt (see the Homebrew provider
  spec for the exact prompt contents). If stdin is NOT a TTY, apply
  behaves identically to `--no-bootstrap`.

The flags are scoped to `hams apply`. `hams refresh` does not invoke
`Bootstrap` on providers (it only calls `Probe`), so no bootstrap
flag is wired there. Providers whose `Probe` fails because a
prerequisite is missing surface that as a regular probe error, which
`refresh` logs per-provider without aborting the whole run.

#### Scenario: Apply with `--bootstrap` delegates via provider framework

- **WHEN** the user runs `hams apply --bootstrap` on a machine without `brew` installed, and the Homebrew provider is included in the two-stage filter result
- **THEN** apply SHALL set `provider.WithBootstrapAllowed(ctx, true)` before calling `p.Bootstrap(ctx)`
- **AND** the Homebrew provider SHALL delegate `install.sh` execution via `provider.RunBootstrap(ctx, p, registry)`
- **AND** after the script exits 0, apply SHALL continue with Probe → Plan → Execute for Homebrew.

#### Scenario: Apply with `--no-bootstrap` fails fast

- **WHEN** the user runs `hams apply --no-bootstrap` on a machine without `brew` installed, and the Homebrew provider is included in the two-stage filter result
- **THEN** apply SHALL NOT prompt (even if stdin is a TTY)
- **AND** apply SHALL exit non-zero with the Homebrew `UserFacingError` describing the missing binary + remedy.

#### Scenario: Apply without either flag prompts on TTY

- **WHEN** the user runs `hams apply` (no bootstrap flags) on a machine without `brew` and stdin IS a TTY
- **THEN** apply SHALL display the consent prompt with the script text + side-effect warnings + Xcode-CLT gotcha
- **AND** on `y`, apply SHALL continue as if `--bootstrap` were set for this provider only
- **AND** on `N` or EOF, apply SHALL exit non-zero with the UserFacingError
- **AND** on `s`, apply SHALL skip the Homebrew provider for this run and continue with others.

#### Scenario: Apply without either flag fails fast when not a TTY

- **WHEN** the user runs `hams apply` (no bootstrap flags) via CI or a pipe (stdin not a TTY) on a machine without `brew`
- **THEN** apply SHALL NOT prompt
- **AND** apply SHALL exit non-zero with the UserFacingError.

#### Scenario: `--bootstrap` and `--no-bootstrap` are mutually exclusive

- **WHEN** the user runs `hams apply --bootstrap --no-bootstrap`
- **THEN** apply SHALL exit with code 2 and an error message naming the conflict
- **AND** no providers SHALL be bootstrapped, probed, or executed.
