package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/term"

	"github.com/zthxxx/hams/internal/provider"
)

// bootDecision is the outcome of consent resolution for a single
// provider's bootstrap prompt.
type bootDecision int

const (
	// bootDecisionDeny means: surface the actionable error and fail.
	bootDecisionDeny bootDecision = iota
	// bootDecisionRun means: execute the bootstrap script (user consented).
	bootDecisionRun
	// bootDecisionSkipProvider means: drop this provider from the run.
	bootDecisionSkipProvider
)

// bootstrapPromptIn and bootstrapPromptOut are the interactive-prompt
// seams. Overridden in tests to inject scripted input and capture
// output. Production values are os.Stdin / os.Stderr.
//
// Cycle 253: prompts write to stderr, not stdout. Same rationale as
// cycle 250 (clone progress) + cycle 252 (profile prompts): stdout
// is the channel for primary machine-consumable output (JSON
// summaries), stderr is for prompts / diagnostics / progress.
// Pre-cycle-253 `interactiveBootstrapPrompt` wrote the
// script-preview + [y/N/s] question to stdout, so an interactive
// `hams --json apply` hitting a BootstrapRequiredError interleaved
// prompt prose into the JSON output surface. Interactive users
// still see the prompt on their terminal (stderr), CI consumers
// redirecting stdout no longer get prose mixed with JSON.
var (
	bootstrapPromptIn  io.Reader = os.Stdin
	bootstrapPromptOut io.Writer = os.Stderr

	// bootstrapPromptIsTTY reports whether the prompt seam is connected
	// to a terminal. In production, checks os.Stdin's fd. In tests, can
	// be overridden to simulate TTY / non-TTY scenarios.
	bootstrapPromptIsTTY = func() bool {
		return term.IsTerminal(int(os.Stdin.Fd()))
	}
)

// resolveBootstrapConsent decides what to do when a provider returned
// a BootstrapRequiredError. The policy:
//
//   - --no-bootstrap (deny) → Deny.
//   - --bootstrap (allow)   → Run.
//   - Neither flag + non-TTY → Deny (the run is likely automated).
//   - Neither flag + TTY     → interactive prompt ([y/N/s]).
func resolveBootstrapConsent(boot bootstrapMode, brerr *provider.BootstrapRequiredError) bootDecision {
	switch {
	case boot.Deny:
		return bootDecisionDeny
	case boot.Allow:
		return bootDecisionRun
	case !bootstrapPromptIsTTY():
		return bootDecisionDeny
	default:
		return interactiveBootstrapPrompt(brerr)
	}
}

// interactiveBootstrapPrompt shows the script and side-effect summary,
// then accepts [y/N/s] from the user. Separated so tests can override
// input/output.
func interactiveBootstrapPrompt(brerr *provider.BootstrapRequiredError) bootDecision {
	// Prompt output is best-effort: a broken stderr while prompting for
	// consent is a non-recoverable UX bug, not a flow error to surface.
	pl := func(args ...any) { fmt.Fprintln(bootstrapPromptOut, args...) }
	pf := func(format string, args ...any) { fmt.Fprintf(bootstrapPromptOut, format, args...) }
	pp := func(s string) { fmt.Fprint(bootstrapPromptOut, s) }

	pl()
	pf("hams: %s is required by provider %q but not installed.\n",
		brerr.Binary, brerr.Provider)
	pl()
	pl("The following bootstrap script will run:")
	pl("  " + brerr.Script)
	pl()
	pl("Expected side effects:")
	pl("  - prompts for sudo password (Homebrew / installer requirement)")
	if runtime.GOOS == "darwin" {
		pl("  - may trigger macOS Xcode Command Line Tools install (~1GB, can hang on a GUI dialog)")
	}
	pl("  - downloads from raw.githubusercontent.com (blocked by some corporate proxies)")
	pl()
	pl("Proceed?  [y]es  [N]o (default, exits)  [s]kip-this-provider")
	pp("> ")

	reader := bufio.NewReader(bootstrapPromptIn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return bootDecisionDeny
	}
	answer := strings.ToLower(strings.TrimSpace(line))

	switch answer {
	case "y", "yes":
		return bootDecisionRun
	case "s", "skip":
		return bootDecisionSkipProvider
	default:
		return bootDecisionDeny
	}
}

// hamsfilePresent reports whether a provider has a committed hamsfile
// (either shared or local) for the active profile. Used to distinguish
// "user declared brew resources" (bootstrap failure = fatal) from
// "only state file lingers" (bootstrap failure = silent skip).
func hamsfilePresent(profileDir string, manifest *provider.Manifest) bool {
	prefix := provider.ManifestFilePrefix(*manifest)
	_, errMain := os.Stat(filepath.Join(profileDir, prefix+".hams.yaml"))
	_, errLocal := os.Stat(filepath.Join(profileDir, prefix+".hams.local.yaml"))
	return errMain == nil || errLocal == nil
}

// buildBootstrapFailureSuggestions produces the list of actionable
// remedies attached to the UserFacingError returned when bootstrap
// fails. For each provider whose Bootstrap surfaced a
// BootstrapRequiredError and whose consent was denied, we surface the
// exact install script (verbatim from the manifest) and the
// --bootstrap remedy. Per the builtin-providers spec scenario
// "Bootstrap emits actionable error when --bootstrap is not set":
// the error body SHALL name the missing binary, the exact script text,
// AND the copy-pasteable remedy.
func buildBootstrapFailureSuggestions(denied []*provider.BootstrapRequiredError) []string {
	if len(denied) == 0 {
		return []string{
			"Ensure required tools are installed or remove the hamsfile entries",
			"Use '--only' / '--except' to skip specific providers",
		}
	}
	suggestions := make([]string, 0, 2+3*len(denied))
	for _, brerr := range denied {
		suggestions = append(suggestions,
			fmt.Sprintf("%s (%s): install manually with:", brerr.Provider, brerr.Binary),
			"  "+brerr.Script,
		)
	}
	suggestions = append(suggestions,
		"Or re-run hams with --bootstrap to auto-install missing prerequisites:",
		"  hams apply --bootstrap [other flags...]",
	)
	return suggestions
}
