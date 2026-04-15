package provider

import (
	"os"
	"path/filepath"
)

// HasArtifacts reports whether the given provider has any local artifacts
// under the active profile/state directories — a hamsfile (main or local)
// or a state file. This is the stage-1 provider filter consulted by both
// `hams apply` and `hams refresh` before any Bootstrap/Probe/Plan/Execute
// call is made.
//
// Callers pass the resolved profile directory and state directory. An
// empty path is treated as "no artifact there".
//
// Returns true if ANY of these files exist:
//   - <profileDir>/<FilePrefix>.hams.yaml
//   - <profileDir>/<FilePrefix>.hams.local.yaml
//   - <stateDir>/<FilePrefix>.state.yaml
func HasArtifacts(p Provider, profileDir, stateDir string) bool {
	prefix := manifestFilePrefix(p.Manifest())
	candidates := []string{}
	if profileDir != "" {
		candidates = append(candidates,
			filepath.Join(profileDir, prefix+".hams.yaml"),
			filepath.Join(profileDir, prefix+".hams.local.yaml"),
		)
	}
	if stateDir != "" {
		candidates = append(candidates,
			filepath.Join(stateDir, prefix+".state.yaml"),
		)
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// FilterByArtifacts returns the subset of providers that have at least
// one local artifact (hamsfile or state file). This is the stage-1
// filter; stage 2 (--only / --except) runs on the returned slice.
//
// Preserves input order. Returns an empty slice (never nil) if no
// provider qualifies, so callers can distinguish "no artifacts at all"
// from "error loading artifacts".
func FilterByArtifacts(providers []Provider, profileDir, stateDir string) []Provider {
	result := make([]Provider, 0, len(providers))
	for _, p := range providers {
		if HasArtifacts(p, profileDir, stateDir) {
			result = append(result, p)
		}
	}
	return result
}

// IsStateOnly reports whether the given provider has a state file but
// neither a hamsfile nor a local-override hamsfile. Used by `hams apply`
// to identify providers whose desired state has been undeclared (the
// hamsfile was deleted) but whose state file still tracks resources.
func IsStateOnly(p Provider, profileDir, stateDir string) bool {
	prefix := manifestFilePrefix(p.Manifest())
	hams := filepath.Join(profileDir, prefix+".hams.yaml")
	hamsLocal := filepath.Join(profileDir, prefix+".hams.local.yaml")
	if _, err := os.Stat(hams); err == nil {
		return false
	}
	if _, err := os.Stat(hamsLocal); err == nil {
		return false
	}
	statePath := filepath.Join(stateDir, prefix+".state.yaml")
	_, err := os.Stat(statePath)
	return err == nil
}

// FilterStateOnlyWithoutPrune removes providers that are in the
// state-only position (have a state file but no hamsfile) when
// pruneOrphans is false. The default `hams apply` semantics promise
// that state-only providers are NOT touched without `--prune-orphans`;
// removing them here (BEFORE refresh + bootstrap + plan/execute) keeps
// `ProbeAll` from rewriting their state file as a side-effect of the
// "did we select them" stage-1 filter. When pruneOrphans is true, all
// providers pass through unchanged so the prune-reconcile path runs.
//
// Preserves input order. The dropped providers are reported via the
// returned `dropped` slice so the caller can debug-log them.
func FilterStateOnlyWithoutPrune(providers []Provider, profileDir, stateDir string, pruneOrphans bool) (kept, dropped []Provider) {
	if pruneOrphans {
		return providers, nil
	}
	kept = make([]Provider, 0, len(providers))
	for _, p := range providers {
		if IsStateOnly(p, profileDir, stateDir) {
			dropped = append(dropped, p)
			continue
		}
		kept = append(kept, p)
	}
	return kept, dropped
}

// ManifestFilePrefix returns the canonical file prefix for a provider's
// hamsfile and state file: `Manifest.FilePrefix` when set, falling back
// to `Manifest.Name`. Single source of truth — `internal/cli` consumes
// this directly so the prefix logic does not drift across packages.
func ManifestFilePrefix(m Manifest) string { //nolint:gocritic // simple helper, copy is acceptable
	if m.FilePrefix != "" {
		return m.FilePrefix
	}
	return m.Name
}

// manifestFilePrefix is a private wrapper for in-package callers that
// want the short name. ManifestFilePrefix is the canonical exported API.
func manifestFilePrefix(m Manifest) string { return ManifestFilePrefix(m) } //nolint:gocritic // simple helper, copy is acceptable
