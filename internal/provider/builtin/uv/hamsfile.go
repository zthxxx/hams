package uv

import (
	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/baseprovider"
)

// tagCLI is the default hamsfile tag for tools recorded by the
// CLI-first `hams uv install <tool>` path.
const tagCLI = "cli"

func (p *Provider) hamsfilePath(hamsFlags map[string]string, flags *provider.GlobalFlags) (string, error) {
	return baseprovider.HamsfilePath(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
}

func (p *Provider) effectiveConfig(flags *provider.GlobalFlags) *config.Config {
	return baseprovider.EffectiveConfig(p.cfg, flags)
}
