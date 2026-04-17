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
// to WrapExecPassthrough, which tried to exec the underlying tool's
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

	// Cycle 220: symmetric with cycles 92/93/217. When the resolved
	// profile directory doesn't exist, fail hard with a clear error
	// naming the missing path. Otherwise `hams <provider> list
	// --profile=Typo` silently printed "No entries tracked" —
	// indistinguishable from a real empty-profile scenario. This
	// mirrors what the top-level `hams list --profile=Typo` does
	// (cycle 217) via a path-exists check: the user's typo surfaces
	// immediately instead of being hidden behind FormatDiff's
	// zero-entry message.
	profileDir := cfg.ProfileDir()
	if info, statErr := os.Stat(profileDir); statErr != nil || !info.IsDir() {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("profile %q not found at %s", cfg.ProfileTag, profileDir),
			"Check available profiles: ls "+cfg.StorePath,
			"Or create this profile: mkdir -p "+profileDir,
		)
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
