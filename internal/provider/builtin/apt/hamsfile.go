package apt

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
)

// tagCLI is the default hamsfile tag for apt CLI packages.
const tagCLI = "cli"

// loadOrCreateHamsfile reads the apt.hams.yaml (or apt.hams.local.yaml when
// --hams-local is set) for the active profile, creating an empty document if
// the file does not yet exist.
func (p *Provider) loadOrCreateHamsfile(hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	path, err := p.hamsfilePath(hamsFlags, flags)
	if err != nil {
		return nil, err
	}

	f, readErr := hamsfile.Read(path)
	if readErr == nil {
		return f, nil
	}
	if !errors.Is(readErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading hamsfile %s: %w", path, readErr)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create profile dir for %s: %w", path, err)
	}

	return &hamsfile.File{
		Path: path,
		Root: &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode, Tag: "!!map"},
			},
		},
	}, nil
}

// hamsfilePath returns the absolute path to the provider's hamsfile (or its
// .local.yaml variant) for the currently active profile.
func (p *Provider) hamsfilePath(hamsFlags map[string]string, flags *provider.GlobalFlags) (string, error) {
	cfg := p.effectiveConfig(flags)
	if cfg.StorePath == "" {
		return "", hamserr.NewUserError(hamserr.ExitUsageError,
			"no store directory configured",
			"Set store_path in hams config or pass --store",
		)
	}

	suffix := ".hams.yaml"
	if _, ok := hamsFlags["local"]; ok {
		suffix = ".hams.local.yaml"
	}

	return filepath.Join(cfg.ProfileDir(), p.Manifest().FilePrefix+suffix), nil
}

// effectiveConfig returns the provider's config overlaid with any per-invocation flags.
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
