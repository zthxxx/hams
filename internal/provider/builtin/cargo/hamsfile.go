package cargo

import (
	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/baseprovider"
)

// tagCLI is the default hamsfile tag for crates recorded by the
// CLI-first `hams cargo install <crate>` path.
const tagCLI = "cli"

// hamsfilePath delegates to baseprovider.HamsfilePath; kept as a
// method so call sites in this package can continue to read
// `p.hamsfilePath(...)` without referring to the runner's manifest.
func (p *Provider) hamsfilePath(hamsFlags map[string]string, flags *provider.GlobalFlags) (string, error) {
	return baseprovider.HamsfilePath(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
}

// effectiveConfig delegates to baseprovider.EffectiveConfig.
func (p *Provider) effectiveConfig(flags *provider.GlobalFlags) *config.Config {
	return baseprovider.EffectiveConfig(p.cfg, flags)
}
