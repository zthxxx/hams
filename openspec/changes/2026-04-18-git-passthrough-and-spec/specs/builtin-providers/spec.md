# Spec delta: builtin-providers — hams git clone natural form

## MODIFIED Requirement: `hams git clone` grammar

The `hams git clone` handler SHALL accept two shapes:

- Natural git form: `hams git clone <remote> <path>` — the handler folds `<path>` into `hamsFlags["path"]` and forwards to the CloneProvider's `add <remote>` verb.
- Legacy form: `hams git clone <remote> --hams-path=<path>` — preserved unchanged so pre-2026-04-18 scripts keep working.

Unforwarded git flags (`--depth`, `--branch`, `--recurse-submodules`, …) SHALL cause the handler to emit a UserFacingError naming the rejected flag and pointing the user at:

- filing a follow-up for the flag to become a supported forwarding, OR
- running plain `git clone ...` and recording the result afterward via `hams git clone <remote> <path>`.

Silently dropping flags is forbidden because a user expecting a shallow clone from `--depth=1` would get a full clone, wasting bandwidth and disk.

The management sub-verbs `hams git clone remove <urn>` and `hams git clone list` SHALL continue to forward directly to the CloneProvider's own CLI handler without translation.

#### Scenario: user types natural git form

- **Given** `hams git clone https://github.com/example/repo.git /tmp/repo`
- **When** the unified handler dispatches
- **Then** `hamsFlags["path"]` is set to `/tmp/repo`, and the CloneProvider is invoked with args `["add", "https://github.com/example/repo.git"]` — recording the clone exactly as if the legacy `--hams-path=` form had been used.

#### Scenario: user passes unforwarded git flag

- **Given** `hams git clone https://example.com/repo.git /tmp/repo --depth=1`
- **When** the unified handler scans the positional args
- **Then** the `--depth=1` token triggers a `UserFacingError` with `ExitUsageError`, naming the rejected flag. No clone is performed.
