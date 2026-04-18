# Spec delta: provider-system — passthrough for unhandled subcommands

## ADDED Requirement: CLI-wrapping providers SHALL passthrough unhandled subcommands

Every builtin provider that wraps a CLI tool (`hams git`, `hams brew`, `hams apt`, `hams npm`, `hams pnpm`, `hams uv`, `hams cargo`, `hams goinstall`, `hams code`, `hams mas`, …) SHALL, for any subcommand it does not explicitly intercept, invoke the wrapped tool transparently — preserving stdin, stdout, stderr, and exit code. This includes:

- `hams git log --oneline` → `git log --oneline`
- `hams git pull origin main` → `git pull origin main`
- `hams brew upgrade htop` → `brew upgrade htop`
- `hams apt list --installed` → `apt list --installed`
- etc.

Providers MUST NOT surface "unknown subcommand" errors for verbs the underlying tool itself supports. The only subcommands a provider SHALL intercept are those that require auto-record side effects (install/remove for package managers; `config` / `clone` for git; similarly narrow sets elsewhere).

A package-level `passthroughExec` (or equivalent DI-seamed exec helper) SHALL be used so unit tests can assert the passthrough invocation without spawning a real process.

When `flags.DryRun` is set, the passthrough SHALL print `[dry-run] Would run: <tool> <args>` to `flags.Stdout()` and return nil without exec.

#### Scenario: user runs `hams git log`

- **Given** a user types `hams git log --oneline`
- **When** `UnifiedHandler.HandleCommand` dispatches
- **Then** the first positional arg `log` matches no intercepted verb (`config`, `clone`), and the default branch invokes `passthrough(ctx, args, flags)`. The passthrough execs real `git log --oneline` with stdio preserved.

#### Scenario: user runs `hams --dry-run git status`

- **Given** `flags.DryRun == true`
- **When** `passthrough` runs
- **Then** the handler prints `[dry-run] Would run: git status` to `flags.Stdout()` and returns nil WITHOUT invoking `git`.

#### Scenario: real git exits non-zero

- **Given** `hams git push` that the real git rejects (no upstream, auth failure, etc.)
- **When** the passthrough runs
- **Then** the non-zero exit bubbles up to hams's exit code unchanged — the user sees the same error they would from plain git.

## MODIFIED Requirement: `standard_cli_flow` integration helper

Because single-provider CLI verbs now satisfy `Manifest().Name == CLI verb == FilePrefix` (after the 2026-04-18 `code-ext` → `code` rename and the spec above that forbids drift), the `MANIFEST_NAME` env var in `e2e/base/lib/provider_flow.sh::standard_cli_flow` SHALL only be required for aggregator CLI verbs (e.g., `hams git` which multiplexes `git-config` + `git-clone`). Single-provider CLI verbs SHALL use the default (`MANIFEST_NAME=$provider`) with no override.
