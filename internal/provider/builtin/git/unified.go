package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/provider"
)

// unifiedProviderName is the single CLI verb for the merged git
// provider. Users type `hams git …` — internal sub-providers
// (git-config, git-clone) still own their own state files and
// hamsfile entries, but the user never types `hams git-config` or
// `hams git-clone` directly.
const unifiedProviderName = "git"

// unifiedDisplayName is the human-readable name shown in `hams
// --help` and provider help output.
const unifiedDisplayName = "git"

// verbRemove / verbList are extracted as constants because
// golangci-lint goconst flags 3+ occurrences across the package;
// the unified dispatcher references them alongside clone.go and
// git.go, so collecting them in one place keeps the lint rule
// satisfied without per-call-site //nolint noise.
const (
	verbRemove = "remove"
	verbList   = "list"
)

// UnifiedProvider is the ProviderHandler for the merged `hams git`
// CLI entry point. It does NOT implement provider.Provider — the
// apply/refresh pipeline still sees git-config and git-clone as
// independent providers so their state files (git-config.state.yaml
// and git-clone.state.yaml) stay separate and the existing Plan /
// Apply / Probe contracts don't need to understand two different
// resource classes under one roof.
//
// CLI dispatch:
//
//   - `hams git config …` → ConfigProvider.HandleCommand.
//   - `hams git clone …`  → CloneProvider-equivalent (args translated
//     from the real `git clone <remote> [<path>]` shape to the
//     git-clone provider's internal `add <remote> --hams-path=<path>`
//     form so the auto-record path still hits).
//   - `hams git <any-other> …` → passthrough to the real `git`
//     binary, preserving stdin / stdout / stderr / exit code.
//
// Matches CLAUDE.md's "provider wrapped commands MUST behave exactly
// like the original command, at least at the first-level command
// entry point": every real git subcommand that hams does NOT
// intercept still works when invoked through `hams git`.
type UnifiedProvider struct {
	config *ConfigProvider
	clone  *CloneProvider
}

// NewUnifiedProvider wires a UnifiedProvider backed by the two
// underlying sub-providers. Register this one (in place of
// NewConfigProvider and NewCloneProvider individually) with the CLI
// dispatcher; keep the sub-providers registered separately with the
// apply registry for Plan / Apply / Probe.
func NewUnifiedProvider(cfg *config.Config) *UnifiedProvider {
	return &UnifiedProvider{
		config: NewConfigProvider(cfg),
		clone:  NewCloneProvider(cfg),
	}
}

// Config exposes the underlying config sub-provider so
// internal/cli/register.go can still register it with the apply
// registry while only binding the unified one to the CLI surface.
func (p *UnifiedProvider) Config() *ConfigProvider { return p.config }

// Clone exposes the underlying clone sub-provider for the same
// reason as Config().
func (p *UnifiedProvider) Clone() *CloneProvider { return p.clone }

// Name returns the CLI verb users type.
func (p *UnifiedProvider) Name() string { return unifiedProviderName }

// DisplayName returns the human-readable name.
func (p *UnifiedProvider) DisplayName() string { return unifiedDisplayName }

// HandleCommand dispatches `hams git …` subcommands. The first
// positional arg decides the branch; unknown subcommands pass
// through to the real git so `hams git pull`, `hams git status`,
// `hams git log`, etc., work identically to their unwrapped form.
func (p *UnifiedProvider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return p.usageError()
	}

	switch args[0] {
	case "config":
		return p.handleConfig(ctx, args[1:], hamsFlags, flags)
	case "clone":
		return p.handleClone(ctx, args[1:], hamsFlags, flags)
	default:
		return p.passthrough(ctx, args, flags)
	}
}

// handleConfig forwards to the existing ConfigProvider handler. The
// argument shape is identical to the legacy `hams git-config …`
// form: `set <key> <value>`, bare `<key> <value>`, `remove <key>`,
// `list`. No translation needed.
func (p *UnifiedProvider) handleConfig(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	return p.config.HandleCommand(ctx, args, hamsFlags, flags)
}

