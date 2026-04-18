package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
//	hams git clone <url> [path]
//	                        → CloneProvider.HandleCommand(["add", url, ...])
//	                          (path, if present, folded into hamsFlags["path"])
//	hams git <anything else>
//	                        → passthrough to the real `git` binary with
//	                          stdin/stdout/stderr + exit code preserved.
//
// The passthrough branch closes the CLAUDE.md §Current Tasks rule that
// "Provider wrapped commands MUST behave exactly like the original
// command, at least at the first-level command entry point." A user who
// aliases `git=hams git` for auto-record now finds that every git
// subcommand that hams does NOT intercept (pull, status, log, push, …)
// works identically to plain `git`.
//
// `clone` is mapped to the CloneProvider's existing `add` verb because
// the CLI grammar of `git clone <url> [path]` matches what `hams git-clone
// add <url> <path>` already accepts.

// unifiedCLIName is the user-facing CLI verb registered in the
// dispatcher. Centralized so the Name/DisplayName accessors and the
// help-text switch in cli/root.go can refer to a single constant.
const unifiedCLIName = "git"

// verbRemove / verbList are the management sub-verbs on the clone
// handler that we forward unchanged (as opposed to translating into the
// `add` DSL the way `clone` itself does). Extracted as consts because
// golangci-lint's goconst rule flags 3+ occurrences across the package
// and this keeps the exception in one place.
const (
	verbRemove = "remove"
	verbList   = "list"
)

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
// (config / clone) and delegates the rest. Unknown subcommands fall
// through to a transparent passthrough against the real `git` binary so
// `hams git pull`, `hams git log`, `hams git status`, etc. work
// identically to their unwrapped form.
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
		return u.handleClone(ctx, rest, hamsFlags, flags)
	default:
		// Passthrough for every other subcommand (pull, push, status,
		// log, diff, …). Mirrors the real git binary exactly — stdio
		// preserved, exit code preserved, honoring flags.DryRun as a
		// preview-only mode.
		return u.passthrough(ctx, args, flags)
	}
}

// handleClone translates `hams git clone <url> [path]` onto the
// CloneProvider's internal `add <url>` DSL. Shapes accepted:
//
//	hams git clone <url>                              (needs --hams-path)
//	hams git clone <url> <path>                       (natural git form;
//	                                                   path folded into
//	                                                   hamsFlags["path"])
//	hams git clone --hams-path=<path> <url>           (legacy form)
//	hams git clone remove <urn>                       (delete recorded clone)
//	hams git clone list                               (show clones diff)
//
// Explicit git flags that hams does not yet forward (--depth, --branch,
// --recurse-submodules, …) are rejected with a UFE so the user knows to
// file a follow-up. Silently dropping them would cause surprises
// downstream (e.g., a `--depth=1` CI clone that secretly becomes a full
// clone because hams ignored the flag).
func (u *UnifiedHandler) handleClone(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"git clone requires a remote URL",
			"Usage: hams git clone <remote> <local-path>",
			"       hams git clone <remote> --hams-path=<local-path>",
			"       hams git clone remove <urn>",
			"       hams git clone list",
		)
	}

	// Forward management sub-verbs unchanged (the CloneProvider handles
	// `remove` and `list` in its own CLI handler).
	switch args[0] {
	case verbRemove, verbList:
		return u.cloneProvider.HandleCommand(ctx, args, hamsFlags, flags)
	}

	remote := args[0]
	extra := args[1:]

	// Canonical legacy form: `--hams-path=<path>` already in hamsFlags.
	// Forward remote + any leftover args straight through so the
	// existing CloneProvider add semantics apply verbatim.
	if path, ok := hamsFlags["path"]; ok && path != "" {
		return u.cloneProvider.HandleCommand(ctx, append([]string{"add", remote}, extra...), hamsFlags, flags)
	}

	// Natural `git clone <url> <path>` form: extract the single
	// positional <path>. Reject unknown git flags loud rather than
	// silently drop them.
	positional := make([]string, 0, len(extra))
	for _, a := range extra {
		if strings.HasPrefix(a, "--") {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				fmt.Sprintf("hams git clone does not yet forward git flag %q", a),
				"File a follow-up request naming the flag you need",
				"Or run plain `git clone ...` and record it with `hams git clone <remote> <path>` after",
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

	// Synthesize the --hams-path that the CloneProvider's add verb
	// expects, then forward. hamsFlags is the flag bag that the
	// CloneProvider also reads, so mutating it is the correct wire-up.
	hamsFlags["path"] = positional[0]
	return u.cloneProvider.HandleCommand(ctx, []string{"add", remote}, hamsFlags, flags)
}

// passthroughExec is the DI seam for running `git <args>` transparently.
// Tests swap this to a fake that records the invocation without spawning
// a real process. Production value calls the real git binary with
// stdin/stdout/stderr + exit code preserved.
//
//nolint:gochecknoglobals // DI seam; documented contract, only swapped in tests.
var passthroughExec = func(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // args flow from the hams CLI; passthrough is the intended behavior
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// passthrough shells out to the real `git` binary with args unchanged,
// preserving stdin/stdout/stderr and exit code. Honors flags.DryRun as
// a preview-only mode (print the would-be invocation, skip exec).
func (u *UnifiedHandler) passthrough(ctx context.Context, args []string, flags *provider.GlobalFlags) error {
	if flags != nil && flags.DryRun {
		fmt.Fprintln(flags.Stdout(), i18n.Tf(i18n.ProviderStatusDryRunRun, map[string]any{"Cmd": "git " + strings.Join(args, " ")}))
		return nil
	}
	return passthroughExec(ctx, args)
}
