# Codex Review Findings (base: 97cdb7b)

Findings from Codex review of branch diff against `97cdb7b`.

## P1 — Critical

- [ ] **CR-1: Use provider-specific planning during apply** (`internal/cli/apply.go:163-166`)
  `runApply` bypasses `Provider.Plan` and feeds raw IDs into `provider.ComputePlan`, so every `Action` loses the provider-specific payload. Breaks any provider whose `Apply` needs more than an ID (e.g. bash provider needs `action.Resource` to carry command text).

- [ ] **CR-2: Locate Hamsfiles with the manifest file prefix** (`internal/cli/apply.go:142-145`)
  Path is built from `manifest.Name`, but several providers use a different `FilePrefix` (`Homebrew.hams.yaml` for `brew`, `vscodeext.hams.yaml` for `code-ext`). `hams apply` silently skips those providers.

- [ ] **CR-3: Bootstrap providers before probing or applying** (`internal/cli/apply.go:120-127`)
  After resolving DAG, we immediately probe/apply without calling `Provider.Bootstrap`. On fresh machines, providers like `brew`, `pnpm`, `code-ext` fail because underlying CLIs are not installed yet.

- [ ] **CR-4: Persist CLI installs into the store** (`internal/provider/builtin/homebrew/homebrew.go:155-163`)
  `hams brew install bat` only shells out to `brew install` and returns; never updates `Homebrew.hams.yaml` or state. The install is not declarative — won't replay on a new machine.

## P2 — Major

- [ ] **CR-5: Save refreshed state to disk** (`internal/cli/commands.go:46-47`)
  `hams refresh` throws away the map returned by `provider.ProbeAll` and exits without persisting state files. Later `list` and `apply` operate on stale data.

- [ ] **CR-6: Set ConfigHash after a successful apply** (`internal/cli/apply.go:164-169`)
  `ComputePlan` only generates removals when `sf.ConfigHash` is non-empty, but code never updates `ConfigHash` before saving state. Packages removed from config stay installed indefinitely.

- [ ] **CR-7: Replace placeholder Homebrew checksums** (`Formula/hams.rb:10-11`)
  Formula ships literal `"PLACEHOLDER"` SHA256 values. `brew install zthxxx/tap/hams` fails on every platform. (Deferred to release automation.)
