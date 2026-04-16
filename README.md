# hams 🐹

[![CI](https://github.com/zthxxx/hams/actions/workflows/ci.yml/badge.svg)](https://github.com/zthxxx/hams/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/zthxxx/hams)](https://goreportcard.com/report/github.com/zthxxx/hams)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**hams** (hamster) is a declarative IaC environment management tool for macOS and Linux workstations. Like a hamster hoarding supplies, hams hoards your environment configurations for safekeeping and restoration.

It wraps existing package managers (Homebrew, pnpm, npm, apt, etc.) to **automatically record installations** into declarative YAML config files ("Hamsfiles"), enabling **one-command environment restoration** on new machines.

## Quick Start

### Install

```bash
# One-line install (downloads latest release binary)
bash -c "$(curl -fsSL https://github.com/zthxxx/hams/raw/master/scripts/install.sh)"

# Or via Homebrew
brew install zthxxx/tap/hams
```

### Usage

```bash
# Install a package and auto-record it
hams brew install htop

# Install via pnpm (auto-adds --global)
hams pnpm add serve

# Set git config and record it
hams git-config user.name "Your Name"

# Restore everything on a new machine (add --bootstrap if brew isn't installed yet)
hams apply --bootstrap --from-repo=your-username/hams-store
```

### How It Works

1. **Install via CLI**: `hams brew install git` runs `brew install git` AND records `git` in `Homebrew.hams.yaml`
2. **Sync to Git**: Push your hams-store repo with all `*.hams.yaml` files
3. **Restore anywhere**: `hams apply --bootstrap --from-repo=you/hams-store` replays all installations on a new machine (add `--bootstrap` if prerequisites like Homebrew aren't installed yet)

## Features

- **15 builtin providers**: Homebrew, apt, pnpm, npm, uv, goinstall, cargo, VS Code Extensions (`code-ext`), mas (App Store), git config/clone, macOS defaults, duti, Ansible
- **Terraform-style state**: tracks what's installed, retries failures, detects drift
- **Comment-preserving YAML**: your Hamsfile comments survive round-trips
- **Multi-machine profiles**: one git repo, multiple machine configs (macOS, Linux, OpenWrt)
- **LLM-powered categorization**: auto-generates tags and descriptions via Claude/Codex
- **Provider plugin system**: extend with custom providers via Go SDK

## Comparison

| Tool | hams advantage |
|------|---------------|
| NixOS / nix-darwin | Pragmatic, not strict; uses existing package managers |
| brew bundle | Multi-provider; preserves comments; has hooks and state tracking |
| Ansible | CLI-first auto-record; no need to write playbooks first |
| chezmoi | Handles packages, not just dotfiles |
| Terraform/Pulumi | Host-level, not cloud; record-as-you-go, not program-first |

## Documentation

Full documentation: [hams.zthxxx.me/docs](https://hams.zthxxx.me/docs)

## Development

```bash
task setup    # Install dev tools
task build    # Build binary
task test     # Run tests
task lint     # Run linters
task check    # fmt + lint + test
```

See [CLAUDE.md](CLAUDE.md) for project architecture and development guidelines.

## License

MIT
