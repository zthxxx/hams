package git

import (
	"context"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/provider"
)

// UnifiedHandler exposes both git-config and git-clone behind a single
// `hams git` CLI entry point — matching the user expectation that
// `hams git ...` mirrors the real `git ...` invocation. The handler is
// CLI-surface only; the apply / refresh layer continues to see the
// underlying ConfigProvider and CloneProvider as separate Providers
// with their own state files (per the original architecture).
//
// Routing:
//
//	hams git config <args>  → ConfigProvider.HandleCommand(args)
//	hams git clone <args>   → CloneProvider.HandleCommand(["add"]+args)
//
// `clone` is mapped to the CloneProvider's existing `add` verb because
// the CLI grammar of `git clone <url> [path]` matches what `hams git-clone
// add <url> <path>` already accepts. Future cycles can rename the
// underlying verb if the user-facing form proves more idiomatic.
// unifiedCLIName is the user-facing CLI verb registered in the
// dispatcher. Centralized so the Name/DisplayName accessors and the
// help-text switch in cli/root.go can refer to a single constant.
const unifiedCLIName = "git"

// UnifiedHandler exposes both ConfigProvider and CloneProvider behind a
// single `hams git` CLI entry point.
type UnifiedHandler struct {
	cfgProvider   *ConfigProvider
	cloneProvider *CloneProvider
}

// NewUnifiedHandler wires both sub-handlers behind one CLI surface.
// Pass the live ConfigProvider/CloneProvider instances so test
// substitutions of CmdRunner propagate transparently.
func NewUnifiedHandler(cfgP *ConfigProvider, cloneP *CloneProvider) *UnifiedHandler {
	return &UnifiedHandler{cfgProvider: cfgP, cloneProvider: cloneP}
}

// Name is the CLI verb users type — `hams git`.
func (u *UnifiedHandler) Name() string { return unifiedCLIName }

// DisplayName for `hams --help`.
func (u *UnifiedHandler) DisplayName() string { return unifiedCLIName }

// HandleCommand parses the first positional arg as the git subcommand
// (config / clone) and delegates the rest. Unknown subcommands surface
// a usage UFE listing the supported entries.
func (u *UnifiedHandler) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			i18n.T(i18n.GitUsageHeader),
			i18n.T(i18n.GitUsageSuggestMain),
			i18n.T(i18n.GitUsageSuggestSubcommands),
			i18n.T(i18n.GitUsageExampleConfig),
			i18n.T(i18n.GitUsageExampleClone),
		)
	}
	subcommand, rest := args[0], args[1:]
	switch subcommand {
	case "config":
		return u.cfgProvider.HandleCommand(ctx, rest, hamsFlags, flags)
	case "clone":
		// Map `git clone <url> [path]` onto the CloneProvider's
		// existing `add <url> <path>` verb so the auto-record path
		// stays unchanged. The provider validates url/path itself.
		return u.cloneProvider.HandleCommand(ctx, append([]string{"add"}, rest...), hamsFlags, flags)
	default:
		return hamserr.NewUserError(hamserr.ExitUsageError,
			i18n.Tf(i18n.GitUnknownSubcommand, map[string]any{"Sub": subcommand}),
			i18n.T(i18n.GitUsageSuggestSubcommands),
			"Run 'hams git --help' for usage.",
		)
	}
}
