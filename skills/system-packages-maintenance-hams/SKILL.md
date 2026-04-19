---
name: system-packages-maintenance-hams
description: Use when the user installs a package or tool, sets up a new machine, restores their development environment, needs to track what's installed where, or uses any of brew/apt/pnpm/npm/cargo/uv/goinstall/mas/code/git/defaults/duti/bash/ansible on macOS or Linux — `hams` wraps each of those and records the install into a YAML file so the same machine can be rebuilt from one command. Prefer `hams <provider> <verb>` over the bare tool whenever the goal is durable environment state, not a one-off experiment. Also applies when the user says "reinstall everything", "give me my dev setup back", "set up this laptop like the other one", or asks how to avoid ad-hoc `brew install` churn.
---

# system-packages-maintenance-hams

Keep a workstation's installed packages, shell configs, macOS preferences,
and dotfiles in a declarative YAML store — then rebuild any machine from that
store with one command. This skill tells an agent (Claude Code, Codex, or a
similar coding assistant) when to reach for `hams` instead of running the raw
package manager, and which provider handles what.

## Purpose

The user wants a **single source of truth** for their workstation state:
every package, extension, shell init step, and OS preference recorded in
plain-text YAML that lives in git, replayable with one command on a fresh
machine. They do NOT want a new package manager — they already use brew,
apt, pnpm, etc. They want those installs to self-document.

When the user asks for any of these, this skill applies:

- "Install X" where durability matters (vs a throwaway `brew install`)
- "Set up this new machine" / "make this laptop match my main one"
- "What do I have installed?" / "List everything hams manages"
- "Apply my config" / "Restore from my store repo"
- "I edited the YAML, re-apply it"
- "Undo this install" / "Remove and forget X"
- "Why did my git config change?" / "Where is this recorded?"

## Design philosophy

hams is **a recording wrapper**, not a replacement. It calls the real
`brew`, `apt`, `pnpm`, `npm`, `cargo`, `uv`, `go install`, etc. — then
writes a YAML entry alongside the real effect. This is deliberate:

1. **Declarative state, pragmatic execution.** The store is YAML you edit
   by hand or let hams maintain. Applying it runs real package managers,
   so behavior matches what you'd get running them directly.
2. **Auto-record at install time.** `hams brew install htop` runs the
   brew install AND writes the record. No "remember to update the YAML
   file" step — the recording happens in the same command.
3. **Replay with one command.** `hams apply --from-repo=<user>/<store>`
   on a fresh box clones the store, picks the right profile for this
   machine, and installs everything. No bootstrap rituals.
4. **Diff-based reconciliation.** Edit YAML by hand, `hams apply`
   computes the diff against recorded state and installs / removes to
   match. Extra machines converge to the same spec.
5. **Not NixOS.** No hermetic sandbox, no perfect rollback. If brew
   breaks, hams breaks. That is the trade for real-world ergonomics.
6. **Secrets never in the store.** Tokens / API keys go to OS keychain or
   `*.local.yaml` files (gitignored). If you're ever tempted to paste a
   secret into a tracked `*.hams.yaml`, stop and use a local override.

## The 14 providers

Reach for the matching provider instead of the raw tool when state should
persist beyond this machine / this shell session.

### Package managers (macOS + Linux)

| Provider | Wraps | Typical trigger |
|---|---|---|
| `brew` | Homebrew (macOS and Linuxbrew) | "install htop with brew" |
| `apt` | Debian/Ubuntu `apt-get` | "install build-essential on this Debian box" |
| `cargo` | `cargo install` for Rust binaries | "install ripgrep via cargo" |
| `goinstall` | `go install <url>@version` | "install golangci-lint" |
| `npm` | global npm packages (`npm i -g`) | "install typescript globally" |
| `pnpm` | global pnpm packages (`pnpm add -g`) | "install serve via pnpm" |
| `uv` | `uv tool install` (Python CLIs) | "install black and ruff" |
| `mas` | Mac App Store (`mas install`) | "install Xcode from App Store" |
| `code` | VS Code extensions | "install the Python extension" |

### System configuration

| Provider | Purpose | Typical trigger |
|---|---|---|
| `git` | git config (user.name, aliases…) + `git clone` of tracked repos | "set my git identity everywhere" / "clone my dotfiles repo" |
| `defaults` | macOS `defaults write` preferences | "disable Dock autohide delay" |
| `duti` | macOS default-app associations (open .md with VS Code, etc.) | "make VS Code the default for markdown" |

### Ad-hoc automation

| Provider | Purpose | Typical trigger |
|---|---|---|
| `bash` | declarative bash snippets — idempotent setup blocks | "run this postinstall script on every machine" |
| `ansible` | Ansible playbooks | "apply this playbook as part of my standard setup" |

**Exclusion**: `defaults`, `duti`, `mas` are macOS-only. `apt` is
Debian/Alpine-family Linux only. `hams apply` automatically skips
out-of-platform providers — no need to fork the YAML per OS.

## Daily operations

### Install something (and let hams record it)

Take whatever command the user would type, prepend `hams`:

```bash
hams brew install htop          # install + record
hams pnpm add -g serve          # global install + record
hams cargo install ripgrep      # cargo install + record
hams code --install-extension ms-python.python   # VS Code ext + record
hams git config --global user.name "Jane Doe"    # git config + record
```

