# Builtin Providers Spec

**Status**: Draft
**Change**: hams-v1-design
**Depends on**: Provider System, Schema Design, CLI Architecture

## Context

hams ships 15 builtin providers compiled into the Go binary. Each provider wraps an existing CLI tool or system facility to install, configure, probe, and remove resources. This spec defines the individual design for every builtin provider: its Hamsfile schema, probe strategy, apply/remove flow, CLI wrapping, auto-inject flags, depend-on declarations, LLM enrichment, and idempotency guarantees.

All providers implement the Provider interface defined in the Provider System spec. Resource classes (Package, KV Config, Check-based, Filesystem) are defined in AGENTS.md Q4. URN semantics are defined in AGENTS.md Q2.

### Provider Classification Summary

| # | Provider | Display Name | Resource Class | Identity | Platform |
|---|----------|-------------|----------------|----------|----------|
| 1 | bash | Bash | Check-based | URN | both |
| 2 | homebrew | Homebrew | Package | natural name | macOS |
| 3 | apt | apt | Package | natural name | Linux |
| 4 | pnpm | pnpm | Package | natural name | both |
| 5 | npm | npm | Package | natural name | both |
| 6 | uv | uv | Package | natural name | both |
| 7 | go | Go | Package | natural name (module path) | both |
| 8 | cargo | Cargo | Package | natural name | both |
| 9 | vscode-ext | VSCode Extension | Package | extension ID | both |
| 10 | git-config | git config | KV Config | URN | both |
| 11 | git-clone | git clone | Filesystem | URN | both |
| 12 | defaults | defaults | KV Config | URN | macOS |
| 13 | duti | duti | KV Config | URN | macOS |
| 14 | mas | mas | Package | numeric app ID | macOS |
| 15 | ansible | Ansible | Check-based | URN | both |

---

## Common Patterns

The following patterns apply across multiple providers. Individual provider sections reference these by name and only document deviations.

### CP-1: Package Provider Common Pattern

All Package-class providers (homebrew, apt, pnpm, npm, uv, go, cargo, vscode-ext, mas) share these behaviors:

**Hamsfile entry structure:**

```yaml
apps:
  - app: <package-name>
    tags: [<tag1>, <tag2>]
    intro: "<one-line description>"
    # Optional fields:
    registry: "<custom-registry-url>"  # e.g., https://registry.npmmirror.com for npm/pnpm
    hooks:
      pre-install: "<command>"
      post-install: "<command>"
      pre-update: "<command>"
      post-update: "<command>"
```

- **`registry:`** — optional custom registry URL for this provider. Used by npm/pnpm/uv/cargo providers when the default registry should be overridden (e.g., for mirrors or private registries).

**Probe**: Run the provider's native list command, parse output into `{name, version}` pairs, compare against state.

**Apply**: For each entry in Hamsfile not present in state (or state = `failed`/`pending`), run the provider's install command. Write state on success.

**Remove**: Delete entry from Hamsfile, run the provider's uninstall command, mark state as `removed`.

**LLM enrichment**: Provider queries the package's native description (if available), combines with existing tags from Hamsfile, sends to LLM for tag recommendation and intro generation. Accessible via `hams <provider> enrich <app>`.

**CLI wrapping**: Provider recognizes `install`, `remove`, and `list` subcommands. All other subcommands are passthrough to the underlying CLI. `list` shows diff between Hamsfile and state (not raw provider output).

**No confirmation**: Package providers do NOT require TTY confirmation for install or remove operations.

### CP-2: KV Config Provider Common Pattern

All KV Config-class providers (git-config, defaults, duti) share:

**Hamsfile entry structure:**

```yaml
configs:
  - urn: "urn:hams:<provider>:<resource-id>"
    args:
      # Provider-specific structured key/value fields
      domain: com.apple.dock
      key: autohide
      type: bool
      value: true
    preview-cmd: "defaults write com.apple.dock autohide -bool true"
    check: "defaults read com.apple.dock autohide"
    tags: [<tag1>]
    intro: "<description>"
```

- **`args:`** — structured key-value representation of the command parameters. Used for actual execution (parsed by the provider).
- **`preview-cmd:`** — the original shell command as a human-readable string for review purposes. NOT directly executed; serves as documentation alongside the structured `args:`.
- **`check:`** — command to verify current state (exit code 0 = already applied). Required for idempotency unless the write command itself is idempotent.

**Probe**: Read-back the value using the `check:` command or provider's native query. Compare against `args:` desired value.

**Apply**: Execute using parsed `args:`, not `preview-cmd:`. Only write if current value differs from desired.

**Remove**: Delete/unset the value using the provider's unset command. Mark state as `removed`.

**Identity**: URN-based. The URN encodes enough information to uniquely identify the config entry.

### CP-3: Check-based Provider Common Pattern

Check-based providers (bash, ansible) share:

**Hamsfile entry structure:**

```yaml
steps:
  - urn: "urn:hams:<provider>:<resource-id>"
    step: "<human-readable step name>"
    description: "<what this step does>"
    check: "<command that returns exit 0 if already applied>"
    # Provider-specific apply fields
```

**Probe**: Execute the `check` command. Exit code 0 = already applied (state `ok`). Non-zero = needs apply. If no `check` field, the step always runs (not idempotent).

**Apply**: Execute the provider's apply command. Then re-run `check` to verify.

**Remove**: Provider-specific. Bash scripts may define a `remove` command; Ansible uses its own idempotency.

---

## ADDED Requirements

---

### Requirement: Bash Provider

The Bash provider SHALL be the escape-hatch provider for arbitrary shell scripts that do not fit any typed provider. It uses URN-based identity and requires a `check:` field for idempotency verification.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `bash` |
| Display name | `Bash` |
| File | `Bash.hams.yaml` |
| Resource class | Check-based |
| Platform | both (macOS + Linux) |
| depend-on | none |
| Priority | 1 (highest, runs first) |

**Hamsfile schema (`Bash.hams.yaml`):**

```yaml
steps:
  - urn: "urn:hams:bash:install-homebrew"
    step: "Install Homebrew"
    description: "Install Homebrew package manager via official script"
    check: "command -v brew"
    run: |
      /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    sudo: false
    tags: [bootstrap]
    hooks:
      post-install: "eval \"$(/opt/homebrew/bin/brew shellenv)\""

  - urn: "urn:hams:bash:set-hostname"
    step: "Set hostname"
    description: "Set macOS hostname via scutil"
    check: "test \"$(scutil --get HostName)\" = 'MacbookM2X'"
    run: "sudo scutil --set HostName MacbookM2X"
    sudo: true
    tags: [sys-pref]

  - urn: "urn:hams:bash:install-jovial"
    step: "Install jovial zsh theme"
    description: "Install jovial zsh theme from GitHub"
    check: "test -f ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/themes/jovial.zsh-theme"
    run: "scripts/install-jovial.sh"
    sudo: true
    tags: [terminal]
```

**Script file convention**: When the `run:` field references a relative path, it SHALL resolve relative to a `bash.hams/` subdirectory alongside the Hamsfile. Example directory layout:

```
macOS/
  Bash.hams.yaml
  bash.hams/
    scripts/
      install-jovial.sh
```

**Fields:**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `urn` | YES | string | `urn:hams:bash:<id>`. Unique identifier. |
| `step` | YES | string | Human-readable step name (replaces `app`). |
| `description` | NO | string | What this step does. |
| `check` | NO | string | Shell command. Exit 0 = already applied. |
| `run` | YES | string | Shell command or script path to execute. |
| `remove` | NO | string | Shell command to reverse the step. |
| `sudo` | NO | bool | Whether `run` needs sudo elevation. Default `false`. |
| `tags` | NO | list | Category tags. |
| `hooks` | NO | object | `pre-install`, `post-install` hooks. |

**CLI wrapping:**

- `hams bash run <urn-id>` -- execute a single step by URN suffix.
- `hams bash list` -- show all steps with status from state.
- All other subcommands after `bash` are NOT passthrough (bash is not a wrapped CLI).

#### Scenario: Execute a bash step with check passing

WHEN a Bash step has `check: "command -v brew"` and `brew` is on `$PATH`
THEN the Bash provider SHALL skip execution of `run:`, mark state as `ok`, and log "already satisfied."

#### Scenario: Execute a bash step with check failing

WHEN a Bash step has `check: "command -v brew"` and `brew` is NOT on `$PATH`
THEN the Bash provider SHALL execute the `run:` command, re-run `check`, and mark state as `ok` on success or `failed` on failure.

#### Scenario: Bash step without check field

WHEN a Bash step omits the `check:` field
THEN the Bash provider SHALL always execute `run:` on every apply, and mark state as `ok` if exit code is 0, `failed` otherwise. The provider SHALL log a warning that the step is not idempotent.

#### Scenario: Bash step with sudo elevation

WHEN a Bash step has `sudo: true`
THEN the Bash provider SHALL prepend `sudo` to the `run:` command using the cached sudo credentials from the apply session.

#### Scenario: Relative script path resolution

WHEN the `run:` field contains a relative path (e.g., `scripts/install-jovial.sh`)
THEN the Bash provider SHALL resolve it relative to `<profile-dir>/bash.hams/` and SHALL verify the file exists before execution. If the file does not exist, the step SHALL be marked `failed` with a descriptive error.

#### Scenario: Remove a bash step

WHEN a Bash step defines a `remove:` field and the user runs `hams bash remove <urn-id>`
THEN the Bash provider SHALL execute the `remove:` command, delete the entry from the Hamsfile, and mark state as `removed`.

#### Scenario: Remove without remove field

WHEN a Bash step does NOT define a `remove:` field and the user runs `hams bash remove <urn-id>`
THEN the Bash provider SHALL delete the entry from the Hamsfile, mark state as `removed`, and log a warning that no undo command was executed.

---

### Requirement: Homebrew Provider

The Homebrew provider SHALL wrap `brew install`, `brew uninstall`, and related commands. It SHALL handle core formulae, casks, and taps in a single Hamsfile. It depends on the Bash provider for self-installation.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `homebrew` |
| Display name | `Homebrew` |
| File | `Homebrew.hams.yaml` |
| Resource class | Package |
| Platform | macOS (Linux Homebrew is supported but secondary) |
| depend-on | `bash` (for Homebrew self-install script) |
| Priority | 2 |

**depend-on bootstrap**: When Homebrew is not installed (`command -v brew` fails), hams SHALL invoke the Bash provider to execute the Homebrew install script (`curl | bash`) before proceeding. This dependency is declared in the provider manifest, not in the Hamsfile.

**Hamsfile schema (`Homebrew.hams.yaml`):**

```yaml
apps:
  # Core formula (no special flag)
  - app: git
    tags: [network-tool]
    intro: "Distributed revision control system."

  # Cask (requires --cask flag)
  - app: visual-studio-code
    cask: true
    tags: [editor-ide-app]
    intro: "Open-source code editor."

  # Tap formula (slash in name auto-detects tap)
  - app: oven-sh/bun/bun
    tags: [runtime-environment]
    intro: "Incredibly fast JavaScript runtime."
```

**Fields (extending CP-1):**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `app` | YES | string | Formula or cask name. Tap formulae use `<tap>/<formula>` format. |
| `cask` | NO | bool | If `true`, auto-inject `--cask` flag. Default `false`. |
| `tags` | NO | list | Category tags. |
| `intro` | NO | string | Package description. |
| `hooks` | NO | object | Pre/post-install hooks. |

**Cask detection rules:**

1. If `cask: true` is set explicitly, use `--cask`.
2. If `cask` is not set and the package name contains a slash with 2 segments (e.g., `homebrew/cask-fonts/font-hack-nerd-font`), the first segments are the tap. The provider SHALL auto-tap if needed.
3. The provider SHALL NOT auto-detect cask vs formula by querying Homebrew. The `cask` field is the authoritative source.

**Tap handling**: When an `app` name contains a slash (e.g., `oven-sh/bun/bun`), the provider SHALL extract the tap portion (`oven-sh/bun`) and run `brew tap oven-sh/bun` before installation if the tap is not already tapped. Tap operations are idempotent.

**Probe implementation:**

- Formula: `brew list --formula --versions` -- parse `<name> <version>` lines.
- Cask: `brew list --cask --versions` -- parse `<name> <version>` lines.
- Combined into a single probe pass. Each entry in state records whether it is a cask.

**Apply flow:**

1. For each entry not in state or state != `ok`:
   a. If tap formula, ensure tap is added.
   b. Run `brew install <app>` (or `brew install --cask <app>` if `cask: true`).
   c. On success, update state with version from `brew list --versions <app>`.

**Remove flow:**

