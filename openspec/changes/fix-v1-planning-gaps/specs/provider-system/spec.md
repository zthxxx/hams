# Provider System — Spec Delta (fix-v1-planning-gaps)

## MODIFIED

### Update Hooks Invocation

The provider executor SHALL invoke pre-update and post-update hooks during resource upgrade operations, following the same contract as install hooks:

- `RunPreUpdateHooks()` SHALL execute before the update action.
- `RunPostUpdateHooks()` SHALL execute after a successful update action.
- Pre-update hook failure SHALL prevent the update and mark the resource as `failed`.
- Post-update hook failure SHALL mark the resource as `hook-failed` (the update itself succeeded).
- Deferred update hooks (`defer: true`) SHALL be collected and executed after all resources in the current provider are processed.

#### Scenario: Package upgrade triggers update hooks

Given a Hamsfile entry for `htop` with a `post-update` hook that runs `htop --version > /tmp/htop-version`
When `hams apply` detects that `htop` needs upgrading (state version differs from available)
Then the provider executor SHALL run the post-update hook after the upgrade completes
And the state SHALL record hook success or failure independently from the upgrade result.

### Provider List Diff

The `List()` method on every provider SHALL compare desired resources (from Hamsfile) against observed resources (from state) and present a diff:

- Resources in Hamsfile but not in state SHALL be marked as additions (`+`).
- Resources in state but not in Hamsfile SHALL be marked as removals (`-`).
- Resources in both but with divergent status SHALL be marked as mismatches (`~`).
- Output SHALL support both human-readable (colored) and `--json` machine-readable formats.

#### Scenario: hams brew list shows diff

Given a Hamsfile containing `git` and `curl`, and state containing `git` and `wget`
When the user runs `hams brew list`
Then the output SHALL show `curl` as `+` (desired but not installed), `wget` as `-` (installed but not desired), and `git` as matched.
