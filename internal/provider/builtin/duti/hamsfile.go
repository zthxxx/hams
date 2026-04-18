package duti

import (
	"path/filepath"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// tagCLI is the default hamsfile tag for duti entries added via the
// CLI auto-record path.
const tagCLI = "cli"

// loadOrCreateHamsfile reads the duti.hams.yaml (or its
// .hams.local.yaml variant when --hams-local is set) for the active
// profile, creating an empty document if the file does not yet exist.
func (p *Provider) loadOrCreateHamsfile(hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	path, err := p.hamsfilePath(hamsFlags, flags)
	if err != nil {
		return nil, err
	}
	return hamsfile.LoadOrCreateEmpty(path)
}

// hamsfilePath returns the absolute path to duti's hamsfile for the
// currently active profile.
func (p *Provider) hamsfilePath(hamsFlags map[string]string, flags *provider.GlobalFlags) (string, error) {
	cfg := p.effectiveConfig(flags)
	if cfg.StorePath == "" {
		return "", hamserr.NewUserError(hamserr.ExitUsageError,
			i18n.T(i18n.ProviderErrNoStore),
			i18n.T(i18n.ProviderErrNoStoreSuggest),
		)
	}

	suffix := ".hams.yaml"
	if _, ok := hamsFlags["local"]; ok {
		suffix = ".hams.local.yaml"
	}

	return filepath.Join(cfg.ProfileDir(), p.Manifest().FilePrefix+suffix), nil
}

// statePath returns the absolute path to duti's state file for the
// currently active machine.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads duti's state file for the currently
// active machine, returning a fresh empty document when the file
// does not yet exist. A read error is treated the same as "missing"
// so a corrupted state file doesn't block the auto-record path.
func (p *Provider) loadOrCreateStateFile(flags *provider.GlobalFlags) *state.File {
	cfg := p.effectiveConfig(flags)
	path := p.statePath(flags)
	sf, err := state.Load(path)
	if err != nil {
		return state.New(p.Manifest().Name, cfg.MachineID)
	}
	return sf
}

// effectiveConfig returns the provider's config overlaid with any
// per-invocation flags.
func (p *Provider) effectiveConfig(flags *provider.GlobalFlags) *config.Config {
	if p.cfg == nil {
		p.cfg = &config.Config{}
	}
	cfg := *p.cfg
	if flags == nil {
		return &cfg
	}
	if flags.Store != "" {
		cfg.StorePath = flags.Store
	}
	if flags.Profile != "" {
		cfg.ProfileTag = flags.Profile
	}
	return &cfg
}
