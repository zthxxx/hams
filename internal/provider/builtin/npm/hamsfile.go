package npm

import (
	"path/filepath"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/i18n"
	"github.com/zthxxx/hams/internal/provider"
)

// tagCLI is the default hamsfile tag for packages recorded by the
// CLI-first `hams npm install <pkg>` path.
const tagCLI = "cli"

// loadOrCreateHamsfile reads the npm.hams.yaml (or npm.hams.local.yaml when
// --hams-local is set) for the active profile, creating an empty document if
// the file does not yet exist. Mirrors apt's implementation.
func (p *Provider) loadOrCreateHamsfile(hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	path, err := p.hamsfilePath(hamsFlags, flags)
	if err != nil {
		return nil, err
	}
	return hamsfile.LoadOrCreateEmpty(path)
}

func (p *Provider) hamsfilePath(hamsFlags map[string]string, flags *provider.GlobalFlags) (string, error) {
	cfg := p.effectiveConfig(flags)
	if cfg.StorePath == "" {
		return "", hamserr.NewUserError(hamserr.ExitUsageError,
			i18n.T(i18n.ProviderNoStoreConfigured),
			i18n.T(i18n.ProviderNoStoreConfiguredHint),
		)
	}

	suffix := ".hams.yaml"
	if _, ok := hamsFlags["local"]; ok {
		suffix = ".hams.local.yaml"
	}

	return filepath.Join(cfg.ProfileDir(), p.Manifest().FilePrefix+suffix), nil
}

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