1. Delete entry from Hamsfile.
2. Run `brew uninstall <app>` (or `brew uninstall --cask <app>`).
3. Mark state as `removed`.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams brew install <app>` | Install + record to Hamsfile. Tag picker fires. |
| `hams brew remove <app>` | Uninstall + remove from Hamsfile. |
| `hams brew list` | Show diff: Hamsfile vs state vs environment. |
| `hams brew enrich <app>` | LLM-driven tag/intro update for an existing entry. |
| `hams brew upgrade <app>` | Passthrough to `brew upgrade`. Update version in state. |
| `hams brew search <query>` | Passthrough to `brew search`. |
| `hams brew info <app>` | Passthrough to `brew info`. |
| Any other | Passthrough to `brew <subcommand> <args>`. |

**LLM enrichment:**

- Source: `brew info --json=v2 <app>` provides `desc`, `homepage`, `caveats`.
- The `desc` field is used as `intro` if `intro` is empty.
- LLM receives: package name, desc, homepage, existing tags from Hamsfile, and recommends tags.

**Auto-inject flags:**

- `--cask` when `cask: true`.
- No `-y` or `--force` auto-inject (Homebrew does not require confirmation by default).

**Idempotency**: `brew install` on an already-installed formula is a no-op (exit 0 with "already installed" message). The provider relies on state to skip redundant calls.

#### Scenario: Install a core formula

WHEN the user runs `hams brew install git`
THEN the Homebrew provider SHALL execute `brew install git`, record `{app: git}` in `Homebrew.hams.yaml` with tags from the tag picker, and update state to `ok` with the installed version.

#### Scenario: Install a cask

WHEN the user runs `hams brew install visual-studio-code --hams-cask`
THEN the Homebrew provider SHALL execute `brew install --cask visual-studio-code`, record `{app: visual-studio-code, cask: true}` in the Hamsfile, and update state.

#### Scenario: Install a tap formula

WHEN the user runs `hams brew install oven-sh/bun/bun`
THEN the Homebrew provider SHALL run `brew tap oven-sh/bun` (if not already tapped), then `brew install oven-sh/bun/bun`, and record the entry.

#### Scenario: Probe detects drift

WHEN `brew list --formula --versions` does not list a formula that state records as `ok`
THEN the Homebrew provider SHALL update state to `not-present` and, on next apply, re-install the formula.

#### Scenario: Remove a cask

WHEN the user runs `hams brew remove visual-studio-code`
THEN the Homebrew provider SHALL execute `brew uninstall --cask visual-studio-code` (detecting `cask: true` from Hamsfile), remove the entry from the Hamsfile, and mark state as `removed`.

### Requirement: Homebrew self-install bootstrap (opt-in)

The Homebrew provider's `Bootstrap` step SHALL detect a missing `brew`
binary on `$PATH` and, rather than silently executing the declared
`DependsOn[].Script`, SHALL gate the bootstrap execution on **explicit
user consent**. hams SHALL NEVER auto-execute a remote install script
without consent, because:

- Auto-executing `curl | bash` from `raw.githubusercontent.com`
  changes the tool's security posture without the user's knowledge.
- Corporate firewalls commonly block `raw.githubusercontent.com`; a
  silent network failure would be indistinguishable from "brew is
  broken" for the user.
- Homebrew's `install.sh` on macOS can trigger the Xcode CLI Tools GUI
  dialog, which blocks stdin — the installer appears hung while a
  modal dialog waits behind the user's IDE. The user MUST be warned
  about this explicitly before execution.

Consent SHALL be expressible in three ways:

1. **`--bootstrap` flag on `hams apply`** (non-interactive consent).
2. **Affirmative answer to an interactive TTY prompt** shown when
   `--bootstrap` is not set but stdin is a terminal.
3. **`--no-bootstrap` flag** explicitly opts OUT of the prompt (used
   in CI or by users who want fail-fast behavior).

When consent is NOT given (default, non-TTY context, or explicit
`--no-bootstrap`), hams SHALL emit a `UserFacingError` containing the
missing binary name, the exact script that would run, and the
copy-pasteable remedy (`hams apply --bootstrap ...`).

When consent IS given, hams SHALL resolve the
`DependsOn[].Provider` entry (typically `bash`), locate that provider
via the provider registry, delegate execution via the Bash provider's
`RunScript(ctx, script)` boundary (honoring the existing DI seam), and
stream the script's stdout/stderr to the user's terminal. Interactive
prompts from `install.sh` SHALL be forwarded unchanged to the TTY.

Bootstrap failure SHALL be terminal: hams SHALL NOT retry, SHALL
surface the script's exit code + last 50 lines of output, and SHALL
abort the apply run with a non-zero exit code.

#### Scenario: Bootstrap emits actionable error when `--bootstrap` is not set

- **WHEN** the Homebrew provider is needed (after the two-stage filter includes it), `brew` is NOT on `$PATH`, AND the user did NOT pass `--bootstrap`
- **AND** stdin is NOT a TTY (e.g., CI, pipe, script)
- **THEN** hams SHALL emit a `UserFacingError` whose body names the missing binary (`brew`), the exact script text from `Manifest().DependsOn[0].Script`, and the remedy `hams apply --bootstrap <original-args>`
- **AND** hams SHALL exit non-zero WITHOUT executing the script, without touching the network, and without modifying state files.

#### Scenario: Bootstrap runs with `--bootstrap` flag

- **WHEN** the Homebrew provider is needed, `brew` is NOT on `$PATH`, AND the user passed `--bootstrap`
- **THEN** hams SHALL resolve the `DependsOn[0]` entry (`Provider: "bash"`)
- **AND** hams SHALL locate the Bash provider in the registry, delegate the script via `BashScriptRunner.RunScript(ctx, script)`, and stream stdout/stderr to the user's terminal
- **AND** after the script exits 0, hams SHALL re-check `exec.LookPath("brew")`; on success, Homebrew operations SHALL proceed as if brew had been present from the start.

#### Scenario: Bootstrap prompts on TTY without `--bootstrap`

- **WHEN** the Homebrew provider is needed, `brew` is NOT on `$PATH`, the user did NOT pass `--bootstrap`, stdin IS a TTY, AND the user did NOT pass `--no-bootstrap`
- **THEN** hams SHALL display a prompt listing: the missing binary, the exact script to run, its documented side effects (sudo password prompt, Xcode Command Line Tools install, install location), and accept `[y/N/s]` input
- **AND** on `y`, the bootstrap proceeds as in "Bootstrap runs with `--bootstrap` flag"
- **AND** on `N` or EOF, hams SHALL emit the same UserFacingError as the no-TTY case and exit non-zero
- **AND** on `s` (skip-this-provider), hams SHALL skip the Homebrew provider for this run (as if `--except=brew` were set) and continue with other providers.

#### Scenario: Bootstrap failure is terminal

- **WHEN** the bootstrap script exits non-zero OR `brew` is still not on `$PATH` after the script completes
- **THEN** hams SHALL NOT retry
- **AND** hams SHALL surface the script's exit code and the last 50 lines of its stderr
- **AND** hams SHALL abort the apply run with a non-zero exit code
- **AND** state files SHALL NOT be modified (no partial progress recorded).

#### Scenario: `--no-bootstrap` disables the interactive prompt

- **WHEN** the Homebrew provider is needed, `brew` is NOT on `$PATH`, stdin IS a TTY, AND the user passed `--no-bootstrap`
- **THEN** hams SHALL skip the interactive prompt entirely
- **AND** hams SHALL emit the same UserFacingError as the no-TTY case and exit non-zero.

### Requirement: Provider framework executes DependOn.Script on explicit consent

The provider framework SHALL expose a `RunBootstrap(ctx, p, registry)`
function (in `internal/provider/bootstrap.go`) that:

1. Iterates `p.Manifest().DependsOn`, skipping entries whose `Platform`
   doesn't match the current OS.
2. For each entry with a non-empty `Script`, looks up the target
   `Provider` name in the registry.
3. Type-asserts the looked-up provider to a `BashScriptRunner`
   interface: `RunScript(ctx context.Context, script string) error`.
4. Delegates the script execution to that runner, which encapsulates
   the exec boundary (and is DI-injected in unit tests).

The Bash builtin provider (`internal/provider/builtin/bash`) SHALL
implement `BashScriptRunner` by shelling out to `/bin/bash -c <script>`
via its existing command runner.

The CLI layer SHALL thread user consent through
`context.Context` via a `provider.WithBootstrapAllowed(ctx, bool)`
helper, so that `Bootstrap` implementations can query consent without
the CLI reaching into each provider's struct.

#### Scenario: RunBootstrap delegates a registered script

- **GIVEN** a provider manifest with `DependsOn: [{Provider: "bash", Script: "echo hi"}]`
- **AND** the Bash provider is registered
- **WHEN** `RunBootstrap(ctx, p, registry)` is called
- **THEN** the registered Bash provider's `RunScript(ctx, "echo hi")` SHALL be invoked exactly once
- **AND** its error (or nil) SHALL be returned.

#### Scenario: RunBootstrap skips platform-gated entries

- **GIVEN** a provider manifest with `DependsOn: [{Provider: "bash", Script: "...", Platform: "darwin"}]`
- **AND** the current OS is `linux`
- **WHEN** `RunBootstrap(ctx, p, registry)` is called
- **THEN** the Bash runner SHALL NOT be invoked
- **AND** the function SHALL return nil.

#### Scenario: RunBootstrap errors on missing host provider

- **GIVEN** a provider manifest with `DependsOn: [{Provider: "bash", Script: "..."}]`
- **AND** the Bash provider is NOT registered
- **WHEN** `RunBootstrap(ctx, p, registry)` is called
- **THEN** the function SHALL return an error whose message names the missing provider
- **AND** no script SHALL be executed.

---

### Requirement: Homebrew Complete Hamsfile Example

The Homebrew Hamsfile SHALL support organizing packages by tags that mirror real-world usage categories. Below is a complete example derived from the `init-macOS-dev` scripts.

#### Scenario: Complete Hamsfile with all categories

WHEN a user stores their Homebrew packages in `Homebrew.hams.yaml`
THEN the file SHALL support the following structure:

```yaml
# Homebrew.hams.yaml
# macOS development environment - managed by hams

apps:
  # ── Runtime & Language Environments ──
  - app: coreutils
    tags: [runtime-environment, sys-pref-tool]
    intro: "GNU core utilities."
  - app: gcc
    tags: [runtime-environment]
    intro: "GNU compiler collection."
  - app: glib
    tags: [runtime-environment]
    intro: "Core application library for GNOME."
  - app: gnupg
    tags: [runtime-environment]
    intro: "GNU Pretty Good Privacy."
  - app: python
    tags: [runtime-environment]
    intro: "Python programming language."
  - app: ipython
    tags: [runtime-environment]
    intro: "Interactive Python shell."
  - app: node
    tags: [runtime-environment]
    intro: "JavaScript runtime built on V8."
  - app: deno
    tags: [runtime-environment]
    intro: "Secure JavaScript/TypeScript runtime."
  - app: oven-sh/bun/bun
    tags: [runtime-environment]
    intro: "Incredibly fast JavaScript runtime, bundler, transpiler."
  - app: openssl
    tags: [runtime-environment]
    intro: "Cryptography and SSL/TLS toolkit."
  - app: rust
    tags: [runtime-environment]
    intro: "Rust programming language."
  - app: golang
    tags: [runtime-environment]
    intro: "Go programming language."
  - app: tinygo-org/tools/tinygo
    tags: [runtime-environment]
    intro: "Go compiler for microcontrollers and WebAssembly."
  - app: luarocks
    tags: [runtime-environment]
    intro: "Lua package manager."

  # ── System Preference Tools ──
  - app: htop
    tags: [sys-pref-tool]
    intro: "Interactive process viewer."
  - app: duti
    tags: [sys-pref-tool]
    intro: "Select default applications for document types."
  - app: tree
    tags: [sys-pref-tool]
    intro: "Display directories as trees."
  - app: ncdu
    tags: [sys-pref-tool]
    intro: "NCurses disk usage analyzer."
  - app: dua-cli
    tags: [sys-pref-tool]
    intro: "Disk usage analyzer with interactive mode."
  - app: pandoc
    tags: [sys-pref-tool]
    intro: "Universal document converter."
  - app: mas
    tags: [sys-pref-tool]
    intro: "Mac App Store CLI."
  - app: mackup
    tags: [sys-pref-tool]
    intro: "Application settings backup/restore."
  - app: ffmpeg
    tags: [sys-pref-tool]
    intro: "Multimedia framework for audio/video processing."
  - app: gdu
    tags: [sys-pref-tool]
    intro: "Fast disk usage analyzer with console interface."

  # ── Terminal Tools ──
  - app: screen
    tags: [terminal-tool]
    intro: "Terminal multiplexer."
  - app: tmux
    tags: [terminal-tool]
    intro: "Terminal multiplexer with sessions."
  - app: gnu-sed
    tags: [terminal-tool]
    intro: "GNU stream editor."
  - app: terminal-notifier
    tags: [terminal-tool]
    intro: "Send macOS notifications from the terminal."
  - app: source-highlight
    tags: [terminal-tool]
    intro: "Convert source code to syntax-highlighted documents."
  - app: autojump
    tags: [terminal-tool]
    intro: "Shell extension to jump to frequently used directories."
  - app: colordiff
    tags: [terminal-tool]
    intro: "Colorized diff output."
  - app: expect
    tags: [terminal-tool]
    intro: "Automate interactive applications."
  - app: zsh
    tags: [terminal-tool]
    intro: "UNIX shell (command interpreter)."
  - app: asciinema
    tags: [terminal-tool]
    intro: "Record and share terminal sessions."
  - app: ranger
    tags: [terminal-tool]
    intro: "Visual file manager in the terminal."
  - app: lazygit
    tags: [terminal-tool]
    intro: "Simple terminal UI for git commands."
  - app: neovim
    tags: [terminal-tool]
    intro: "Ambitious Vim-fork focused on extensibility."
  - app: smudge/smudge/nightlight
    tags: [terminal-tool]
    intro: "CLI for macOS Night Shift."
  - app: neofetch
    tags: [terminal-tool]
    intro: "System information tool."
  - app: onefetch
    tags: [terminal-tool]
    intro: "Git repository summary in the terminal."
  - app: ripgrep
    tags: [terminal-tool]
    intro: "Recursively search directories for a regex pattern."
  - app: typst
    tags: [terminal-tool]
    intro: "Markup-based typesetting system."
  - app: trzsz-ssh
    tags: [terminal-tool]
    intro: "SSH client with trzsz file transfer support."
  - app: watch
    tags: [terminal-tool]
    intro: "Execute a program periodically, showing output."
  - app: jless
    tags: [terminal-tool]
    intro: "JSON viewer for the command line."
  - app: lnav
    tags: [terminal-tool]
    intro: "Log file navigator."
  - app: librsvg
    tags: [terminal-tool]
    intro: "SVG rendering library."
  - app: imagemagick
    tags: [terminal-tool]
    intro: "Image manipulation tools."
  - app: clol
    tags: [terminal-tool]
    intro: "Colorful git log."

  # ── Network Tools ──
  - app: git
    tags: [network-tool]
    intro: "Distributed revision control system."
  - app: git-lfs
    tags: [network-tool]
    intro: "Git extension for large file storage."
  - app: curl
    tags: [network-tool]
    intro: "Transfer data with URLs."
  - app: wget
    tags: [network-tool]
    intro: "Internet file retriever."
  - app: aria2
    tags: [network-tool]
    intro: "Lightweight multi-protocol download utility."
  - app: laggardkernel/tap/iterm2-zmodem
    tags: [network-tool]
    intro: "iTerm2 integration for zmodem file transfer."
  - app: telnet
    tags: [network-tool]
    intro: "Telnet client."
  - app: mosh
    tags: [network-tool]
    intro: "Mobile shell with roaming and intelligent local echo."
  - app: httpie
    tags: [network-tool]
    intro: "User-friendly HTTP client."
  - app: rs/tap/curlie
    tags: [network-tool]
    intro: "Power of curl with the ease of httpie."
  - app: iproute2mac
    tags: [network-tool]
    intro: "CLI wrapper for macOS networking (ip command)."
  - app: netcat
    tags: [network-tool]
    intro: "Networking utility for reading/writing network connections."
  - app: socat
    tags: [network-tool]
    intro: "Multipurpose relay for bidirectional data transfer."
  - app: iperf3
    tags: [network-tool]
    intro: "Network throughput measurement tool."
  - app: doggo
    tags: [network-tool]
    intro: "DNS client for the command line."

  # ── AI CLI Tools ──
  - app: claude-code
    tags: [ai-cli]
    intro: "Claude Code CLI for AI-assisted development."
  - app: gemini-cli
    tags: [ai-cli]
    intro: "Gemini CLI for AI interactions."

  # ── Terminal Apps (Casks) ──
  - app: iterm2
    cask: true
    tags: [terminal-app]
    intro: "Terminal emulator for macOS."
  - app: warp
    cask: true
    tags: [terminal-app]
    intro: "Modern Rust-based terminal."

  # ── Network Apps (Casks) ──
  - app: google-chrome
    cask: true
    tags: [network-app]
    intro: "Web browser by Google."
  - app: teamviewer
    cask: true
    tags: [network-app]
    intro: "Remote desktop access."
  - app: rapidapi
    cask: true
    tags: [network-app]
    intro: "API development platform."
  - app: postman
    cask: true
    tags: [network-app]
    intro: "API platform for building and testing APIs."
  - app: wireshark
    cask: true
    tags: [network-app]
    intro: "Network protocol analyzer."
  - app: netspot
    cask: true
    tags: [network-app]
    intro: "WiFi survey and analysis tool."
  - app: tailscale
    cask: true
    tags: [network-app]
    intro: "Mesh VPN built on WireGuard."
  - app: sfm
    cask: true
    tags: [network-app]
    intro: "Sing-box client."

  # ── Database Manager (Casks) ──
  - app: datagrip
    cask: true
    tags: [database-manager]
    intro: "JetBrains database IDE."

  # ── Editor & IDE Apps (Casks) ──
  - app: visual-studio-code
    cask: true
    tags: [editor-ide-app]
    intro: "Open-source code editor by Microsoft."
  - app: webstorm
    cask: true
    tags: [editor-ide-app]
    intro: "JetBrains JavaScript IDE."
  - app: pycharm
    cask: true
    tags: [editor-ide-app]
    intro: "JetBrains Python IDE."
  - app: heynote
    cask: true
    tags: [editor-ide-app]
    intro: "Dedicated scratchpad for developers."

  # ── Version Control Apps (Casks) ──
  - app: fork
    cask: true
    tags: [version-control-app]
    intro: "Git client for Mac."

  # ── Dev Utilities (Casks) ──
  - app: orbstack
    cask: true
    tags: [dev-utils-app]
    intro: "Fast Docker & Linux on macOS."

  # ── AI Apps (Casks) ──
  - app: codex
    cask: true
    tags: [ai-app]
    intro: "OpenAI Codex desktop application."

  # ── System Helper & Service Apps (Casks) ──
  - app: raycast
    cask: true
    tags: [system-helper-app]
    intro: "Productivity launcher for macOS."
  - app: istat-menus
    cask: true
    tags: [system-helper-app]
    intro: "System monitor for the menu bar."
  - app: keycastr
    cask: true
    tags: [system-helper-app]
    intro: "Keystroke visualizer."
  - app: snipaste
    cask: true
    tags: [system-helper-app]
    intro: "Screenshot and paste tool."
  - app: ubersicht
    cask: true
    tags: [system-helper-app]
    intro: "Desktop widget platform."
  - app: monitorcontrol
    cask: true
    tags: [system-helper-app]
    intro: "Control external display brightness and volume."
  - app: maczip
    cask: true
    tags: [system-helper-app]
    intro: "Archive utility for macOS."
  - app: jordanbaird-ice
    cask: true
    tags: [system-helper-app]
    intro: "Menu bar manager."
  - app: wacom-tablet
    cask: true
    tags: [system-helper-app]
    intro: "Wacom tablet driver."
  - app: oversight
    cask: true
    tags: [system-helper-app]
    intro: "Monitor camera and microphone usage."
  - app: input-source-pro
    cask: true
    tags: [system-helper-app]
    intro: "Input source switcher per application."
  - app: trex
    cask: true
    tags: [system-helper-app]
    intro: "Text recognition (OCR) tool."
  - app: alt-tab
    cask: true
    tags: [system-helper-app]
    intro: "Windows-style alt-tab window switcher."
  - app: homebrew/cask-fonts/font-hack-nerd-font
    cask: true
    tags: [system-helper-app]
    intro: "Nerd Font patched version of Hack."

  # ── QuickLook Plugins (Casks) ──
  - app: qlvideo
    cask: true
    tags: [quicklook-plugin]
    intro: "QuickLook preview for video files."
  - app: qlstephen
    cask: true
    tags: [quicklook-plugin]
    intro: "QuickLook preview for plain text files."
  - app: qlmarkdown
    cask: true
    tags: [quicklook-plugin]
    intro: "QuickLook preview for Markdown files."
  - app: qlcolorcode
    cask: true
    tags: [quicklook-plugin]
    intro: "QuickLook preview with syntax highlighting."
  - app: qlprettypatch
    cask: true
    tags: [quicklook-plugin]
    intro: "QuickLook preview for patch files."
  - app: quicklook-csv
    cask: true
    tags: [quicklook-plugin]
    intro: "QuickLook preview for CSV files."
  - app: webpquicklook
    cask: true
    tags: [quicklook-plugin]
    intro: "QuickLook preview for WebP images."
  - app: quicklook-json
    cask: true
    tags: [quicklook-plugin]
    intro: "QuickLook preview for JSON files."

  # ── Media & Entertainment (Casks) ──
  - app: neteasemusic
    cask: true
    tags: [media-entertainment-app]
    intro: "NetEase Cloud Music client."
  - app: iina
    cask: true
    tags: [media-entertainment-app]
    intro: "Modern media player for macOS."

  # ── IM & Communication (Casks) ──
  - app: discord
    cask: true
    tags: [im-app, gaming]
    intro: "Voice, video, and text communication."
  - app: telegram
    cask: true
    tags: [im-app]
    intro: "Messaging app with focus on speed and security."

  # ── Office & Productivity (Casks) ──
  - app: obsidian
    cask: true
    tags: [office-app]
    intro: "Knowledge base and note-taking with Markdown."
  - app: microsoft-office
    cask: true
    tags: [office-app]
    intro: "Microsoft Office suite."

  # ── Design Apps (Casks) ──
  - app: sketch
    cask: true
    tags: [design-app]
    intro: "Digital design toolkit."
  - app: figma
    cask: true
    tags: [design-app]
    intro: "Collaborative interface design tool."
  - app: spline
    cask: true
    tags: [design-app]
    intro: "3D design tool for the web."
  - app: blender
    cask: true
    tags: [design-app]
    intro: "Free 3D creation suite."

  # ── Gaming (Casks) ──
  - app: steam
    cask: true
    tags: [gaming]
    intro: "Game distribution platform."