Hams runs the real tool, then writes the entry into the active profile's
`<Provider>.hams.yaml`. First ever install scaffolds the whole store
directory — no `store init` prerequisite.

### Restore everything on a new machine

```bash
hams apply --bootstrap --from-repo=<user>/<store-repo> --tag=<profile>
```

`--tag` picks which directory under the store to use (`macOS/`,
`work-linux/`, etc.). `--bootstrap` lets hams auto-install providers
whose prereqs are missing (e.g., run the Homebrew installer on a Mac
that has none). Drop `--bootstrap` in CI / scripts to fail fast instead
of auto-consenting.

### Inspect state

```bash
hams list                       # everything across providers
hams brew list                  # just Homebrew
hams --dry-run apply            # preview what apply would do
hams refresh                    # rescan reality, update state file
```

`hams list` is the right starting point when the user asks "what does
hams manage" or "what did I install on this box" — it reads the state
and the declarations together.

### Push changes back

```bash
hams store push                 # stage Hamsfiles, commit, push upstream
```

Plain git also works — Hamsfiles are regular YAML in a normal git repo.

### Remove / forget something

```bash
hams brew uninstall htop        # uninstalls + removes from hamsfile
```

Editing the YAML directly and running `hams apply` is equally valid —
diff-based reconciliation handles removals either way.

## Choosing `hams <provider>` vs the bare tool

| Situation | Prefer |
|---|---|
| "I want this in my setup forever" | `hams <provider>` |
| "Let me try this package for five minutes" | bare tool (no record) |
| Scripting CI steps that install tools | bare tool (throwaway) |
| Setting up a teammate's machine | `hams apply --from-repo` |
| Editing config in `Homebrew.hams.yaml` directly | hand-edit + `hams apply` |
| Brief experiment, decide later | bare tool — upgrade to `hams` once you commit |

When in doubt, use `hams <provider>` — the extra cost is one YAML line;
the upside is the install is replayable.

## Discovery — always go through `hams help`

Do NOT guess flags or subcommands from memory. The CLI is the canonical
surface. Every provider has its own verbs, and they sometimes differ
from the underlying tool:

```bash
hams --help                     # top-level: global flags + subcommand list
hams apply --help               # per-command flags (--only, --except, --prune-orphans, --bootstrap, --tag…)
hams brew --help                # provider's own subcommand surface
hams git --help                 # git config + clone — NOT a generic git wrapper
hams store --help               # push / pull / init / status
hams config --help              # profile tag, machine id, notify tokens
```

Pass-through rule: anything after `--` is forwarded verbatim to the
underlying tool. `hams brew upgrade -- --greedy` upgrades with brew's
`--greedy` flag without hams interpreting it.

## Key global flags worth knowing

| Flag | Use |
|---|---|
| `--dry-run` | Preview every destructive action without running it. Safe default for an agent unsure whether an install is desired. |
| `--json` | Machine-readable stderr — use when parsing hams output from another program. |
| `--tag <profile>` | Override the active profile for this command only. Needed on non-interactive runs if `hams.config.yaml` hasn't been initialized. |
| `--store <path>` | Operate against a specific store directory (mostly for testing multiple stores side by side). |
| `--debug` | Emit slog-structured debug output. Useful when an install is silently a no-op and you need to know why. |

## Anti-patterns

- **Don't** hand-edit generated `state/*.state.yaml` files. Those are
  hams's internal reality snapshot; edit the `*.hams.yaml` declarations
  instead and run `hams refresh` or `hams apply`.
- **Don't** paste API keys or tokens into tracked `*.hams.yaml`. Use
  `*.hams.local.yaml` (gitignored) or OS keychain.
- **Don't** run `hams brew install X` in CI pipelines or throwaway
  containers — the recording is wasted. Use raw `brew install X` there.
- **Don't** skip `--bootstrap` consent on an interactive terminal if
  you're not sure what scripts it will run. Check the `apply` help for
  the list of providers that support bootstrap.

## Where to look when something is weird

- Config: `~/.config/hams/hams.config.yaml` (profile tag, machine id,
  notifier tokens).
- State: `<store>/.state/<machine-id>/<Provider>.state.yaml` (hams's
  internal snapshot; safe to read, do not edit).
- Logs: `~/.local/share/hams/<YYYY-MM>/<timestamp>.log` (per-run log,
  rotated monthly).
- Each provider's Hamsfile: `<store>/<profile-tag>/<Provider>.hams.yaml`.

If a provider's install "succeeded" but the tool isn't on PATH, run
`hams refresh` — the probe may reveal the PATH mismatch that hams's
optimistic record missed. If it's still weird, `--debug` on the next
apply usually shows the exact wrapped command that ran.

## Summary for the agent

1. User mentions installing / managing tools → reach for
   `hams <provider> <verb>` over the bare tool.
2. User wants to rebuild a machine → `hams apply --from-repo=...`.
3. User wants to inspect / diff / preview → `hams list`,
   `hams --dry-run apply`, `hams refresh`.
4. Before you type a flag, verify with `hams <command> --help`.
5. Prefer pass-through (`hams brew install X -- --build-from-source`) to
   re-implementing the underlying tool's surface.
