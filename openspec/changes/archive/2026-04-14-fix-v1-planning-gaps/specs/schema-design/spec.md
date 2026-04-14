# Schema Design — Spec Delta (fix-v1-planning-gaps)

## ADDED

### `preview-cmd` Field for KV Config Resources

Hamsfile entries for KV config providers (defaults, duti, git-config) SHALL support an optional `preview-cmd` field:

- `preview-cmd` stores the original human-readable command string (e.g., `defaults write com.apple.dock autohide -bool true`).
- The field is for review/audit purposes only; execution SHALL use the structured `args` field.
- `preview-cmd` SHALL be preserved during YAML round-trips (comment preservation applies).
- When a KV config resource is recorded via CLI (e.g., `hams defaults write ...`), the provider SHALL auto-populate `preview-cmd` from the original command arguments.

#### Scenario: defaults provider records preview-cmd

Given the user runs `hams defaults write com.apple.dock autohide -bool true`
When the provider records this to `defaults.hams.yaml`
Then the entry SHALL contain:
```yaml
- urn: urn:hams:defaults:com.apple.dock.autohide
  args:
    domain: com.apple.dock
    key: autohide
    type: bool
    value: "true"
  preview-cmd: "defaults write com.apple.dock autohide -bool true"
```

## MODIFIED

### Sensitive Config Key Detection

`hams config set` SHALL detect sensitive keys using both exact match and substring pattern matching:

- Exact matches: `llm_cli` (existing).
- Substring patterns: keys containing `token`, `key`, `secret`, `password`, `credential`.
- When a sensitive key is detected, the value SHALL be written to `hams.config.local.yaml` instead of `hams.config.yaml`.
- A log message SHALL inform the user that the value was routed to `.local.yaml` for security.

#### Scenario: token key auto-routes to local config

Given the user runs `hams config set notification.bark_token abc123`
When the config module detects `bark_token` contains the substring `token`
Then the value SHALL be written to `hams.config.local.yaml`
And a message SHALL be logged: "Sensitive key 'notification.bark_token' written to .local.yaml".