```

---

### Requirement: apt Provider

The apt provider SHALL wrap `apt-get install`, `apt-get remove`, and `dpkg -s` for Debian/Ubuntu-based Linux systems. The CLI handlers SHALL also write `<store>/.state/<machine-id>/apt.state.yaml` directly after each successful install / remove (in addition to mutating the hamsfile), so imperative actions produce a state-file audit trail without requiring a follow-up `hams apply`.

The auto-record path SHALL accept three install-args shapes (bare, version pinned, release pinned). The recording loop SHALL upgrade an existing bare entry to pinned in-place when the user re-runs the install with a pin. Bookkeeping SHALL call the hamsfile's structured-fields helper unconditionally (not just on absent entries) — the helper merges the new fields into the existing mapping node when an entry already exists under any tag.

`Plan` SHALL inspect each declared app's structured fields directly from the hamsfile (not just the app name) so that a hamsfile authored or restored on a fresh machine — including the `apply --from-repo=...` bootstrap path — replays the user's pinned versions on first install. Drift detection (observed dpkg version differs from the requested pin) SHALL emit an Update action whose `ID` remains the bare package name AND whose `Resource` field carries the install-token form (`pkg=version` / `pkg/source`). The executor's `Provider.Apply` SHALL prefer `action.Resource` over `action.ID` when invoking the runner so that state stays keyed on the canonical bare name (no `nginx=1.24.0` orphan rows). Plan SHALL also populate `action.StateOpts` with the `requested_version` / `requested_source` options so the executor records the pin onto the bare-keyed state row.

Dry-run flags (`--download-only`, `--simulate`, `-s`, `--just-print`, `--no-act`, `--recon`) MUST still trigger the "complex invocation: do not record" short-circuit — they are correctly unrecordable because no host state change occurred this invocation.

The `apt` Provider has the same shape as `homebrew`, with Debian/Ubuntu-specific commands:

- **Detect:** `command -v apt-get` and `[ -f /etc/debian_version ]`.
- **Capabilities:** `install`, `remove`, `update`, `upgrade`, `list`, `search`, `show`, `apply`.
- **Flags:**
  - `--no-install-recommends` SHALL be passable through `flags:` per package.
  - `--purge` SHALL be passable for the remove path.
- `sudo` SHALL be auto-injected for all mutating apt commands (apt requires root).

**Command boundary (DI requirement):**

All outbound calls to `apt-get` and `dpkg` SHALL be routed through a dedicated Go interface owned by the apt provider package. The interface SHALL expose at minimum the following methods:

| Method | Real implementation |
|--------|---------------------|
| `Install(ctx, pkg string) error` | `sudo apt-get install -y <pkg>`, streaming stdout/stderr to the user's terminal |
| `Remove(ctx, pkg string) error` | `sudo apt-get remove -y <pkg>`, streaming stdout/stderr |
| `IsInstalled(ctx, pkg string) (installed bool, version string, err error)` | `dpkg -s <pkg>`, parse `Status: install ok installed` line + `Version:` line |

The real implementation SHALL compose with the existing `sudo.CmdBuilder` to acquire root. The interface SHALL be injected via the provider's constructor so that unit tests can substitute an in-memory fake that records call history and maintains a virtual "installed packages" set. Unit tests SHALL NOT shell out to the real `apt-get` or `dpkg` under any circumstance.

**Probe implementation:**

- Iterate resources present in the state file.
- For each resource whose `state != removed`, call `IsInstalled(pkg)` via the command interface.
- Populate `provider.ProbeResult` with `State: ok` + observed `Version` for installed packages, or `State: failed` for any package that is absent or whose `dpkg -s` invocation errors.

**Apply flow (executor path):**

1. `sudo apt-get update` (once per apply session, not per package).
2. For each missing package, call `runner.Install(pkg)`.

**Remove flow (executor path):**

1. For each removal action, call `runner.Remove(pkg)`.
2. Provider SHALL NOT auto-run `apt autoremove` (user decision).

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams apt install <pkg>` | 1. Invoke `runner.Install(pkg)`. 2. On success, load (or create) `apt.state.yaml`, call `state.SetResource(pkg, StateOK, WithVersion(version))` using version captured from `runner.IsInstalled(pkg)`, and persist via atomic write. 3. Load the effective `apt.hams.yaml` (+ `.local.yaml` override), append `{app: <pkg>}` to the default group if absent, and write the file atomically via the hamsfile SDK. 4. On failure of any `runner.Install` call, return the error without modifying the hamsfile or state file (atomic semantics preserved). |
| `hams apt remove <pkg>` | 1. Invoke `runner.Remove(pkg)`. 2. On success, load (or create) `apt.state.yaml`, call `state.SetResource(pkg, StateRemoved)`, and persist via atomic write. 3. Load the effective `apt.hams.yaml`, remove the `{app: <pkg>}` entry via the hamsfile SDK's `RemoveItem` method, and write atomically. Missing entry SHALL be a silent no-op on the hamsfile side. 4. On failure of any `runner.Remove` call, return the error without modifying the hamsfile or state file. |
| `hams apt list` | Diff view (`FormatDiff` of desired vs observed). |
| `hams apt search <query>` | Passthrough to `apt search`. |
| `hams apt show <pkg>` | Passthrough to `apt show`. |
| Any other verb | Passthrough to `apt-get <verb> <args>`. |

**Stdout/stderr policy:**

All commands invoked through the `CmdRunner` interface's real implementation (both Apply/Remove executor paths AND the `hams apt install`/`remove` CLI paths) SHALL stream stdout and stderr to the user's terminal in real time. Stdout/stderr SHALL NOT be silenced, redirected to `io.Discard`, or buffered. Dry-run mode (`--dry-run` flag) SHALL print the equivalent command to stdout without executing it and SHALL NOT invoke the `CmdRunner`, the hamsfile, or the state file.

**State ownership:**

The apt provider's CLI handlers (`hams apt install`, `hams apt remove`) SHALL load and atomically persist `<store>/.state/<machine-id>/apt.state.yaml` after each successful `runner.Install` / `runner.Remove` invocation. State writes from the CLI handlers SHALL go through `state.SetResource(...)` (with `state.WithVersion(...)` for installs) so timestamps (`first_install_at`, `updated_at`, `removed_at`) follow the same lifecycle rules the executor uses. The executor (`provider.Executor`) retains state-write authority for the declarative `hams apply` path. Both writers reuse the same atomic-write helper to avoid partial-state on crash.

**Complex invocation scope (auto-record contract):**

The auto-record path (CLI mutation of hamsfile + state) covers ONLY the bare-name install/remove syntax: `hams apt install <pkg1> <pkg2> ...` and `hams apt remove <pkg1> <pkg2> ...`. Apt-get grammar extensions and dry-run flags fall outside this contract:

- **Version pinning** — any token containing `=` (e.g., `nginx=1.24.0`).
- **Release pinning** — any token containing `/` (e.g., `nginx/bookworm-backports`).
- **Dry-run flags** — `--download-only`, `--simulate`, `-s`, `--just-print`, `--no-act`, `--recon`.

A "complex invocation" SHALL still execute apt-get (passthrough is preserved — flags reach the real package manager) but SHALL NOT mutate the hamsfile or state. The CLI handler SHALL emit a warning log naming the invocation and pointing the user at the declarative path: edit the hamsfile and run `hams apply`. This boundary keeps hams from silently recording phantom packages from `-o KEY=VAL` flag values, from misclassifying version-pinned tokens as raw package names, or from recording packages that were already installed when a dry-run flag short-circuited the actual install.

Future grammar-aware recording (e.g., serialising version pins as a structured `{app: nginx, version: '1.24.0'}` hamsfile entry) requires hamsfile schema extensions and is tracked in the deferred openspec proposal `apt-cli-complex-invocations`.

**LLM enrichment:**

- Source: `apt show <package>` provides `Description`, `Homepage`.

#### Scenario: Install an apt package updates hamsfile and state

