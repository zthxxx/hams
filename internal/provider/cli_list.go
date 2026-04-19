package provider

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// ValidateProfileDirExists returns the resolved profile directory for
// cfg when it exists on disk, otherwise returns an ExitUsageError
// naming the missing path. Used by every `hams <provider> list` code
// path — both the shared HandleListCmd and per-provider custom
// handlers (git-clone, git-config, homebrew, …) — so a typo'd
// --profile fails fast with the same actionable error everywhere.
//
// Symmetric with the top-level `hams list --profile=Typo` check
// (cycle 217) and the apply/refresh `flags.Profile != ""` guards
// (cycles 92/93).
//
// Cycle 220 introduced the guard inline; cycle 222 extracted it so
// custom-handler providers (git-clone, git-config, homebrew) can
// share the exact error shape without duplication. If a future
// provider bypasses HandleListCmd, adopting this helper is one call.
//
// Order of failure modes (both are ExitUsageError):
//  1. cfg.StorePath empty → "no store directory configured" (this
//     matches what loadOrCreateHamsfile returns, so the error is
//     stable regardless of whether the caller validates profile
//     first or goes straight to hamsfile loading).
//  2. profile dir missing → "profile %q not found at %s".
func ValidateProfileDirExists(cfg *config.Config) (string, error) {
	if cfg == nil || cfg.StorePath == "" {
		return "", hamserr.NewUserError(hamserr.ExitUsageError,
			"no store directory configured",
			"Set store_path in hams config or pass --store",
		)
	}
	profileDir := cfg.ProfileDir()
	info, statErr := os.Stat(profileDir)
	if statErr == nil && info.IsDir() {
		return profileDir, nil
	}
	return "", hamserr.NewUserError(hamserr.ExitUsageError,
		fmt.Sprintf("profile %q not found at %s", cfg.ProfileTag, profileDir),
		"Check available profiles: ls "+cfg.StorePath,
		"Or create this profile: mkdir -p "+profileDir,
	)
}

// HandleListCmd is the shared implementation of the `hams <provider>
// list` CLI verb. It loads the provider's hamsfile + state file
// (tolerating absent files by creating empty in-memory doubles),
// invokes p.List, and prints the human-readable diff to stdout.
// Mirrors the output of `hams list --only=<provider>` so users reach
// the same information through either entry point.
//
// The builtin-providers spec table for every Package-class (and many
// non-Package-class) providers promises "Diff view" for `hams
// <provider> list`, but pre-cycle-214 the CLI dispatchers fell through
// to Passthrough (formerly WrapExecPassthrough), which tried to exec the underlying tool's
// `list` subcommand. That either errored (cargo, vscodeext, goinstall)
// or emitted raw unrelated output (apt, mas) instead of the
// hams-tracked diff. Cycle 213 fixed ansible inline; cycle 214 pulled
// the logic out here so the rest of the providers can adopt the fix
// by adding one `case "list"` branch.
//
// No JSON output — callers wanting machine-parseable output should use
// `hams --json list --only=<provider>` which already honors the --json
// flag at the top-level list command.
func HandleListCmd(ctx context.Context, p Provider, cfg *config.Config) error {
	if cfg == nil || cfg.StorePath == "" {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"no store directory configured",
			"Set store_path in hams config or pass --store",
		)
	}

	manifest := p.Manifest()
	prefix := manifest.FilePrefix

	profileDir, profileErr := ValidateProfileDirExists(cfg)
	if profileErr != nil {
		return profileErr
	}

	// Cycle 216: read-only load. hamsfile.LoadOrCreateEmpty would call
	// os.MkdirAll for the profile directory when the hamsfile is
	// missing — fine for install/remove (first-use bootstrap) but a
	// surprising side effect for `list`, which users expect to be a
	// pure query. Use hamsfile.Read + fallback to NewEmpty so a list
	// call against a fresh store never writes to disk.
	hfPath := filepath.Join(profileDir, prefix+".hams.yaml")
	hf, err := hamsfile.Read(hfPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("loading %s hamsfile: %w", manifest.Name, err)
		}
		hf = hamsfile.NewEmpty(hfPath)
	}

	statePath := filepath.Join(cfg.StateDir(), prefix+".state.yaml")
	sf, loadErr := state.Load(statePath)
	if loadErr != nil {
		if !errors.Is(loadErr, fs.ErrNotExist) {
			return fmt.Errorf("loading %s state: %w", manifest.Name, loadErr)
		}
		sf = state.New(manifest.Name, cfg.MachineID)
	}

	output, listErr := p.List(ctx, hf, sf)
	if listErr != nil {
		return listErr
	}
	fmt.Print(output)
	return nil
}
