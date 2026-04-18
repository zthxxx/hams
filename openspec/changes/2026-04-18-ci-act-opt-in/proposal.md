# Proposal: CI act → opt-in; docker-direct default for integration/e2e

## Why

Integration and e2e tests today route through `act` (`nektos/act`) by default. `act`'s built-in artifact-server emits `ECONNRESET` on `actions/upload-artifact@v4`'s final PUT, which makes `task test:e2e` / `task test:integration` / `task test:itest` fail unreliably on developer machines. The reference branch `origin/local/loop` (commit `d63edb9`) already diagnosed this and made the `ci:*` tasks (direct docker) the default, with `:one-via-act` variants retained as the explicit opt-in simulation. `dev` missed the change.

Until this lands, a developer verifying an integration-impacting change on their laptop cannot reproduce what CI runs — the `act` path fails on the artifact upload step every time. The CLAUDE.md rule *"Local/CI isomorphism: … MUST all run identically on a developer's local machine and in GitHub Actions CI"* is silently broken.

## What changes

1. `.github/workflows/ci.yml` — three `actions/upload-artifact@v4` steps gain `if: ${{ !env.ACT }}` so `act` simulation does not fail on the unreliable upload. Two downstream jobs (`integration`, `e2e`, `itest`) add an "act fallback" step that rebuilds the linux binary in-place via `task build:linux` when running under `act` (so they still have a binary to run, just one rebuilt by the runner instead of downloaded).
2. `Taskfile.yml`:
   - `test:e2e`, `test:e2e:one`, `test:integration`, `test:sudo`, `test:itest:one` — re-route to invoke the `ci:*` task directly via docker, with `deps: [build:linux]` so the binary is fresh.
   - `test:itest` added as a new top-level task that runs `ci:itest` directly.
   - New `:one-via-act` variants (`test:e2e:one-via-act`, `test:itest:one-via-act`) preserve the explicit act simulation entry point for anyone who needs the full runner reproduction.
   - Extended comment block documents the root cause (act artifact-server ECONNRESET).
3. `.golangci.yml` — `errcheck.exclude-functions` grows to cover the `io.Writer`-bound `fmt.Fprint*` + builder-write variants that are used throughout the CLI/provider layer for stdout/stderr prose. Write failures on stdout have no recoverable action, and sprinkling `//nolint:errcheck` across every call-site is noise.

## Impact

- **Capability `cli-architecture` / section "Verification"** — the canonical local-run paths for integration/e2e tests change from `act` to direct docker. The `:one-via-act` variants remain for anyone who needs act parity.
- **Capability `code-standards` / section "Go linting"** — `errcheck` no longer flags `fmt.Fprint*` + builder writes. Adopters avoid boilerplate `//nolint:errcheck` on stdout/stderr writes.
- **Developer experience** — `task test:itest:one PROVIDER=apt` becomes reliable. `task test:e2e:one TARGET=debian` becomes reliable. Fewer lint-boilerplate diffs in PRs.
- **CI behavior on real runners** — unchanged. Only the `ACT` environment variable (set by `act` itself) toggles the alternate path.
- **No user-visible product change.** Tooling only.
