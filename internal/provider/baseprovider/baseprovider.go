// Package baseprovider hosts the cross-provider helpers that every builtin
// CLI-first auto-record provider duplicates. Pre-baseprovider, each of the
// 13 builtin providers shipped a hand-written hamsfile.go with the same
// `loadOrCreateHamsfile`, `hamsfilePath`, and `effectiveConfig` helpers,
// only the package name and the FilePrefix differing. Centralizing the
// helpers shrinks the per-provider boilerplate to one file (the package's
// runner + Provider struct + Manifest) and locks the contract: every
// provider's hamsfile resolution rules stay in lockstep.
//
// Usage from a provider package:
//
//	hf, err := baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
//	if err != nil { return err }
//
// The package is intentionally minimal — no struct fields, no Provider
// interface. Each provider keeps its own struct + constructor; the
// helpers operate on the values they need (cfg + filePrefix + flags) so
// adopting them does not require any structural refactor of existing
// providers. The shared base lets new providers focus on the package-
// manager-specific shape and avoid re-implementing the boilerplate.
package baseprovider

import (
	"path/filepath"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/provider"
)

// LoadOrCreateHamsfile returns the parsed hamsfile for the active
// profile, creating an empty document when the file does not yet exist.
// `filePrefix` is the provider's Manifest().FilePrefix.
func LoadOrCreateHamsfile(cfg *config.Config, filePrefix string, hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	path, err := HamsfilePath(cfg, filePrefix, hamsFlags, flags)
	if err != nil {
		return nil, err
	}
	return hamsfile.LoadOrCreateEmpty(path)
}

// HamsfilePath returns the absolute path to the provider's hamsfile
// (or its `.hams.local.yaml` variant when `--hams-local` was passed)
// for the currently active profile.
func HamsfilePath(cfg *config.Config, filePrefix string, hamsFlags map[string]string, flags *provider.GlobalFlags) (string, error) {
	eff := EffectiveConfig(cfg, flags)
	if eff.StorePath == "" {
		return "", hamserr.NewUserError(hamserr.ExitUsageError,
			i18n.T(i18n.ProviderErrNoStore),
			i18n.T(i18n.ProviderErrNoStoreSuggest),
		)
	}
	suffix := ".hams.yaml"
	if _, ok := hamsFlags["local"]; ok {
		suffix = ".hams.local.yaml"
	}
	return filepath.Join(eff.ProfileDir(), filePrefix+suffix), nil
}

// EffectiveConfig returns a copy of cfg overlaid with any per-invocation
// flag overrides (--store, --profile/--tag). The returned pointer is
// safe to mutate without disturbing the original cfg, so callers can
// continue to layer further overlays per-call.
func EffectiveConfig(cfg *config.Config, flags *provider.GlobalFlags) *config.Config {
	if cfg == nil {
		cfg = &config.Config{}
	}
	out := *cfg
	if flags == nil {
		return &out
	}
	if flags.Store != "" {
		out.StorePath = flags.Store
	}
	if flags.Profile != "" {
		out.ProfileTag = flags.Profile
	}
	return &out
}
