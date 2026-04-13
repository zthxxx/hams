// Package homebrew wraps the Homebrew package manager for macOS and Linux.
package homebrew

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// Provider implements the Homebrew package manager provider.
type Provider struct {
	cfg *config.Config
}

// New creates a new Homebrew provider.
func New(cfg *config.Config) *Provider {
	return &Provider{cfg: cfg}
}

// Manifest returns the Homebrew provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "brew",
		DisplayName:   "Homebrew",
		Platform:      provider.PlatformAll,
		ResourceClass: provider.ClassPackage,
		DependsOn: []provider.DependOn{
			{
				Provider: "bash",
				Script:   `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`,
			},
		},
		FilePrefix: "Homebrew",
	}
}

// Bootstrap checks if brew is available and installs it if not.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("brew"); err == nil {
		return nil
	}
	slog.Info("Homebrew not found, bootstrapping via bash provider")
	return fmt.Errorf("homebrew not installed; run the bootstrap script first")
}

// Probe queries brew for installed formulae and casks.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	installed, err := listInstalled(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing installed brew packages: %w", err)
	}

	var results []provider.ProbeResult
	for id := range sf.Resources {
		if sf.Resources[id].State == state.StateRemoved {
			continue
		}

		version, found := installed[id]
		if found {
			results = append(results, provider.ProbeResult{
				ID:      id,
				State:   state.StateOK,
				Version: version,
			})
		} else {
			results = append(results, provider.ProbeResult{
				ID:    id,
				State: state.StateFailed,
			})
		}
	}

	return results, nil
}

// Plan computes actions for Homebrew packages.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply installs a brew package.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	args := []string{"install", action.ID}
	slog.Info("brew install", "package", action.ID)
	return provider.WrapExecPassthrough(ctx, "brew", args, nil)
}

// Remove uninstalls a brew package.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("brew uninstall", "package", resourceID)
	return provider.WrapExecPassthrough(ctx, "brew", []string{"uninstall", resourceID}, nil)
}

// List returns installed packages with their status.
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		version := r.Version
		if version == "" {
			version = "?"
		}
		fmt.Fprintf(&sb, "  %-30s %-10s %s\n", id, r.State, version)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for the brew provider.
func (p *Provider) HandleCommand(args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install":
		return p.handleInstall(remaining, hamsFlags, flags)
	case "remove", "uninstall":
		return p.handleRemove(remaining, hamsFlags, flags)
	case "list":
		fmt.Println("Homebrew managed packages:")
		// TODO: show diff between hamsfile and state.
		return nil
	default:
		// Passthrough to brew.
		slog.Debug("passthrough to brew", "args", args)
		return provider.WrapExecPassthrough(context.Background(), "brew", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "brew" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "Homebrew" }

func (p *Provider) handleInstall(args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"brew install requires a package name",
			"Usage: hams brew install <package> [--cask] [--hams-tag=<tag>]",
			"To install all recorded packages, use: hams apply --only=brew",
		)
	}

	tag := parseInstallTag(hamsFlags)
	packages := packageArgs(args)
	if len(packages) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"brew install requires at least one package name",
			"Usage: hams brew install <package> [--cask] [--hams-tag=<tag>]",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would install: brew install %s\n", strings.Join(args, " "))
		return nil
	}

	if err := provider.WrapExecPassthrough(context.Background(), "brew", append([]string{"install"}, args...), nil); err != nil {
		return err
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}

	for _, pkg := range packages {
		hf.AddApp(tag, pkg, "")
	}

	return hf.Write()
}

func (p *Provider) handleRemove(args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"brew remove requires a package name",
			"Usage: hams brew remove <package>",
		)
	}

	packages := packageArgs(args)
	if len(packages) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"brew remove requires at least one package name",
			"Usage: hams brew remove <package>",
		)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove: brew uninstall %s\n", strings.Join(args, " "))
		return nil
	}

	if err := provider.WrapExecPassthrough(context.Background(), "brew", append([]string{"uninstall"}, args...), nil); err != nil {
		return err
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}

	for _, pkg := range packages {
		hf.RemoveApp(pkg)
	}

	return hf.Write()
}

// listInstalled returns a map of installed package name → version.
func listInstalled(ctx context.Context) (map[string]string, error) {
	// List formulae.
	formulae, err := listByType(ctx, "--formula")
	if err != nil {
		return nil, err
	}

	// List casks.
	casks, err := listByType(ctx, "--cask")
	if err != nil {
		// Cask list might fail if no casks installed. That's OK.
		slog.Debug("listing casks failed, ignoring", "error", err)
	}

	// Merge.
	maps.Copy(formulae, casks)

	return formulae, nil
}

func listByType(ctx context.Context, typeFlag string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, "brew", "info", "--json=v2", "--installed", typeFlag) //nolint:gosec // typeFlag is --formula or --cask, not user input
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("brew info %s: %w", typeFlag, err)
	}

	var data struct {
		Formulae []struct {
			Name              string `json:"name"`
			InstalledVersions []struct {
				Version string `json:"version"`
			} `json:"installed"`
		} `json:"formulae"`
		Casks []struct {
			Token   string `json:"token"`
			Version string `json:"version"`
		} `json:"casks"`
	}

	if err := json.Unmarshal(output, &data); err != nil {
		return nil, fmt.Errorf("parsing brew JSON: %w", err)
	}

	result := make(map[string]string)
	for _, f := range data.Formulae {
		version := ""
		if len(f.InstalledVersions) > 0 {
			version = f.InstalledVersions[0].Version
		}
		result[f.Name] = version
	}
	for _, c := range data.Casks {
		result[c.Token] = c.Version
	}

	return result, nil
}

func (p *Provider) loadOrCreateHamsfile(hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	path, err := p.hamsfilePath(hamsFlags, flags)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); err == nil {
		return hamsfile.Read(path)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat hamsfile %s: %w", path, err)
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

func parseInstallTag(hamsFlags map[string]string) string {
	tag := "cli"
	if raw := strings.TrimSpace(hamsFlags["tag"]); raw != "" {
		tag = strings.TrimSpace(strings.Split(raw, ",")[0])
	}

	if tag == "" {
		return "cli"
	}

	return tag
}

func packageArgs(args []string) []string {
	var packages []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		packages = append(packages, arg)
	}
	return packages
}