// handleClone translates from the real git shape into the
// CloneProvider's internal DSL:
//
//   - `hams git clone <remote>` → requires --hams-path=<path> (same
//     as `hams git-clone add <remote> --hams-path=<path>`).
//   - `hams git clone <remote> <path>` → treats <path> as the
//     intended local directory; synthesizes --hams-path=<path> on
//     behalf of the user.
//   - `hams git clone remove <urn>` → passes through to the
//     CloneProvider's remove verb.
//   - `hams git clone list` → prints the CloneProvider's diff.
//
// The second shape is the one a user types reflexively because it
// matches git's own CLI. The first is accepted for symmetry with the
// legacy `hams git-clone add` form so pre-merge scripts keep working.
func (p *UnifiedProvider) handleClone(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"git clone requires a remote URL",
			"Usage: hams git clone <remote> <local-path>",
			"       hams git clone <remote> --hams-path=<local-path>",
			"       hams git clone remove <urn>",
			"       hams git clone list",
		)
	}

	// Forward management subcommands straight through without
	// translation.
	switch args[0] {
	case verbRemove, verbList:
		return p.clone.HandleCommand(ctx, args, hamsFlags, flags)
	}

	// Canonical add form: `<remote>` [+ optional `<path>` positional].
	// Translate to the CloneProvider's `add <remote> --hams-path=<path>`
	// DSL so auto-record still works unchanged.
	remote := args[0]
	extra := args[1:]
	if path, ok := hamsFlags["path"]; ok && path != "" {
		// Already using the explicit --hams-path form — delegate to
		// the legacy handler directly.
		return p.clone.HandleCommand(ctx, append([]string{"add", remote}, extra...), hamsFlags, flags)
	}

	// Fish a positional <path> out of the remaining args. Reject more
	// than one positional so `hams git clone <remote> <path> <bogus>`
	// doesn't silently drop the extra token — matches the strict
	// arg-count approach the other providers take.
	positional := make([]string, 0, len(extra))
	for _, a := range extra {
		if strings.HasPrefix(a, "--") {
			// Real git flags (--depth, --branch, …) — not supported in
			// the record path yet. Reject loud rather than silently
			// ignore so the user knows to file a follow-up for the
			// flag they wanted.
			return hamserr.NewUserError(hamserr.ExitUsageError,
				fmt.Sprintf("hams git clone does not yet forward git flag %q", a),
				"File a follow-up request with the flag name",
				"Or run plain `git clone` and add the record later via `hams git clone <remote> <path>`",
			)
		}
		positional = append(positional, a)
	}
	if len(positional) != 1 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("hams git clone expects exactly one local path (got %d: %v)", len(positional), positional),
			"Usage: hams git clone <remote> <local-path>",
		)
	}

	path := positional[0]
	hamsFlags["path"] = path
	return p.clone.HandleCommand(ctx, []string{"add", remote}, hamsFlags, flags)
}

// passthrough shells out to the real `git` binary with args
// unchanged. Used for any subcommand hams does not intercept (pull,
// push, status, diff, log, etc.). Preserves stdio and exit code so
// users cannot tell they went through `hams git` vs plain `git` for
// these verbs.
func (p *UnifiedProvider) passthrough(ctx context.Context, args []string, flags *provider.GlobalFlags) error {
	if flags.DryRun {
		fmt.Fprintf(flags.Stdout(), "[dry-run] Would run: git %s\n", strings.Join(args, " "))
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // args come from hams CLI; pass-through is intentional
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (p *UnifiedProvider) usageError() error {
	return hamserr.NewUserError(hamserr.ExitUsageError,
		"hams git requires a subcommand",
		"Recorded subcommands:",
		"  hams git config <key> <value>    — set a global git config entry and record it",
		"  hams git config remove <key>     — unset a recorded entry",
		"  hams git config list             — show the managed config",
		"  hams git clone <remote> <path>   — clone a repo and record it",
		"  hams git clone remove <urn>      — drop a recorded clone",
		"  hams git clone list              — show the managed clones",
		"Any other subcommand (hams git pull, hams git push, …) is passed through to the real git.",
	)
}