- **WHEN** the user runs `hams apt install htop` on a Debian system where `htop` is not yet in `apt.hams.yaml` and not in `apt.state.yaml`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, "htop")` which executes `sudo apt-get install -y htop` with stdout/stderr streaming to the user's terminal
- **AND** on success, SHALL append `{app: htop}` to `apt.hams.yaml` via the hamsfile SDK
- **AND** on success, SHALL write `apt.state.yaml` with `resources.htop.state = ok`, `first_install_at` set to the current timestamp, `updated_at` equal to `first_install_at`, no `removed_at` key, and `version` populated from `runner.IsInstalled(ctx, "htop")`.

#### Scenario: Install command failure leaves hamsfile and state untouched

- **WHEN** the user runs `hams apt install nonexistent-pkg-xyz`
- **AND** `runner.Install` returns an error (exit code non-zero from `apt-get`)
- **THEN** the apt provider SHALL return the error to the user
- **AND** `apt.hams.yaml` SHALL NOT contain a new entry for `nonexistent-pkg-xyz`
- **AND** `apt.state.yaml` SHALL NOT contain a new entry for `nonexistent-pkg-xyz`.

#### Scenario: Re-install bumps updated_at and preserves first_install_at

- **WHEN** the user runs `hams apt install htop` and `htop` is already present in `apt.state.yaml` with `state: ok`, `first_install_at: T0`
- **THEN** the apt provider SHALL still invoke `runner.Install(ctx, "htop")` (apt-get itself is idempotent)
- **AND** SHALL NOT create a duplicate entry for `htop` in `apt.hams.yaml` — the hamsfile SHALL contain exactly one `{app: htop}` entry after the command completes
- **AND** SHALL update `apt.state.yaml` so `resources.htop.first_install_at = T0` (immutable), `updated_at` equals the new timestamp, and `state = ok`.

#### Scenario: Remove an apt package updates hamsfile and state

- **WHEN** the user runs `hams apt remove htop` on a Debian system where `htop` is present in `apt.hams.yaml` with `first_install_at: T0`
- **THEN** the apt provider SHALL invoke `runner.Remove(ctx, "htop")` which executes `sudo apt-get remove -y htop` with stdout/stderr streaming
- **AND** on success, SHALL remove the `{app: htop}` entry from `apt.hams.yaml` via the hamsfile SDK
- **AND** on success, SHALL update `apt.state.yaml` so `resources.htop.state = removed`, `first_install_at = T0` (preserved), `removed_at` set to the current timestamp, and `updated_at` equal to `removed_at`.

#### Scenario: Remove command failure leaves hamsfile and state untouched

- **WHEN** the user runs `hams apt remove htop`
- **AND** `runner.Remove` returns an error (exit code non-zero from `apt-get`)
- **THEN** the apt provider SHALL return the error to the user
- **AND** `apt.hams.yaml` SHALL still contain the `{app: htop}` entry
- **AND** `apt.state.yaml` SHALL retain the previous resource entry for `htop` unchanged.

#### Scenario: Remove of absent hamsfile entry is a no-op on the file

- **WHEN** the user runs `hams apt remove htop` and `htop` is NOT present in `apt.hams.yaml`
- **AND** `runner.Remove` succeeds (apt-get is idempotent: removing an already-removed package returns 0)
- **THEN** the apt provider SHALL complete successfully without modifying `apt.hams.yaml` and without error
- **AND** SHALL still record `state: removed` for `htop` in `apt.state.yaml` so the audit trail is complete.

#### Scenario: Re-install after remove clears removed_at

- **WHEN** the user runs `hams apt install htop` and `htop` is currently in `apt.state.yaml` with `state: removed`, `first_install_at: T0`, `removed_at: T1`
- **THEN** the apt provider SHALL transition `apt.state.yaml` to `state: ok`, `first_install_at: T0` (preserved), `updated_at: T2` (current time), no `removed_at` key (cleared via YAML omitempty).

#### Scenario: Stdout and stderr are not silenced

- **WHEN** the user runs `hams apt install htop`
- **THEN** all output from `sudo apt-get install -y htop` (progress lines, "Setting up htop (...)" messages, errors) SHALL appear on the user's terminal in real time
- **AND** SHALL NOT be buffered, discarded, or captured by hams.

#### Scenario: Dry-run does not invoke the command runner or touch state

- **WHEN** the user runs `hams apt install htop --dry-run` (or an equivalent global `--dry-run` flag)
- **THEN** the apt provider SHALL print the equivalent command (`[dry-run] Would install: sudo apt-get install -y htop`) to stdout
- **AND** SHALL NOT call `runner.Install`
- **AND** SHALL NOT modify `apt.hams.yaml`
- **AND** SHALL NOT load or modify `apt.state.yaml`.

#### Scenario: Probe apt packages via CmdRunner

- **WHEN** the apt provider runs probe for a state file containing resources `htop` and `jq`, both in `state: ok`
- **THEN** the provider SHALL call `runner.IsInstalled(ctx, "htop")` and `runner.IsInstalled(ctx, "jq")`
- **AND** for each installed result, SHALL emit `ProbeResult{ID, State: ok, Version}`; for each uninstalled result, SHALL emit `ProbeResult{ID, State: failed}`.

#### Scenario: Probe skips removed resources

- **WHEN** the apt provider runs probe for a state file where `htop` is in `state: removed`
- **THEN** the provider SHALL NOT call `runner.IsInstalled(ctx, "htop")` — removed resources are excluded from probe iteration.

#### Scenario: apt on macOS

- **WHEN** the apt provider is loaded on macOS
- **THEN** the provider SHALL report itself as `unsupported` for the current platform and SHALL NOT register any commands. The provider SHALL be silently skipped during apply.

#### Scenario: apt update runs once per session

- **WHEN** multiple apt packages need installation during a single `hams apply`
- **THEN** the apt provider SHALL run `sudo apt-get update` exactly once at the beginning of its apply phase, not before each individual package install.

#### Scenario: Unit test with fake CmdRunner detects missing hamsfile + state update

- **WHEN** a unit test injects a fake `CmdRunner` that records every call and invokes `HandleCommand(ctx, ["install", "htop"])`
- **THEN** the test SHALL assert that `runner.Install` was called exactly once with `pkg == "htop"`
- **AND** SHALL assert that `apt.hams.yaml` on the test tempdir now contains `{app: htop}`
- **AND** SHALL assert that `apt.state.yaml` on the test tempdir now contains `resources.htop.state = ok` with `first_install_at` and `updated_at` populated
- **AND** SHALL fail if any assertion fails — the fake does not shell out to real `apt-get`, making this assertion runnable on any developer's machine regardless of OS or privilege level.

#### Scenario: Version-pinned install records structured entry

- **WHEN** the user runs `hams apt install nginx=1.24.0` on a Debian system
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])` so apt-get installs nginx pinned to version 1.24.0
- **AND** on success, SHALL append `{app: nginx, version: "1.24.0"}` to `apt.hams.yaml`
- **AND** on success, SHALL write `apt.state.yaml.resources.nginx` with `state=ok`, `version` populated from `dpkg -s nginx`, AND a `requested_version` field equal to `"1.24.0"`
- **AND** SHALL NOT emit the legacy "complex invocation; not auto-recorded" warning.

#### Scenario: Existing bare entry is upgraded to pinned in-place

- **WHEN** `apt.hams.yaml` already contains a bare entry `{app: nginx}` AND the user runs `hams apt install nginx=1.24.0`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])`
- **AND** SHALL update the existing nginx entry in `apt.hams.yaml` IN PLACE to `{app: nginx, version: "1.24.0"}` (NOT add a duplicate entry, NOT leave the bare entry unchanged)
- **AND** SHALL set `apt.state.yaml.resources.nginx.requested_version = "1.24.0"`.

#### Scenario: Plan replays hamsfile-declared pin on fresh state

- **WHEN** `apt.hams.yaml` declares `{app: nginx, version: "1.24.0"}` AND `apt.state.yaml` has no entry for nginx (fresh machine OR restore path)
- **THEN** `Plan` SHALL emit an Install action with `ID = "nginx"` AND `Resource = "nginx=1.24.0"`
- **AND** Execute SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])`
- **AND** the resulting state row SHALL be keyed on the bare name `nginx` AND carry `requested_version: "1.24.0"`.

#### Scenario: Plan re-installs when host version differs from pin (state-key invariant)

- **WHEN** `apt.hams.yaml` declares `{app: nginx, version: "1.24.0"}` and `apt.state.yaml.resources.nginx` has `version: "1.22.1"` (host was upgraded out of band)
- **THEN** `Plan` SHALL emit an Update action with `ID = "nginx"` AND `Resource = "nginx=1.24.0"`
- **AND** Execute SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])` to re-pin
- **AND** the state file SHALL retain a SINGLE row for nginx, keyed on the bare name (no duplicate `resources["nginx=1.24.0"]` orphan).

#### Scenario: Release-pinned install records structured entry

- **WHEN** the user runs `hams apt install nginx/bookworm-backports`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["nginx/bookworm-backports"])` so apt-get installs nginx from the bookworm-backports release
- **AND** on success, SHALL ensure `apt.hams.yaml` contains `{app: nginx, source: "bookworm-backports"}` (whether by appending a new entry or upgrading an existing bare one in place)
- **AND** on success, SHALL record `apt.state.yaml.resources.nginx` with `state=ok` and the `source` field replicated.

#### Scenario: Dry-run flag still short-circuits auto-record

- **WHEN** the user runs `hams apt install --download-only nginx=1.24.0`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["--download-only", "nginx=1.24.0"])` so apt-get downloads but does not install
- **AND** SHALL emit the "complex invocation; not auto-recorded" warning
- **AND** SHALL NOT mutate `apt.hams.yaml` or `apt.state.yaml`. Dry-run wins over version-pinning recording — the host did not change, so no record is appropriate.

#### Scenario: Benign passthrough flag still auto-records

- **WHEN** the user runs `hams apt install --no-install-recommends htop`
- **THEN** the apt provider SHALL invoke `runner.Install(ctx, ["--no-install-recommends", "htop"])` and continue into the auto-record path
- **AND** SHALL append `{app: htop}` to `apt.hams.yaml`
- **AND** SHALL write `apt.state.yaml.resources.htop.state = ok`. Benign flags (those that do not pin versions, pin releases, or short-circuit installation) preserve the auto-record contract.

---

### Requirement: Hamsfile structured-fields read API

The hamsfile package SHALL expose `(*File).AppFields(appName string) map[string]string` returning the structured per-app fields (e.g., `version`, `source`) for the entry whose `app` value matches `appName`. The `app` and `intro` fields SHALL be omitted from the result; only the optional structured fields are returned. When no entry matches, SHALL return nil.

This API is the read-side counterpart to `AddAppWithFields`: callers that need to consult per-app structured fields (e.g., apt's `Plan` reading version/source pins from a hamsfile) SHALL use this helper rather than re-walking the YAML node tree at every call site.

#### Scenario: AppFields returns recorded structured fields

- **WHEN** the hamsfile contains `{app: nginx, version: "1.24.0", source: "bookworm-backports"}`
- **AND** a caller invokes `f.AppFields("nginx")`
- **THEN** the result SHALL be a map equal to `{"version": "1.24.0", "source": "bookworm-backports"}` (NO `app` key, NO `intro` key).

#### Scenario: AppFields returns nil for unknown apps

- **WHEN** the hamsfile does not contain an entry for `nginx`
- **AND** a caller invokes `f.AppFields("nginx")`
- **THEN** the result SHALL be nil (NOT an empty non-nil map).

#### Scenario: AppFields returns nil for bare entries

- **WHEN** the hamsfile contains `{app: htop}` (no extra fields)
- **AND** a caller invokes `f.AppFields("htop")`
- **THEN** the result SHALL be nil OR an empty map (callers MUST NOT distinguish between the two; both signal "no structured fields recorded").

---

### Requirement: apt resource schema fields for version and release pinning

The hamsfile per-package entry for apt SHALL accept two optional fields beyond `app`:

- `version: "<spec>"` — the version specifier the user wants apt-get to pin. Forwarded verbatim to apt-get as `<app>=<version>` on install.
- `source: "<release>"` — the release/suite the user wants apt-get to install from. Forwarded verbatim to apt-get as `<app>/<release>`.

The state file's per-resource entry SHALL accept the symmetric `requested_version` and `requested_source` fields so refresh/probe can detect host drift away from the pin.

Both new hamsfile fields and both new state fields SHALL be optional and omitempty. Existing bare-name entries SHALL continue to round-trip without modification.

#### Scenario: Bare-name entry round-trips without new fields appearing

- **WHEN** the hamsfile contains `{app: htop}` and is loaded, mutated (e.g., a comment added), and saved
- **THEN** the persisted YAML SHALL still be `{app: htop}` — no spurious `version: ""` or `source: ""` keys SHALL appear.

#### Scenario: Version-pinned entry round-trips through hamsfile + state

- **WHEN** the hamsfile contains `{app: nginx, version: "1.24.0"}` and `hams apply` runs
- **THEN** the executor SHALL invoke `runner.Install(ctx, ["nginx=1.24.0"])`
- **AND** the resulting state row SHALL carry `requested_version: "1.24.0"` AND `version: "<observed>"` populated from `dpkg -s nginx`.

---

### Requirement: pnpm Provider

The pnpm provider SHALL wrap `pnpm add` and `pnpm remove` for globally-installed Node.js packages. It auto-injects the `--global` flag.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `pnpm` |
| Display name | `pnpm` |
| File | `pnpm.hams.yaml` |
| Resource class | Package |
| Platform | both |
| depend-on | `npm` (pnpm can be installed via `npm install -g pnpm`) |
| Priority | 4 |

**Hamsfile schema**: Follows CP-1. Example:

```yaml
apps:
  - app: typescript
    tags: [dev-tool]
    intro: "TypeScript language compiler."
  - app: tsx
    tags: [dev-tool]
    intro: "TypeScript execute - Node.js enhanced to run TypeScript."
  - app: zx
    tags: [dev-tool]
    intro: "Tool for writing better scripts."
  - app: "@typescript/native-preview"
    tags: [dev-tool]
    intro: "Native TypeScript compiler preview."
  - app: git-split-diffs
    tags: [terminal-tool]
    intro: "GitHub-style split diffs in the terminal."
  - app: serve
    tags: [network-tool]
    intro: "Static file serving and directory listing."
  - app: vercel
    tags: [dev-tool]
    intro: "Vercel platform CLI."
```

**Auto-inject flags:**

- `--global` (or `-g`) SHALL be auto-injected on `pnpm add` and `pnpm remove` commands.

**Probe implementation:**

- `pnpm list --global --json` -- parse JSON output for package names and versions.

**Apply flow:**

1. `pnpm add --global <package>` for each missing package.

**Remove flow:**

1. `pnpm remove --global <package>`.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams pnpm add <pkg>` | Install globally + record. Equivalent to `hams pnpm install`. |
| `hams pnpm install <pkg>` | Alias for `add`. MUST have a package name; bare `hams pnpm install` is invalid. |
| `hams pnpm remove <pkg>` | Remove globally + delete from Hamsfile. |
| `hams pnpm list` | Diff view. |
| Any other | Passthrough to `pnpm <subcommand>`. |

