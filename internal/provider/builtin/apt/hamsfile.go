package apt

import (
	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/baseprovider"
)

// tagCLI is the default hamsfile tag for apt CLI packages.
const tagCLI = "cli"

// loadOrCreateHamsfile reads the apt.hams.yaml (or apt.hams.local.yaml
// when --hams-local is set) for the active profile, creating an empty
// document if the file does not yet exist.
func (p *Provider) loadOrCreateHamsfile(hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	return baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
}

func (p *Provider) effectiveConfig(flags *provider.GlobalFlags) *config.Config {
	return baseprovider.EffectiveConfig(p.cfg, flags)
}
