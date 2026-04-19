package defaults

import (
	"path/filepath"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/baseprovider"
	"github.com/zthxxx/hams/internal/state"
)

// tagCLI is the default hamsfile tag for defaults entries added via
// the CLI auto-record path.
const tagCLI = "cli"

func (p *Provider) loadOrCreateHamsfile(hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	return baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
}

// statePath returns the absolute path to defaults' state file for the
// currently active machine.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads the defaults state file for the
// currently active machine, returning a fresh empty document when the
// file does not yet exist. A read error is treated the same as
// "missing" — the caller's next Save() will overwrite the file with a
// clean one rather than blocking the user on an unrelated
// disk/permission issue.
func (p *Provider) loadOrCreateStateFile(flags *provider.GlobalFlags) *state.File {
	cfg := p.effectiveConfig(flags)
	path := p.statePath(flags)
	sf, err := state.Load(path)
	if err != nil {
		return state.New(p.Manifest().Name, cfg.MachineID)
	}
	return sf
}