**LLM enrichment:**

- Source: `pnpm info <package>` provides `description`, `homepage`.

**Idempotency**: `pnpm add --global` on an already-installed package updates it to latest. The provider SHALL rely on state to skip redundant installs.

#### Scenario: Install a scoped package

WHEN the user runs `hams pnpm add @typescript/native-preview`
THEN the pnpm provider SHALL execute `pnpm add --global @typescript/native-preview`, record `{app: "@typescript/native-preview"}` in the Hamsfile, and update state.

#### Scenario: Bare install is rejected

WHEN the user runs `hams pnpm install` without a package name
THEN the pnpm provider SHALL print an error: "package name required. Use 'hams apply' for bulk install." and exit with a non-zero code.

#### Scenario: pnpm self-install via npm

WHEN pnpm is needed but `command -v pnpm` fails
THEN hams SHALL resolve `depend-on: npm` and use the npm provider to execute `npm install -g pnpm` before proceeding.

---

### Requirement: npm Provider

The npm provider SHALL wrap `npm install` and `npm uninstall` for globally-installed packages. It auto-injects the `--global` flag.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `npm` |
| Display name | `npm` |
| File | `npm.hams.yaml` |
| Resource class | Package |
| Platform | both |
| depend-on | none (npm ships with Node.js; Node.js install is user's responsibility or a Homebrew/apt entry) |
| Priority | 5 |

**Hamsfile schema**: Follows CP-1.

**Auto-inject flags:**

- `--global` (or `-g`) SHALL be auto-injected on `npm install` and `npm uninstall`.

**Probe implementation:**

- `npm list --global --json --depth=0` -- parse JSON `dependencies` object for names and versions.

**Apply flow:**

1. `npm install --global <package>` for each missing package.

**Remove flow:**

1. `npm uninstall --global <package>`.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams npm install <pkg>` | Install globally + record. |
| `hams npm uninstall <pkg>` | Uninstall + delete from Hamsfile. |
| `hams npm remove <pkg>` | Alias for `uninstall`. |
| `hams npm list` | Diff view. |
| Any other | Passthrough to `npm <subcommand>`. |

**LLM enrichment:**

- Source: `npm info <package> --json` provides `description`, `homepage`.

#### Scenario: Install an npm global package

WHEN the user runs `hams npm install node-gyp`
THEN the npm provider SHALL execute `npm install --global node-gyp`, record it in `npm.hams.yaml`, and update state.

#### Scenario: Probe npm global packages

WHEN the npm provider probes
THEN it SHALL execute `npm list --global --json --depth=0`, parse the `dependencies` object, and update state with `{name, version}` pairs.

---

### Requirement: uv Provider

The uv provider SHALL wrap `uv tool install` and `uv tool uninstall` for Python CLI tools.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `uv` |
| Display name | `uv` |
| File | `uv.hams.yaml` |
| Resource class | Package |
| Platform | both |
| depend-on | none (uv is self-contained; install via Homebrew or curl) |
| Priority | 6 |

**Hamsfile schema**: Follows CP-1.

**Probe implementation:**

- `uv tool list` -- parse output lines `<name> v<version>` format.

**Apply flow:**

1. `uv tool install <package>` for each missing package.

**Remove flow:**

1. `uv tool uninstall <package>`.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams uv install <pkg>` | Maps to `uv tool install` + record. |
| `hams uv remove <pkg>` | Maps to `uv tool uninstall` + delete from Hamsfile. |
| `hams uv list` | Diff view. |
| Any other | Passthrough to `uv <subcommand>`. |

**LLM enrichment:**

- Source: `uv pip show <package>` or PyPI API for description.

#### Scenario: Install a uv tool

WHEN the user runs `hams uv install ruff`
THEN the uv provider SHALL execute `uv tool install ruff`, record it in `uv.hams.yaml`, and update state.

#### Scenario: Probe uv tools

WHEN the uv provider probes
THEN it SHALL execute `uv tool list`, parse each line for tool name and version, and update state.

---

### Requirement: Go Provider

The Go provider SHALL wrap `go install` for Go binaries. It requires a version suffix and auto-injects `@latest` if missing.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `go` |
| Display name | `Go` |
| File | `Go.hams.yaml` |
| Resource class | Package |
| Platform | both |
| depend-on | none (Go must be installed via Homebrew/apt or manually) |
| Priority | 7 |

**Hamsfile schema:**

```yaml
apps:
  - app: github.com/golangci/golangci-lint/cmd/golangci-lint
    tags: [dev-tool]
    intro: "Fast Go linters runner."
  - app: golang.org/x/tools/cmd/goimports
    tags: [dev-tool]
    intro: "Updates Go import lines."
```

The `app` field SHALL contain the full Go module path (as used by `go install`).

**Auto-inject flags:**

- `@latest` SHALL be auto-injected if the `app` value does not contain `@`. For example, `github.com/foo/bar` becomes `github.com/foo/bar@latest`.
- If the user specifies a version (`@v1.2.3`), it SHALL be preserved.

**Probe implementation:**

- Go install binaries go to `$GOPATH/bin` or `$GOBIN`. The provider SHALL check for the binary name (last path segment before `@`) in that directory.
- Version detection: `<binary> --version` or `go version -m <binary-path>` to read embedded module info.

**Apply flow:**

1. `go install <module-path>@<version>` for each missing binary.

**Remove flow:**

1. `rm $GOBIN/<binary-name>` (Go has no `go uninstall` command).
2. The provider SHALL warn that removing a go-installed binary only deletes the binary file.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams go install <module>` | Install + record. Auto-inject `@latest`. |
| `hams go remove <module>` | Remove binary + delete from Hamsfile. |
| `hams go list` | Diff view. |
| Any other | Passthrough to `go <subcommand>`. |

**LLM enrichment:**

- Source: Go module proxy (`https://pkg.go.dev/<module>`) for description. Or parse `go doc` output.

#### Scenario: Install with auto-inject @latest

WHEN the user runs `hams go install github.com/golangci/golangci-lint/cmd/golangci-lint`
THEN the Go provider SHALL execute `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` and record the entry.

#### Scenario: Install with explicit version

WHEN the user runs `hams go install github.com/foo/bar@v1.2.3`
THEN the Go provider SHALL execute `go install github.com/foo/bar@v1.2.3` preserving the explicit version and record `app: github.com/foo/bar` with the version in state.

#### Scenario: Remove a go binary

WHEN the user runs `hams go remove github.com/golangci/golangci-lint/cmd/golangci-lint`
THEN the Go provider SHALL delete `$GOBIN/golangci-lint`, remove the entry from the Hamsfile, mark state as `removed`, and log a warning: "Only the binary was removed. Source cache may remain in module cache."

---

### Requirement: Cargo Provider

The Cargo provider SHALL wrap `cargo install` and `cargo uninstall` for Rust binaries.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `cargo` |
| Display name | `Cargo` |
| File | `Cargo.hams.yaml` |
| Resource class | Package |
| Platform | both |
| depend-on | none (Rust/Cargo must be installed via Homebrew/apt or rustup) |
| Priority | 8 |

**Hamsfile schema**: Follows CP-1.

**Probe implementation:**

- `cargo install --list` -- parse output. Each installed crate shows `<name> v<version>:` followed by binary paths.

**Apply flow:**

1. `cargo install <crate>` for each missing crate.

**Remove flow:**

1. `cargo uninstall <crate>`.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams cargo install <crate>` | Install + record. |
| `hams cargo remove <crate>` | Alias for uninstall + delete from Hamsfile. |
| `hams cargo uninstall <crate>` | Uninstall + delete from Hamsfile. |
| `hams cargo list` | Diff view. |
| Any other | Passthrough to `cargo <subcommand>`. |

**LLM enrichment:**

- Source: `cargo info <crate>` or crates.io API for description.

**Idempotency**: `cargo install` on an already-installed crate with same version is a no-op (prints "already installed"). Different version triggers rebuild.

#### Scenario: Install a cargo crate

WHEN the user runs `hams cargo install ripgrep`
THEN the Cargo provider SHALL execute `cargo install ripgrep`, record it in `Cargo.hams.yaml`, and update state.

#### Scenario: Probe cargo crates

WHEN the Cargo provider probes
THEN it SHALL execute `cargo install --list`, parse each `<name> v<version>:` header line, and update state.

---

### Requirement: VSCode Extension Provider

The VSCode Extension provider SHALL wrap `code --install-extension` and `code --uninstall-extension`. It depends on Homebrew for VSCode installation (the `visual-studio-code` cask).

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `vscode-ext` |
| Display name | `VSCode Extension` |
| File | `VSCode Extension.hams.yaml` |
| Resource class | Package |
| Platform | both |
| depend-on | `homebrew` (for `visual-studio-code` cask on macOS) |
| Priority | 9 |

**depend-on note**: On Linux, VSCode may be installed via apt, snap, or flatpak. The depend-on declaration SHALL be platform-conditional: macOS depends on `homebrew`, Linux has no automatic depend-on (user must ensure VSCode is installed).

**Hamsfile schema:**

```yaml
apps:
  - app: ms-python.python
    tags: [language-support]
    intro: "Python language support for VSCode."
  - app: esbenp.prettier-vscode
    tags: [formatter]
    intro: "Code formatter using Prettier."
```

The `app` field SHALL contain the extension ID in `<publisher>.<extension>` format.

**Probe implementation:**

- `code --list-extensions --show-versions` -- parse `<id>@<version>` lines.

**Apply flow:**

1. `code --install-extension <extension-id>` for each missing extension.
2. The `--force` flag is NOT auto-injected (allows user to keep newer versions).

**Remove flow:**

1. `code --uninstall-extension <extension-id>`.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams vscode-ext install <id>` | Install + record. |
| `hams vscode-ext remove <id>` | Uninstall + delete from Hamsfile. |
| `hams vscode-ext list` | Diff view. |
| Any other | Not passthrough (vscode-ext is not a full CLI). |

**LLM enrichment:**

- Source: VSCode Marketplace API (`https://marketplace.visualstudio.com/_apis/public/gallery/extensionquery`) for extension description.

#### Scenario: Install a VSCode extension

WHEN the user runs `hams vscode-ext install ms-python.python`
THEN the provider SHALL execute `code --install-extension ms-python.python`, record it in the Hamsfile, and update state.

#### Scenario: VSCode not installed

WHEN the VSCode Extension provider is needed but `code` is not on `$PATH`
THEN on macOS, hams SHALL resolve `depend-on: homebrew` and check if `visual-studio-code` is in the Homebrew Hamsfile. If not, the provider SHALL report an error: "VSCode (visual-studio-code cask) must be installed via Homebrew before managing extensions."

#### Scenario: Probe extensions

WHEN the VSCode Extension provider probes
THEN it SHALL execute `code --list-extensions --show-versions`, parse `<publisher>.<name>@<version>` lines, and update state.

---

### Requirement: git config Provider

The git config provider SHALL wrap `git config --global` and `git config --file` for managing Git configuration entries. It is a KV Config-class provider.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `git-config` |
| Display name | `git config` |
| File | `git config.hams.yaml` |
| Resource class | KV Config |
| Platform | both |
| depend-on | none |
| Priority | 11 (within the `git` provider group) |

**Hamsfile schema (`git config.hams.yaml`):**

```yaml
configs:
  # Global git configs
  - urn: "urn:hams:git-config:global.init.defaultBranch"
    scope: global
    key: "init.defaultBranch"
    value: "main"
    tags: [git-pref]

  - urn: "urn:hams:git-config:global.user.name"
    scope: global
    key: "user.name"
    value: "zthxxx"
    tags: [git-identity]

  - urn: "urn:hams:git-config:global.user.email"
    scope: global
    key: "user.email"
    value: "zthxxx.me@gmail.com"
    tags: [git-identity]

  - urn: "urn:hams:git-config:global.core.ignorecase"
    scope: global
    key: "core.ignorecase"
    value: "false"
    tags: [git-pref]

  - urn: "urn:hams:git-config:global.core.pager"
    scope: global
    key: "core.pager"
    value: "git-split-diffs --color | less -RFX"
    tags: [git-pref]
    intro: "Use git-split-diffs for side-by-side diff view."

  - urn: "urn:hams:git-config:global.push.default"
    scope: global
    key: "push.default"
    value: "simple"
    tags: [git-pref]

  - urn: "urn:hams:git-config:global.rerere.enabled"
    scope: global
    key: "rerere.enabled"
    value: "true"
    tags: [git-pref]
    intro: "Reuse recorded resolution in git-rebase."

  - urn: "urn:hams:git-config:global.branch.sort"
    scope: global
    key: "branch.sort"
    value: "-committerdate"
    tags: [git-pref]

  # File-scoped git configs (conditional includes)
  - urn: "urn:hams:git-config:file.github-zthxxx.user.name"
    scope: file
    file: "~/.config/git/github.zthxxx"
    key: "user.name"
    value: "zthxxx"
    tags: [git-identity]

  - urn: "urn:hams:git-config:file.github-zthxxx.user.signingKey"
    scope: file
    file: "~/.config/git/github.zthxxx"
    key: "user.signingKey"
    value: "~/.ssh/zthxxx.ed25519"
    tags: [git-identity]

  - urn: "urn:hams:git-config:file.github-zthxxx.commit.gpgsign"
    scope: file
    file: "~/.config/git/github.zthxxx"
    key: "commit.gpgsign"
    value: "true"
    tags: [git-identity]

  # Conditional include (includeIf)
  - urn: "urn:hams:git-config:global.includeIf.github-zthxxx"
    scope: global
    key: "includeIf.hasconfig:remote.*.url:git@github.com:zthxxx/**.path"
    value: "~/.config/git/github.zthxxx"
    tags: [git-identity]
    intro: "Conditional git config for zthxxx GitHub repos."
```

**Fields:**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `urn` | YES | string | `urn:hams:git-config:<scope>.<section>.<key>` |
| `scope` | YES | string | `global` or `file`. |
| `file` | if scope=file | string | Path to git config file. Tilde expansion supported. |
| `key` | YES | string | Git config key (e.g., `user.name`, `core.pager`). |
| `value` | YES | string | Desired value. All values stored as strings. |
| `tags` | NO | list | Category tags. |
| `intro` | NO | string | Description of what this config does. |

**Probe implementation:**

- For `scope: global`: `git config --global --get <key>`.
- For `scope: file`: `git config --file <path> --get <key>`.
- Compare returned value against desired `value`. Match = `ok`, mismatch = `drift`, not found = `not-present`.

**Apply flow:**

1. For `scope: global`: `git config --global <key> <value>`.
2. For `scope: file`: ensure parent directory exists (`mkdir -p`), then `git config --file <path> <key> <value>`.

**Remove flow:**

1. `git config --global --unset <key>` or `git config --file <path> --unset <key>`.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams git-config set <key> <value>` | Set globally + record. |
| `hams git-config set --hams-file=<path> <key> <value>` | Set in file + record. |
| `hams git-config remove <key>` | Unset + delete from Hamsfile. |
| `hams git-config list` | Diff view. |

**LLM enrichment:**

- LLM generates `intro` based on the config key name and value.

#### Scenario: Set a global git config

WHEN the user runs `hams git-config set user.name zthxxx`
THEN the provider SHALL execute `git config --global user.name zthxxx`, record a config entry with URN `urn:hams:git-config:global.user.name` in the Hamsfile, and update state.

#### Scenario: Set a file-scoped git config

WHEN the user runs `hams git-config set --hams-file=~/.config/git/github.zthxxx user.signingKey '~/.ssh/zthxxx.ed25519'`
THEN the provider SHALL create `~/.config/git/` if needed, execute `git config --file ~/.config/git/github.zthxxx user.signingKey '~/.ssh/zthxxx.ed25519'`, and record the entry.

#### Scenario: Probe detects drift in global config

WHEN `git config --global --get core.pager` returns a value different from the Hamsfile's `value` field
THEN the provider SHALL update state to `drift`, and on apply, overwrite with the Hamsfile value.

#### Scenario: Remove a git config entry

WHEN the user runs `hams git-config remove user.name`
THEN the provider SHALL execute `git config --global --unset user.name`, remove the entry from the Hamsfile, and mark state as `removed`.

---

### Requirement: git clone Provider

The git clone provider SHALL record repository clone locations. It is a Filesystem-class provider that tracks remote URL, local path, and default branch. It does NOT track commit hashes or branch state.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `git-clone` |
| Display name | `git clone` |
| File | `git clone.hams.yaml` |
| Resource class | Filesystem |
| Platform | both |
| depend-on | none |
| Priority | 11 (within the `git` provider group) |

**Hamsfile schema (`git clone.hams.yaml`):**

```yaml
repos:
  - urn: "urn:hams:git-clone:github.com/zthxxx/hams"
    remote: "git@github.com:zthxxx/hams.git"
    path: "~/Project/Golang/hams"
    branch: "main"
    tags: [project]
    intro: "hams project repository."

  - urn: "urn:hams:git-clone:github.com/zthxxx/jovial"
    remote: "git@github.com:zthxxx/jovial.git"
    path: "~/Project/OSS/jovial"
    branch: "master"
    tags: [oss]
```

**Fields:**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `urn` | YES | string | `urn:hams:git-clone:<host>/<owner>/<repo>`. |
| `remote` | YES | string | Git remote URL (SSH or HTTPS). |
| `path` | YES | string | Local path to clone to. Tilde expansion supported. |
| `branch` | NO | string | Default branch to check out. Default: repository default. |
| `tags` | NO | list | Category tags. |
| `intro` | NO | string | Repository description. |

**Probe implementation:**

- Check if `path` exists and is a git repository (`test -d <path>/.git`).
- If path exists: state = `ok`. If not: state = `not-present`.
- No version/commit tracking. No `git fetch` or `git pull` during probe.

**Apply flow:**

1. If path does not exist: `git clone <remote> <path>`.
2. If `branch` is specified: `git clone --branch <branch> <remote> <path>`.
3. If path already exists: skip (already cloned). No pull.

**Remove flow:**

1. Delete the entry from the Hamsfile.
2. The provider SHALL NOT delete the local directory (too dangerous). It SHALL log: "Entry removed from Hamsfile. Local directory at <path> was NOT deleted."
3. Mark state as `removed`.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams git-clone add <remote> --hams-path=<path>` | Clone + record. |
| `hams git-clone remove <urn-id>` | Remove from Hamsfile (no directory deletion). |
| `hams git-clone list` | Show all repos with status. |

#### Scenario: Clone a new repository

WHEN the user runs `hams git-clone add git@github.com:zthxxx/hams.git --hams-path=~/Project/Golang/hams`
THEN the provider SHALL execute `git clone git@github.com:zthxxx/hams.git ~/Project/Golang/hams`, record the entry in the Hamsfile, and update state to `ok`.

#### Scenario: Probe an existing clone

WHEN the path `~/Project/Golang/hams` exists and contains a `.git` directory
THEN the git clone provider SHALL mark state as `ok` without running any git commands.

#### Scenario: Probe a missing clone

WHEN the path `~/Project/Golang/hams` does not exist
THEN the git clone provider SHALL mark state as `not-present`, and the next apply SHALL clone the repository.

#### Scenario: Remove does not delete directory

WHEN the user runs `hams git-clone remove github.com/zthxxx/hams`
THEN the provider SHALL remove the entry from the Hamsfile, mark state as `removed`, and SHALL NOT delete `~/Project/Golang/hams`. It SHALL log a warning about the retained directory.

---

### Requirement: defaults Provider

The defaults provider SHALL wrap the macOS `defaults write`, `defaults read`, and `defaults delete` commands for managing application and system preferences. It is macOS-only and supports post-write `killall` hooks for Dock, Finder, and other processes.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `defaults` |
| Display name | `defaults` |
| File | `defaults.hams.yaml` |
| Resource class | KV Config |
| Platform | macOS only |
| depend-on | none |
| Priority | 12 |

**Hamsfile schema (`defaults.hams.yaml`):**

```yaml
configs:
  # ── Dock preferences ──
  - urn: "urn:hams:defaults:com.apple.dock.showhidden"
    domain: "com.apple.dock"
    key: "showhidden"
    type: bool
    value: true
    tags: [dock]
    intro: "Show indicator for hidden applications in the Dock."

  - urn: "urn:hams:defaults:com.apple.dock.mineffect"
    domain: "com.apple.dock"
    key: "mineffect"
    type: string
    value: "suck"
    tags: [dock]
    intro: "Set Dock minimize animation to suck effect."

  - urn: "urn:hams:defaults:com.apple.dock.autohide"
    domain: "com.apple.dock"
    key: "autohide"
    type: bool
    value: true
    tags: [dock]
    intro: "Automatically hide and show the Dock."

  - urn: "urn:hams:defaults:com.apple.dock.autohide-delay"
    domain: "com.apple.dock"
    key: "autohide-delay"
    type: int
    value: 0
    tags: [dock]
    intro: "Remove autohide delay for the Dock."

  - urn: "urn:hams:defaults:com.apple.dock.mru-spaces"
    domain: "com.apple.dock"
    key: "mru-spaces"
    type: bool
    value: false
    tags: [dock]
    intro: "Disable automatic rearranging of Spaces based on most recent use."

  - urn: "urn:hams:defaults:com.apple.dashboard.mcx-disabled"
    domain: "com.apple.dashboard"
    key: "mcx-disabled"
    type: bool
    value: true
    tags: [dock]
    intro: "Disable Dashboard."

  # ── Finder preferences ──
  - urn: "urn:hams:defaults:com.apple.finder.AppleShowAllFiles"
    domain: "com.apple.finder"
    key: "AppleShowAllFiles"
    type: bool
    value: true
    tags: [finder]
    intro: "Show hidden files in Finder."

  - urn: "urn:hams:defaults:com.apple.desktopservices.DSDontWriteNetworkStores"
    domain: "com.apple.desktopservices"
    key: "DSDontWriteNetworkStores"
    type: bool
    value: true
    tags: [finder]
    intro: "Prevent .DS_Store files on network volumes."

  # ── Global preferences ──
  - urn: "urn:hams:defaults:NSGlobalDomain.NSWindowShouldDragOnGesture"
    domain: NSGlobalDomain
    key: "NSWindowShouldDragOnGesture"
    type: bool
    value: true
    tags: [window-management]
    intro: "Enable Cmd+Ctrl+Click to drag any window."

  # ── App preferences (import plist) ──
  - urn: "urn:hams:defaults:com.googlecode.iterm2.import"
    domain: "com.googlecode.iterm2"
    import: "app-preferences/iTerm2.plist"
    tags: [terminal-app]
    intro: "Import iTerm2 preferences from plist."
```

**Fields:**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `urn` | YES | string | `urn:hams:defaults:<domain>.<key>`. |
| `domain` | YES | string | macOS defaults domain (e.g., `com.apple.dock`, `NSGlobalDomain`). |
| `key` | conditional | string | Preference key. Required unless `import` is used. |
| `type` | conditional | string | Value type: `bool`, `string`, `int`, `float`, `array`, `dict`. Required when `key` is set. |
| `value` | conditional | any | Desired value. Type must match `type` field. Required when `key` is set. |
| `import` | NO | string | Path to a `.plist` file for `defaults import`. Mutually exclusive with `key`/`value`. |
| `killall` | NO | string | Process name to `killall` after write (e.g., `Dock`, `Finder`). |
| `tags` | NO | list | Category tags. |
| `intro` | NO | string | Description. |

**Post-write killall**: The provider SHALL support a `killall` field. When set, after writing the defaults value, the provider SHALL execute `killall <process>` to force the process to restart and pick up the new preference. Multiple defaults entries for the same domain/killall target SHALL batch the killall to run once after all entries for that domain are written.

**Probe implementation:**

- `defaults read <domain> <key>` -- compare output against desired `value`.
- Type coercion: `defaults read` returns typed output. The provider SHALL parse and compare by type.
- For `import` entries: no probe (always re-import on apply). Or optionally compare via `defaults export <domain> -` and diff.

**Apply flow:**

1. For `key`/`value` entries: `defaults write <domain> <key> -<type> <value>`.
   - Type flag mapping: `bool` -> `-bool`, `string` -> `-string`, `int` -> `-int`, `float` -> `-float`.
2. For `import` entries: `defaults import <domain> <plist-path>`.
3. After all entries for a domain are written, execute `killall <process>` if any entry in that domain has `killall` set.

**Remove flow:**

1. `defaults delete <domain> <key>`.
2. Execute `killall` if configured.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams defaults write <domain> <key> -<type> <value>` | Write + record. |
| `hams defaults read <domain> <key>` | Passthrough to `defaults read`. |
| `hams defaults remove <urn-id>` | Delete + remove from Hamsfile. |
| `hams defaults list` | Diff view. |
| `hams defaults import <domain> <plist>` | Import + record. |

**Idempotency**: `defaults write` with the same value is idempotent. The provider SHALL skip writes when the probe shows the value already matches, to avoid unnecessary `killall` restarts.

#### Scenario: Write a boolean defaults preference

WHEN the user runs `hams defaults write com.apple.dock autohide -bool true`
THEN the provider SHALL execute `defaults write com.apple.dock autohide -bool true`, record the entry in the Hamsfile, and update state.

#### Scenario: Batch killall for Dock preferences

WHEN multiple defaults entries for `com.apple.dock` are applied during a single apply run and at least one has `killall: Dock`
THEN the provider SHALL execute `killall Dock` exactly once after all Dock entries are written.

#### Scenario: Probe a defaults value

WHEN the provider probes `defaults read com.apple.dock autohide` and the output is `1` (boolean true)
THEN the provider SHALL compare against `value: true` and mark state as `ok`.

#### Scenario: Import a plist

WHEN a defaults entry has `import: "app-preferences/iTerm2.plist"`
THEN the provider SHALL resolve the path relative to `<profile-dir>/` and execute `defaults import com.googlecode.iterm2 <resolved-path>`.

#### Scenario: defaults on Linux

WHEN the defaults provider is loaded on Linux
THEN the provider SHALL report itself as `unsupported` for the current platform and SHALL be silently skipped.

---

### Requirement: duti Provider

The duti provider SHALL wrap the `duti` command for managing default application associations on macOS. It is a KV Config-class provider.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `duti` |
| Display name | `duti` |
| File | `duti.hams.yaml` |
| Resource class | KV Config |
| Platform | macOS only |
| depend-on | `homebrew` (duti is installed via Homebrew) |
| Priority | 13 |

**Hamsfile schema (`duti.hams.yaml`):**

```yaml
configs:
  # Set VSCode as default for source code files
  - urn: "urn:hams:duti:com.microsoft.VSCode.public.data"
    bundle_id: "com.microsoft.VSCode"
    uti: "public.data"
    role: all
    tags: [editor-default]
    intro: "Open generic data files with VSCode."

  - urn: "urn:hams:duti:com.microsoft.VSCode.public.source-code"
    bundle_id: "com.microsoft.VSCode"
    uti: "public.source-code"
    role: all
    tags: [editor-default]
    intro: "Open source code files with VSCode."

  - urn: "urn:hams:duti:com.microsoft.VSCode.public.plain-text"
    bundle_id: "com.microsoft.VSCode"
    uti: "public.plain-text"
    role: all
    tags: [editor-default]
    intro: "Open plain text files with VSCode."

  # Set IINA as default for video files
  - urn: "urn:hams:duti:com.colliderli.iina.public.movie"
    bundle_id: "com.colliderli.iina"
    uti: "public.movie"
    role: all
    tags: [media-default]
    intro: "Open movie files with IINA."

  - urn: "urn:hams:duti:com.colliderli.iina.public.video"
    bundle_id: "com.colliderli.iina"
    uti: "public.video"
    role: all
    tags: [media-default]

  # Set Chrome as default browser
  - urn: "urn:hams:duti:com.google.Chrome.https"
    bundle_id: "com.google.Chrome"
    uti: "https"
    role: all
    tags: [browser-default]
    intro: "Set Chrome as default HTTPS handler."

  - urn: "urn:hams:duti:com.google.Chrome.http"
    bundle_id: "com.google.Chrome"
    uti: "http"
    role: all
    tags: [browser-default]

  # Set iTerm2 as default for SSH
  - urn: "urn:hams:duti:com.googlecode.iterm2.ssh"
    bundle_id: "com.googlecode.iterm2"
    uti: "ssh"
    role: all
    tags: [terminal-default]
    intro: "Handle SSH URLs with iTerm2."
```

**Fields:**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `urn` | YES | string | `urn:hams:duti:<bundle_id>.<uti>`. |
| `bundle_id` | YES | string | macOS application bundle identifier. |
| `uti` | YES | string | Uniform Type Identifier or URL scheme. |
| `role` | NO | string | `all`, `viewer`, `editor`, `shell`, `none`. Default `all`. |
| `tags` | NO | list | Category tags. |
| `intro` | NO | string | Description. |

**Bundle ID resolution**: The `bundle_id` field SHALL accept either a direct bundle identifier (e.g., `com.microsoft.VSCode`) or the provider can offer a helper to resolve app name to bundle ID via `osascript -e 'id of app "<AppName>"'`. The Hamsfile SHALL always store the resolved bundle ID.

**Probe implementation:**

- `duti -x <uti>` -- parse output for the current default handler's bundle ID.
- For URL schemes (http, https, ssh): `duti -d <bundle_id> <scheme>` or parse `duti -x <scheme>`.
- Compare against desired `bundle_id`. Match = `ok`, mismatch = `drift`.

**Apply flow:**

1. `duti -s <bundle_id> <uti> <role>` for each entry that is not `ok`.

**Remove flow:**

1. There is no `duti unset` command. The provider SHALL remove the entry from the Hamsfile and mark state as `removed`, but log a warning: "Default app association cannot be programmatically unset. The current association will remain until changed."

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams duti set <bundle_id> <uti> [role]` | Set association + record. |
| `hams duti remove <urn-id>` | Remove from Hamsfile (no actual unset). |
| `hams duti list` | Diff view. |
| `hams duti resolve <app-name>` | Helper: resolve app name to bundle ID. |

#### Scenario: Set a default file association

WHEN the user runs `hams duti set com.microsoft.VSCode public.source-code all`
THEN the duti provider SHALL execute `duti -s com.microsoft.VSCode public.source-code all`, record the entry in the Hamsfile, and update state.

#### Scenario: Probe a file association

WHEN `duti -x public.source-code` returns a bundle ID different from `com.microsoft.VSCode`
THEN the provider SHALL mark state as `drift` and re-apply on next apply.

#### Scenario: Remove has no undo

WHEN the user runs `hams duti remove com.microsoft.VSCode.public.source-code`
THEN the provider SHALL remove the entry from the Hamsfile, mark state as `removed`, and log a warning that the OS association was not changed.

#### Scenario: Resolve app name to bundle ID

WHEN the user runs `hams duti resolve "Visual Studio Code"`
THEN the provider SHALL execute `osascript -e 'id of app "Visual Studio Code"'` and print the result (e.g., `com.microsoft.VSCode`).

#### Scenario: duti on Linux

WHEN the duti provider is loaded on Linux
THEN the provider SHALL report `unsupported` and be silently skipped.

---

### Requirement: mas Provider

The mas provider SHALL wrap `mas install` and `mas uninstall` for Mac App Store applications. It is macOS-only and uses numeric app IDs.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `mas` |
| Display name | `mas` |
| File | `mas.hams.yaml` |
| Resource class | Package |
| Platform | macOS only |
| depend-on | `homebrew` (mas is installed via Homebrew) |
| Priority | 10 |

**Hamsfile schema (`mas.hams.yaml`):**

```yaml
apps:
  - app: "836500024"
    name: "WeChat"
    tags: [im-app]
    intro: "WeChat messaging app."

  - app: "451108668"
    name: "QQ"
    tags: [im-app]
    intro: "QQ messaging app."

  - app: "1449962996"
    name: "Tencent Lemon Cleaner"
    tags: [sys-pref-tool]
    intro: "System cleaner by Tencent."

  - app: "1500855883"
    name: "CapCut"
    tags: [media-entertainment-app]

  - app: "1233965871"
    name: "ScreenBrush"
    tags: [sys-pref-tool]

  - app: "747648890"
    name: "Telegram"
    tags: [im-app]

  - app: "1136220934"
    name: "Infuse"
    tags: [media-entertainment-app]
```

**Fields (extending CP-1):**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `app` | YES | string | Numeric Mac App Store ID (stored as string). |
| `name` | YES | string | Human-readable app name (for display; not used in commands). |
| `tags` | NO | list | Category tags. |
| `intro` | NO | string | Description. |

**Probe implementation:**

- `mas list` -- parse output `<id> <name> (<version>)` lines.

**Apply flow:**

1. `mas install <app-id>` for each missing app.
2. If the user is not signed in to the Mac App Store, `mas` will fail. The provider SHALL detect this and trigger the interactive popup (per TUI spec) to prompt the user to sign in.

**Remove flow:**

1. `mas uninstall <app-id>`.
2. Note: `mas uninstall` may not be available in all mas versions. If unavailable, log an error and suggest manual removal.

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams mas install <app-id>` | Install + record. Prompts for `name` if not provided. |
| `hams mas remove <app-id>` | Uninstall + delete from Hamsfile. |
| `hams mas list` | Diff view. |
| `hams mas search <query>` | Passthrough to `mas search`. |
| Any other | Passthrough to `mas <subcommand>`. |

**LLM enrichment:**

- Source: `mas info <app-id>` provides app name and description.
- The `name` field is auto-populated from `mas info` if not provided by the user.

**Signin handling**: The mas provider SHALL front-load the signin check at the beginning of its apply phase. If `mas account` returns no account, the provider SHALL trigger the notification system and interactive popup per the TUI spec (Q9). This prevents mid-run blocking.

**Idempotency**: `mas install` on an already-installed app is a no-op.

#### Scenario: Install a Mac App Store app

WHEN the user runs `hams mas install 836500024`
THEN the mas provider SHALL query `mas info 836500024` to get the app name, execute `mas install 836500024`, record `{app: "836500024", name: "WeChat"}` in the Hamsfile, and update state.

#### Scenario: Probe installed apps

WHEN the mas provider probes
THEN it SHALL execute `mas list`, parse `<id> <name> (<version>)` lines, and update state.

#### Scenario: Signin required

WHEN `mas account` indicates no active App Store account and apps need installation
THEN the mas provider SHALL trigger the interactive popup and notification system, log "Mac App Store signin required", and block until the user completes signin or the operation times out.

#### Scenario: App Store account switch needed

WHEN some apps are from a different App Store region (e.g., CN vs EN)
THEN the mas provider SHALL group apps by region annotation (if tags indicate region), process one group at a time, and MAY prompt the user to switch accounts. This is a manual, interactive operation.

#### Scenario: mas on Linux

WHEN the mas provider is loaded on Linux
THEN the provider SHALL report `unsupported` and be silently skipped.

---

### Requirement: Ansible Provider

The Ansible provider SHALL store paths to Ansible playbooks and wrap `ansible-playbook` execution. It uses check-based probing where idempotency is delegated to the playbooks themselves.

**Provider metadata:**

| Field | Value |
|-------|-------|
| Name | `ansible` |
| Display name | `Ansible` |
| File | `Ansible.hams.yaml` |
| Resource class | Check-based |
| Platform | both |
| depend-on | none (Ansible must be pre-installed via pip/uv/brew) |
| Priority | 14 (runs late, after all package/config providers) |

**Hamsfile schema (`Ansible.hams.yaml`):**

```yaml
steps:
  - urn: "urn:hams:ansible:setup-dev-environment"
    step: "Setup dev environment"
    description: "Run the full development environment playbook"
    playbook: "playbooks/dev-environment.yml"
    check: "ansible-playbook playbooks/dev-environment.yml --check"
    extra_vars:
      user: "zthxxx"
      home: "~"
    tags: [bootstrap]

  - urn: "urn:hams:ansible:configure-firewall"
    step: "Configure firewall"
    description: "Set up UFW firewall rules"
    playbook: "playbooks/firewall.yml"
    tags: [security]
```

**Fields:**

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `urn` | YES | string | `urn:hams:ansible:<id>`. |
| `step` | YES | string | Human-readable step name. |
| `description` | NO | string | What this playbook does. |
| `playbook` | YES | string | Path to the Ansible playbook (relative to `ansible.hams/` or absolute). |
| `check` | NO | string | Command to check if already applied. Default: `ansible-playbook <playbook> --check`. |
| `extra_vars` | NO | map | Key-value pairs passed as `--extra-vars`. |
| `tags` | NO | list | Category tags (hams tags, not Ansible tags). |

**Playbook path resolution**: Relative paths resolve relative to `<profile-dir>/ansible.hams/`. Absolute paths are used as-is.

**Probe implementation:**

- If `check:` is provided: execute it. Exit 0 = `ok`.
- If `check:` is not provided: run `ansible-playbook <playbook> --check`. Ansible's check mode reports whether changes would be made. Exit 0 with no changes = `ok`. Exit 0 with changes = `needs-apply`.
- If the check command fails, mark as `failed` (probe error).

**Apply flow:**

1. `ansible-playbook <playbook>` with `--extra-vars` if `extra_vars` is set.
2. Third-party output goes to session log file.

**Remove flow:**

1. Ansible playbooks are generally not reversible via hams. The provider SHALL remove the entry from the Hamsfile and mark state as `removed`.
2. Log a warning: "Ansible playbook effects were not reversed. Manual cleanup may be needed."

**CLI wrapping:**

| Subcommand | Behavior |
|------------|----------|
| `hams ansible run <urn-id>` | Execute a single playbook. |
| `hams ansible list` | Show all playbooks with status. |
| `hams ansible remove <urn-id>` | Remove from Hamsfile (no rollback). |

#### Scenario: Run an Ansible playbook

WHEN the user runs `hams ansible run setup-dev-environment`
THEN the Ansible provider SHALL resolve the playbook path to `<profile-dir>/ansible.hams/playbooks/dev-environment.yml`, execute `ansible-playbook playbooks/dev-environment.yml --extra-vars 'user=zthxxx home=~'`, and update state.

#### Scenario: Probe with check mode

WHEN an Ansible step has no explicit `check:` field
THEN the provider SHALL run `ansible-playbook <playbook> --check` and parse the output. If Ansible reports 0 changes, state = `ok`. If changes would be made, state = `needs-apply`.

#### Scenario: Probe with custom check command

WHEN an Ansible step has `check: "test -f /etc/ufw/ufw.conf"`
THEN the provider SHALL execute that command. Exit 0 = `ok`, non-zero = `needs-apply`.

#### Scenario: Remove does not rollback

WHEN the user runs `hams ansible remove configure-firewall`
THEN the provider SHALL remove the entry from the Hamsfile, mark state as `removed`, and log: "Ansible playbook effects were not reversed."

---

## Cross-Cutting Concerns

### Requirement: Provider Self-Install Dependencies

Each provider SHALL declare its `depend-on` chain in its manifest. The dependency resolution engine (defined in Provider System spec) SHALL resolve these dependencies before executing any provider operations.

| Provider | depend-on | Bootstrap action |
|----------|-----------|-----------------|
| bash | (none) | -- |
| homebrew | bash | Execute Homebrew install script via Bash provider |
| apt | (none) | -- |
| pnpm | npm | `npm install -g pnpm` |
| npm | (none) | Requires Node.js (user responsibility) |
| uv | (none) | -- |
| go | (none) | Requires Go (user responsibility) |
| cargo | (none) | Requires Rust/Cargo (user responsibility) |
| vscode-ext | homebrew (macOS) | Requires `visual-studio-code` cask |
| git-config | (none) | -- |
| git-clone | (none) | -- |
| defaults | (none) | -- |
| duti | homebrew | `brew install duti` |
| mas | homebrew | `brew install mas` |
| ansible | (none) | -- |

#### Scenario: Bootstrap chain resolution

WHEN `hams apply` encounters the VSCode Extension provider on macOS and `code` is not on `$PATH`
THEN hams SHALL resolve: vscode-ext depends on homebrew, homebrew depends on bash. It SHALL execute: (1) Bash provider bootstrap (no-op if bash is available), (2) Homebrew provider bootstrap (install Homebrew if missing), (3) check if `visual-studio-code` cask is installed, and THEN proceed with VSCode extension operations.

#### Scenario: Circular dependency detection

WHEN a provider manifest declares a circular depend-on chain (e.g., A depends on B, B depends on A)
THEN hams SHALL detect the cycle during manifest registration and fail with an error message listing the cycle path.

### Requirement: Platform-Conditional Provider Loading

Providers that are platform-specific SHALL declare their supported platforms in the manifest. On unsupported platforms, the provider SHALL be silently skipped during `hams apply`.

#### Scenario: macOS-only providers on Linux

WHEN `hams apply` runs on Linux and the profile includes `defaults.hams.yaml`, `duti.hams.yaml`, and `mas.hams.yaml`
THEN hams SHALL skip these three providers, log "Skipping <provider>: unsupported on linux" at debug level, and continue with remaining providers.

#### Scenario: Linux-only providers on macOS

WHEN `hams apply` runs on macOS and the profile includes `apt.hams.yaml`
THEN hams SHALL skip the apt provider, log "Skipping apt: unsupported on darwin" at debug level, and continue.

### Requirement: Hamsfile Naming Convention

Each provider's Hamsfile SHALL follow the naming convention `<Display Name>.hams.yaml` where the display name uses the provider's canonical capitalization.

| Provider | Hamsfile name |
|----------|--------------|
| bash | `Bash.hams.yaml` |
| homebrew | `Homebrew.hams.yaml` |
| apt | `apt.hams.yaml` |
| pnpm | `pnpm.hams.yaml` |
| npm | `npm.hams.yaml` |
| uv | `uv.hams.yaml` |
| go | `Go.hams.yaml` |
| cargo | `Cargo.hams.yaml` |
| vscode-ext | `VSCode Extension.hams.yaml` |
| git-config | `git config.hams.yaml` |
| git-clone | `git clone.hams.yaml` |
| defaults | `defaults.hams.yaml` |
| duti | `duti.hams.yaml` |
| mas | `mas.hams.yaml` |
| ansible | `Ansible.hams.yaml` |

#### Scenario: File name resolution

WHEN the Homebrew provider loads its Hamsfile
THEN it SHALL look for `<profile-dir>/Homebrew.hams.yaml` and `<profile-dir>/Homebrew.hams.local.yaml`, with the display name `Homebrew` determining the file name capitalization.

### Requirement: Provider LLM Enrichment Contract

All providers SHALL support the `enrich` operation, which uses LLM to generate or update `tags` and `intro` fields for Hamsfile entries.

**Common enrichment flow:**

1. Provider queries its native description source (see per-provider LLM enrichment sections).
2. Provider reads existing tags from the Hamsfile.
3. Payload sent to LLM (via claude/codex CLI subprocess): `{name, description, existing_tags, all_tags_in_file}`.
4. LLM returns: `{recommended_tags: [...], intro: "..."}`.
5. If interactive: tag picker shows recommendations. If `--hams-lucky`: auto-accept.
6. Updates written to Hamsfile via the hamsfile module.

| Provider | Description source |
|----------|-------------------|
| homebrew | `brew info --json=v2 <app>` (desc, homepage) |
| apt | `apt show <pkg>` (Description) |
| pnpm | `pnpm info <pkg>` (description) |
| npm | `npm info <pkg> --json` (description) |
| uv | `uv pip show <pkg>` or PyPI API |
| go | pkg.go.dev page or `go doc` |
| cargo | `cargo info <crate>` or crates.io API |
| vscode-ext | VS Marketplace API |
| mas | `mas info <id>` |
| bash | User-provided `description` field (no external source) |
| git-config | Inferred from key name |
| git-clone | GitHub/GitLab API for repo description |
| defaults | Inferred from domain + key |
| duti | Inferred from bundle_id + UTI |
| ansible | User-provided `description` field |

#### Scenario: Enrich a Homebrew package

WHEN the user runs `hams brew enrich git`
THEN the Homebrew provider SHALL fetch `brew info --json=v2 git`, extract the description, read existing tags and all tags from `Homebrew.hams.yaml`, send to LLM, and present the tag picker with recommendations.

#### Scenario: Enrich with --hams-lucky

WHEN the user runs `hams brew enrich git --hams-lucky`
THEN the provider SHALL auto-accept all LLM-recommended tags and intro without showing the tag picker.

#### Scenario: LLM unavailable during enrich

WHEN the LLM CLI subprocess is not available or times out
THEN the provider SHALL log an error, keep existing tags and intro unchanged, and report the failure in the final summary.

### Requirement: Tag picker TUI layout

When no `--hams-tag` is specified during install, providers that support LLM enrichment SHALL display a TUI multi-select picker with three sections:

1. **LLM-recommended tags** — pre-selected (checked by default).
2. **All other existing tags** from the current Hamsfile — unselected.
3. **Free-text input field** — for entering a new tag not in either list.

The user SHALL be able to toggle selections with space, type to filter, and confirm with enter. When `--hams-lucky` is specified or the terminal is non-interactive, the picker SHALL be skipped and all LLM-recommended tags SHALL be auto-accepted.

#### Scenario: Interactive tag picker with LLM recommendations

WHEN `hams brew install git` is run in an interactive terminal without `--hams-tag`
AND the LLM recommends tags `["network-tool", "development-tool"]`
AND the Hamsfile already contains tags `["runtime-environment", "terminal-tool", "network-tool"]`
THEN the picker SHALL show `network-tool` and `development-tool` as pre-selected
AND `runtime-environment` and `terminal-tool` as unselected options
AND a free-text input field at the bottom for creating new tags.

#### Scenario: Free-text new tag creation

WHEN the user types `version-control` in the free-text field and presses enter
AND `version-control` does not exist in the current tag list
THEN `version-control` SHALL be added to the selected tags
AND it SHALL appear in the picker as a selected item.

### Requirement: Async LLM enrichment during install

When a provider installs a resource and LLM enrichment is configured, the LLM call SHALL run **asynchronously in a separate goroutine** concurrently with the install command execution, not blocking the user. The enrichment flow:

1. Install command starts executing.
2. Concurrently, provider fetches package description (e.g., `brew info --json`) and submits to LLM.
3. If install finishes before LLM returns, the Hamsfile entry is written without tags/intro.
4. When LLM returns, the hamsfile SDK updates the entry in-place (adding tags/intro).
5. If LLM times out or is interrupted by Ctrl+C, the error SHALL be collected and reported in the **final apply summary**, not as an immediate failure.

#### Scenario: LLM completes before install

WHEN `hams brew install git` is run with LLM configured
AND the LLM returns tag recommendations before `brew install git` completes
THEN the Hamsfile entry SHALL be written with both the package and the LLM-recommended tags/intro after install succeeds.

#### Scenario: LLM times out during apply

WHEN `hams apply` is running and LLM enrichment times out for 3 packages
THEN the 3 packages SHALL be installed successfully without tags/intro
AND the final apply summary SHALL report "3 packages installed without LLM enrichment (timeout)"
AND the user SHALL be told they can run `hams brew enrich <app>` to retry.

---

### Requirement: Per-provider Docker integration tests

Every linux-containerizable builtin provider SHALL own its integration
test under `internal/provider/builtin/<provider>/integration/`, with
exactly two files:

- `Dockerfile` — `FROM hams-itest-base:<base-hash>` plus whatever
  minimal delta the provider needs to run. The Dockerfile SHALL NOT
  pre-install the provider's runtime (e.g., python, node, go, rust,
  brew) — runtime installation is the provider's own responsibility
  at integration-test time, because that is what hams must do for real
  users.
- `integration.sh` — bash script sourcing the shared helpers at
  `/e2e/base/lib/{assertions,yaml_assert,provider_flow}.sh`. At
  minimum, the script SHALL call
  `standard_cli_flow <provider> <install_verb> <existing_pkg> <new_pkg>`
  (or its `post_install_check` variant for providers without a PATH
  binary) to exercise the canonical install / re-install / refresh /
  remove lifecycle.

**In scope** (11 providers): apt, ansible, bash, cargo, git (config +
clone), goinstall, homebrew, npm, pnpm, uv, vscodeext.

**Out of scope** (macOS-only): defaults, duti, mas. No docker path
exists; a macOS CI runner would be required.

**Base image**:

- `e2e/base/Dockerfile` SHALL produce a `hams-itest-base:<sha(Dockerfile)>`
  image containing only `debian:bookworm-slim` + `ca-certificates`,
  `curl`, `bash`, `git`, `sudo`, `yq` (pinned version from GitHub
  releases). No language toolchains, no package managers beyond apt-get.
- The base image SHALL be built once per change to `e2e/base/Dockerfile`
  and cached by content hash (`docker image inspect` gate before
  rebuild).

**Per-provider image**:

- SHALL be named `hams-itest-<provider>:<sha(integration/Dockerfile)>`.
- SHALL `FROM hams-itest-base:<frozen-base-hash>` to reuse base layers.
- SHALL be rebuilt only when its own Dockerfile hash changes.
- Stale tags (same repo, different hash) SHALL be pruned opportunistically.

**Test runtime contract**:

- The `hams` binary SHALL be bind-mounted read-only at `/usr/local/bin/hams`.
- Shared helpers SHALL be bind-mounted read-only at `/e2e/base/lib/`.
- Each provider's integration dir SHALL be bind-mounted read-only at
  `/integration/`, with `integration.sh` executable.
- Tests SHALL run as root inside the container (sudo is a no-op).
- Every docker run SHALL start a fresh container; no state crosses
  between provider tests.

#### Scenario: apt integration test runs in isolation with no other provider bootstrapped

- **WHEN** `task ci:itest:run PROVIDER=apt` executes
- **THEN** the `hams-itest-apt` container starts with only the base image + any apt-specific runtime (none needed; apt-get is pre-installed by debian)
- **AND** the container SHALL NOT contain `brew`, `cargo`, `node`, `python3` beyond what debian's base image provides
- **AND** `integration.sh` SHALL call `standard_cli_flow apt install jq btop` and SHALL assert the install/re-install/refresh/remove lifecycle against `.state/<machine>/apt.state.yaml`
- **AND** at NO point SHALL any other provider (Homebrew, pnpm, cargo, etc.) have `Bootstrap` called during the test, because no hamsfile or state file exists for them.

#### Scenario: Per-provider Dockerfile cache reuses across runs

- **WHEN** the developer runs `task ci:itest:run PROVIDER=apt` twice in a row without changing `e2e/base/Dockerfile` or `internal/provider/builtin/apt/integration/Dockerfile`
- **THEN** the second run SHALL NOT rebuild either image — `docker image inspect hams-itest-base:<hash>` and `docker image inspect hams-itest-apt:<hash>` both succeed, skipping the build step.

---

### Requirement: `standard_cli_flow` shared helper

`e2e/base/lib/provider_flow.sh` SHALL expose a function
`standard_cli_flow` that implements the canonical CLI-only integration
flow. Every in-scope provider's `integration.sh` SHALL call this helper.

**Signature**:

```
standard_cli_flow <provider> <install_verb> <existing_pkg> <new_pkg> [<post_install_check>]
```

- `<provider>` — hams provider name (e.g., `apt`, `brew`, `pnpm`).
- `<install_verb>` — the provider's install subcommand (`install`, `add`).
- `<existing_pkg>` — a package name that the helper installs first to
  seed state, then re-installs to verify timestamp semantics.
- `<new_pkg>` — a second, distinct package name used for the
  install-new / refresh / remove portion.
- `<post_install_check>` (optional, for providers without a PATH
  binary) — a bash function name that verifies installation succeeded.
  Defaults to `command -v <pkg>` if unset.

**Steps performed** (all assertions fail fast with exit 1):

1. `hams <provider> <install_verb> <existing_pkg>` — seed state.
2. Capture `existing_pkg`'s `first_install_at` from the state file.
3. `sleep 1`.
4. `hams <provider> <install_verb> <existing_pkg>` — assert
   `updated_at` bumped, `first_install_at` unchanged.
5. Pre-check: `command -v <new_pkg>` (or the supplied
   `post_install_check`) SHALL fail.
6. `hams <provider> <install_verb> <new_pkg>` — assert the post-install
   check now succeeds, and assert state has `new_pkg.state = ok`,
   `first_install_at` set, `removed_at` absent.
7. `sleep 1; hams refresh --only=<provider>` — assert `new_pkg`'s
   `updated_at` bumped.
8. `hams <provider> remove <new_pkg>` — assert the post-install check
   fails again, state has `new_pkg.state = removed`, `removed_at` set.

#### Scenario: standard_cli_flow used by apt provider

- **WHEN** `internal/provider/builtin/apt/integration/integration.sh` calls `standard_cli_flow apt install jq btop`
- **THEN** the helper SHALL execute the 8-step lifecycle against the `hams` binary in the container and the state file at `<store>/.state/<machine-id>/apt.state.yaml`
- **AND** any failed assertion SHALL exit non-zero with a descriptive message naming the failed step, path, and expected vs actual values.

#### Scenario: standard_cli_flow used with a post-install check hook

- **WHEN** a provider has no PATH binary (e.g., `git-config` sets a git configuration key rather than installing an executable), and its `integration.sh` supplies a custom `post_install_check` function
- **THEN** `standard_cli_flow` SHALL call the supplied function in place of `command -v <pkg>` at the pre-check and post-remove assertions
- **AND** the rest of the lifecycle (state-file timestamp assertions) SHALL proceed identically.

---
<!-- Merged from change: fix-v1-planning-gaps -->

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

### Requirement: pnpm Bootstrap signals consent required when missing

The pnpm provider's `Bootstrap(ctx)` SHALL return a
`*provider.BootstrapRequiredError` (wrapping `provider.ErrBootstrapRequired`)
when `pnpm` is not on `$PATH`. The error SHALL carry:

- `Provider: "pnpm"`
- `Binary: "pnpm"`
- `Script: "npm install -g pnpm"`

The pnpm manifest's `DependsOn` SHALL declare two entries with
single-purpose semantics:

- `{Provider: "npm", Package: "pnpm"}` — DAG ordering only (no
  `Script`). Ensures npm is processed before pnpm in the apply
  pipeline.
- `{Provider: "bash", Script: "npm install -g pnpm"}` — script host.
  `bash` is the only provider that implements
  `provider.BashScriptRunner`, so any DependsOn entry with a `.Script`
  MUST target bash; the script's own invocation is what calls into
  npm.

`provider.RunBootstrap` can then execute the script under user
consent (via the `--bootstrap` flag or TTY `[y/N/s]` prompt) by
delegating to the bash provider's `RunScript` boundary.

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

The duti manifest's `DependsOn` SHALL declare two darwin-gated
entries: `{Provider: "brew", Platform: darwin}` for DAG ordering
(brew must be bootstrapped before duti) and `{Provider: "bash",
Script: "brew install duti", Platform: darwin}` for the script host
(bash is the only BashScriptRunner; the shell command itself calls
into brew).

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

The mas manifest's `DependsOn` SHALL declare two darwin-gated
entries: `{Provider: "brew", Platform: darwin}` for DAG ordering and
`{Provider: "bash", Script: "brew install mas", Platform: darwin}`
for the script host (same rationale as duti).

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

pipx is used over pip because PEP 668 flags system-pip installs on
modern Python installations (Debian 12+, brew-python) with
"externally-managed environment". pipx creates an isolated venv per
app and is the Python community's accepted answer for installing
tools from PyPI.

The ansible manifest's `DependsOn` SHALL include
`{Provider: "bash", Script: "pipx install --include-deps ansible"}`.

Users without pipx will see the script fail with "pipx: command not
found"; the surrounding bootstrap-failure error path surfaces that
chain so users can `apt install pipx` (Debian) / `brew install pipx`
(macOS) first.

#### Scenario: ansible missing produces structured error with pipx

- **WHEN** ansible-playbook is not on `$PATH`
- **THEN** `Bootstrap` SHALL return `*provider.BootstrapRequiredError` with `Binary: "ansible-playbook"` and `Script: "pipx install --include-deps ansible"`.

### Requirement: DependsOn Script entries must target the bash provider

For every builtin provider manifest, any `DependsOn[i]` entry with a
non-empty `.Script` field MUST have `.Provider == "bash"`. Rationale:
`provider.RunBootstrap` looks up `dep.Provider` in the registry and
type-asserts the looked-up provider to `provider.BashScriptRunner`.
Only the `bash` builtin implements that interface. Targeting any
other provider (e.g. `npm`, `brew`) makes RunBootstrap fail at
`--bootstrap` time with "bootstrap host does not implement
BashScriptRunner" — a runtime error surfaced exactly on the
fresh-machine path the consent flow is meant to serve.

DAG-only entries (empty `.Script`, present purely for topological
ordering via `ResolveDAG`) can target any provider; this invariant
applies only to scripted entries.

This invariant is enforced by a unit test
(`cli/bootstrap_invariant_test.go::TestBuiltinManifestScriptHostsAreBash`)
that iterates every registered builtin's manifest and fails the build
if a Script entry targets a non-bash host.

#### Scenario: framework invariant enforces bash host

- **WHEN** the test `TestBuiltinManifestScriptHostsAreBash` is run
- **THEN** it SHALL iterate every registered builtin provider's `Manifest().DependsOn`
- **AND** for each entry with a non-empty `.Script`, assert `.Provider == "bash"`
- **AND** fail the test on any non-bash host, naming the offending provider and index.

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
  curl-installer (the installer uses git itself).
- `bash`: always present; `Bootstrap` is already a no-op.

This skip-list is explicit scope; it SHALL be reconsidered only when
a concrete user story or support incident demonstrates one of these
providers as a blocker on a fresh-machine restore path.

#### Scenario: skipped providers retain plain-string errors

- **WHEN** an npm / cargo / goinstall / uv / vscodeext / apt / defaults / git provider is needed but its prerequisite binary is missing
- **THEN** `Bootstrap` SHALL return a plain `fmt.Errorf` string as today
- **AND** the CLI bootstrap loop SHALL surface that error via the existing "bootstrap failed for providers with hamsfiles" path (NOT the `BootstrapRequiredError` consent flow).
