package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/provider"
)

// ensureProfileConfigured fills in any missing profile_tag / machine_id
// fields on cfg. Three paths, in priority order:
//
//  1. First-run auto-init (NEW): when `${HAMS_CONFIG_HOME}/hams.config.yaml`
//     does not exist on disk AND the user supplied `--tag` (or the
//     legacy `--profile` alias), hams seeds the config with the
//     supplied tag + a derived machine_id (env or hostname, via
//     config.DeriveMachineID), then continues. This is the "fresh
//     machine" workflow: one-shot `hams apply --from-repo=X --tag=Y`
//     with no manual config editing. Fires regardless of TTY —
//     explicit CLI input is sufficient consent.
//  2. TTY prompt: interactive terminals get the legacy
//     promptProfileInit flow, identical to pre-cycle behavior.
//  3. Non-TTY failure: surface a UserFacingError naming the missing
//     keys + concrete remediation instead of reading EOF from a
//     pipe.
//
// Auto-init is intentionally scoped to the "no global config exists"
// case: if a user deliberately wrote a config with `profile_tag:
// macOS` but left `machine_id:` blank, that is a declarative choice
// and hams still surfaces the missing-machine_id error (path 3).
// The auto-init is for pristine machines only.
//
// Lives in autoinit.go (extracted from apply.go in
// 2026-04-19-cli-modularization) so the first-run UX is grep-locatable
// by name and isolated from the apply-pipeline orchestration. Any
// future "apply on a fresh machine" UX change should touch only this
// helper + its dedicated autoinit_test.go.
func ensureProfileConfigured(paths config.Paths, storePath string, cfg *config.Config, flags *provider.GlobalFlags) error {
	cliTag, _ := config.ResolveCLITagOverride(flags.Tag, flags.Profile) //nolint:errcheck // ambiguity already checked upstream
	globalConfigPresent, _ := statFile(paths.GlobalConfigPath())
	if cliTag != "" && !globalConfigPresent {
		cfg.ProfileTag = cliTag
		if writeErr := config.WriteConfigKey(paths, storePath, "profile_tag", cliTag); writeErr != nil {
			slog.Warn("failed to persist profile_tag", "error", writeErr)
		}
		mid := config.DeriveMachineID()
		cfg.MachineID = mid
		if writeErr := config.WriteConfigKey(paths, storePath, "machine_id", mid); writeErr != nil {
			slog.Warn("failed to persist machine_id", "error", writeErr)
		}
		// Log records stay in English (per CLAUDE.md: "Log records
		// do not require i18n"). i18n.T use here would route the
		// operator-facing log through the user's UI locale, which
		// breaks grep'ing across CI log streams that aggregate logs
		// from multi-locale boxes.
		slog.Info("auto-initialized global config", "profile_tag", cliTag, "machine_id", mid)
		return nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // Fd() returns uintptr that fits in int on all supported platforms
		// Cycle 252: diagnostic notice goes to stderr, symmetric with
		// promptProfileInit's stderr prompts. Keeps stdout reserved
		// for the primary output (apply summary / JSON).
		fmt.Fprintln(os.Stderr, i18n.T(i18n.BootstrapStatusProfileMissing))
		tag, mid, promptErr := promptProfileInit()
		if promptErr != nil {
			return fmt.Errorf("profile init: %w", promptErr)
		}
		cfg.ProfileTag = tag
		cfg.MachineID = mid
		if writeErr := config.WriteConfigKey(paths, storePath, "profile_tag", tag); writeErr != nil {
			slog.Warn("failed to persist profile_tag", "error", writeErr)
		}
		if writeErr := config.WriteConfigKey(paths, storePath, "machine_id", mid); writeErr != nil {
			slog.Warn("failed to persist machine_id", "error", writeErr)
		}
		return nil
	}

	missing := make([]string, 0, 2)
	if cfg.ProfileTag == "" {
		missing = append(missing, "profile_tag")
	}
	if cfg.MachineID == "" {
		missing = append(missing, "machine_id")
	}
	return hamserr.NewUserError(hamserr.ExitUsageError,
		i18n.Tf(i18n.BootstrapErrNotConfigured, map[string]any{
			"Missing": strings.Join(missing, " and "),
		}),
		"Set them explicitly (example):",
		"  hams config set profile_tag macOS",
		"  hams config set machine_id $(hostname)",
		"Or pass --tag=<tag> on the command line — hams will auto-derive machine_id",
	)
}

// statFile returns (exists, path) for a regular-file check. Used by
// ensureProfileConfigured to detect "first run" state. Directories
// and permission-denied errors are conservatively treated as "exists"
// so auto-init never fires when the path is present-but-unreadable.
func statFile(path string) (exists bool, checkedPath string) {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return true, path
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, path
	}
	// Any other error (permission denied, broken symlink, etc.) →
	// treat as present so we don't auto-init over it.
	return true, path
}
