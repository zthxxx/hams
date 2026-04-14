# Builtin Providers — Spec Delta (fix-v1-planning-gaps)

## MODIFIED

### Homebrew Provider: Tap Classification

The Homebrew provider SHALL recognize three distinct resource classifications in the Hamsfile:

1. **formula** — Standard Homebrew-core packages (default classification).
2. **cask** — GUI applications installed via `brew install --cask`.
3. **tap** — Third-party repositories added via `brew tap <user/repo>`.

Each classification SHALL be stored as a separate group/tag in the Hamsfile:

```yaml
formula:
  - app: git
    intro: Distributed revision control system

cask:
  - app: visual-studio-code
    intro: Code editing. Redefined.

tap:
  - app: homebrew/cask-fonts
    intro: Cask fonts repository
```

#### Scenario: brew tap recorded separately

Given the user runs `hams brew tap homebrew/cask-fonts`
When the provider records this to `Homebrew.hams.yaml`
Then the entry SHALL appear under the `tap` classification group
And SHALL NOT be mixed with formula or cask entries.

#### Scenario: brew tap probed during refresh

Given the Hamsfile contains tap entry `homebrew/cask-fonts`
When `hams refresh --only=homebrew` is run
Then the provider SHALL run `brew tap` to list installed taps
And SHALL mark the tap resource as `ok` if present, or `pending` if missing.

#### Scenario: brew tap applied

Given the Hamsfile contains tap entry `zthxxx/tap` not present in state
When `hams apply` processes the Homebrew provider
Then the provider SHALL run `brew tap zthxxx/tap` before processing formula/cask entries that may depend on it.

### All Providers: List Diff Display

Every builtin provider's `List()` method SHALL display a diff between desired (Hamsfile) and observed (state) resources, rather than just dumping state contents.

#### Scenario: provider list shows additions and removals

Given a provider with Hamsfile entries [A, B, C] and state entries [B, C, D]
When the user runs `hams <provider> list`
Then the output SHALL show:
- `+ A` (in Hamsfile, not in state)
- `  B` (matched)
- `  C` (matched)
- `- D` (in state, not in Hamsfile)
