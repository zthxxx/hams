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

// manifestFilePrefix returns the canonical file prefix for a provider's
// hamsfile and state file. Kept private to the provider package so we
// don't duplicate the fallback logic across callers.
func manifestFilePrefix(m Manifest) string { //nolint:gocritic // simple helper, copy is acceptable
	if m.FilePrefix != "" {
		return m.FilePrefix
	}
	return m.Name
}
