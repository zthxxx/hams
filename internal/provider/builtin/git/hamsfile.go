package git

import (
	"path/filepath"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/baseprovider"
	"github.com/zthxxx/hams/internal/state"
)

// tagCLI is the default hamsfile tag for git-config entries added via
// the CLI auto-record path.
const tagCLI = "cli"

func (p *ConfigProvider) loadOrCreateHamsfile(hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	return baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
}

// statePath returns the absolute path to git-config's state file for
// the currently active machine under the active profile.
func (p *ConfigProvider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads the git-config state file for the
// currently active machine, returning a fresh empty document when the
// file does not yet exist. A read error is treated the same as
// "missing" — the caller's next Save() will overwrite a corrupted or
// unreadable state file with a clean one rather than blocking the
// user on an unrelated disk/permission issue.
func (p *ConfigProvider) loadOrCreateStateFile(flags *provider.GlobalFlags) *state.File {
	cfg := p.effectiveConfig(flags)
	path := p.statePath(flags)
	sf, err := state.Load(path)
	if err != nil {
		return state.New(p.Manifest().Name, cfg.MachineID)
	}
	return sf
}

func (p *ConfigProvider) effectiveConfig(flags *provider.GlobalFlags) *config.Config {
	return baseprovider.EffectiveConfig(p.cfg, flags)
}
