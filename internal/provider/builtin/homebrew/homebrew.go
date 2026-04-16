// Package homebrew wraps the Homebrew package manager for macOS and Linux.
package homebrew

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

const (
	// cliName is the Homebrew provider's manifest + CLI name.
	cliName = "brew"
	// brewDisplayName is the human-readable display name.
	brewDisplayName = "Homebrew"
	// tagCLI is the default hamsfile tag for CLI (non-cask, non-tap) brew formulas.
	tagCLI = "cli"
)

// BrewResource holds provider-specific data for a Homebrew action.
type BrewResource struct {
	IsCask bool
}

// Provider implements the Homebrew package manager provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new Homebrew provider wired with a real CmdRunner.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
// Bootstrap uses the legacy brewBinaryLookup + envPathAugment
// package-level seams (preserved for the existing install.sh retry
// flow tests); the CmdRunner covers the post-bootstrap operations
// (list formulae/casks/taps, install, uninstall, tap).
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the Homebrew provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   brewDisplayName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassPackage,
		DependsOn: []provider.DependOn{
			{
				Provider: "bash",
				Script:   `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`,
			},
		},
		FilePrefix: brewDisplayName,
	}
}

// brewBinaryLookup is the PATH-check seam Bootstrap uses. Swapped in
// tests to simulate "brew missing" / "brew present" without mutating the
// host's real PATH. Production value is exec.LookPath.
var brewBinaryLookup = exec.LookPath

// brewInstallLocations are the canonical directories where the official
// `install.sh` drops the brew binary. After a fresh install.sh, the
// user's login shell would source `brew shellenv` and pick these up,
// but the hams process's own $PATH won't have them — so we prepend them
// here on the retry path. Order matches what `install.sh` itself checks.
var brewInstallLocations = []string{
	"/opt/homebrew/bin",              // macOS Apple Silicon
	"/usr/local/bin",                 // macOS Intel (usually already on $PATH)
	"/home/linuxbrew/.linuxbrew/bin", // Linuxbrew
}

// envPathAugment is the env-mutation seam Bootstrap uses to surface a
// post-install-sh brew onto the process $PATH. Swapped in tests to
// assert path augmentation without mutating the test harness's env.
// Production value mutates the live process env via os.Setenv.
var envPathAugment = func(additions []string) {
	existing := os.Getenv("PATH")
	for _, dir := range additions {
		if strings.Contains(existing, dir) {
			continue
		}
		existing = dir + string(os.PathListSeparator) + existing
	}
	if err := os.Setenv("PATH", existing); err != nil {
		slog.Warn("failed to set PATH for brew lookup", "error", err)
	}
}

// Bootstrap reports whether brew is installed. A missing binary is
// signaled via provider.BootstrapRequiredError (which wraps
// provider.ErrBootstrapRequired); the CLI orchestrator decides whether to
// run the manifest-declared install script based on --bootstrap / TTY
// prompt. Bootstrap itself NEVER executes a network install.
//
// On a retry after RunBootstrap succeeded, we proactively prepend the
// canonical install locations to $PATH before the second LookPath —
// otherwise users on a fresh Mac/Linux would hit "still unavailable
// after bootstrap" because `install.sh` writes to /opt/homebrew/bin
// (or linuxbrew equivalent), which the hams process never sourced.
func (p *Provider) Bootstrap(_ context.Context) error {
	if _, err := brewBinaryLookup("brew"); err == nil {
		return nil
	}
	// Fallback: maybe install.sh just ran and brew is sitting in one of
	// the canonical locations but not on our $PATH. Augment and retry.
	envPathAugment(brewInstallLocations)
	if _, err := brewBinaryLookup("brew"); err == nil {
		slog.Info("Homebrew found after PATH augmentation",
			"paths", strings.Join(brewInstallLocations, ":"))
		return nil
	}

	manifest := p.Manifest()
	script := ""
	if len(manifest.DependsOn) > 0 {
		script = manifest.DependsOn[0].Script
	}
	slog.Info("Homebrew not found on PATH; bootstrap consent required",
		"provider", manifest.Name, "binary", "brew")
	return &provider.BootstrapRequiredError{
		Provider: manifest.Name,
		Binary:   "brew",
		Script:   script,
	}
}

// Probe queries brew for installed formulae, casks, and taps.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	installed, err := p.listInstalled(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing installed brew packages: %w", err)
	}

	taps, err := p.runner.ListTaps(ctx)
	if err != nil {
		slog.Debug("listing taps failed, ignoring", "error", err)
	}
	for _, tap := range taps {
		installed[tap] = ""
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

// listInstalled merges formulae and casks via the runner. Cask-listing
// errors are logged and ignored (brew returns non-zero when zero casks
// are installed).
func (p *Provider) listInstalled(ctx context.Context) (map[string]string, error) {
	formulae, err := p.runner.ListFormulae(ctx)
	if err != nil {
		return nil, err
	}
	casks, err := p.runner.ListCasks(ctx)
	if err != nil {
		slog.Debug("listing casks failed, ignoring", "error", err)
	}
	maps.Copy(formulae, casks)
	return formulae, nil
}

// isTapFormat returns true if the package name looks like a tap (user/repo format without formula).
func isTapFormat(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 2 && !strings.Contains(parts[1], ".")
}

// Plan computes actions for Homebrew packages.
// Tags named "cask" in the hamsfile are marked so Apply can inject
// --cask. Hamsfile-declared hooks are attached to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	caskSet := caskApps(desired)
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	for i := range actions {
		if caskSet[actions[i].ID] {
			actions[i].Resource = BrewResource{IsCask: true}
		}
	}
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply installs a brew package. If the action carries a BrewResource with IsCask set,
// --cask is appended to the install command via the runner.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	isCask := false
	if res, ok := action.Resource.(BrewResource); ok && res.IsCask {
		isCask = true
	}
	slog.Info("brew install", "package", action.ID, "cask", isCask)
	return p.runner.Install(ctx, action.ID, isCask)
}

// caskApps returns the set of app names that appear under a "cask" tag in the hamsfile.
func caskApps(f *hamsfile.File) map[string]bool {
	result := make(map[string]bool)
	if f.Root == nil || len(f.Root.Content) == 0 {
		return result
	}

	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return result
	}

	for i := 0; i < len(doc.Content)-1; i += 2 {
		tagName := doc.Content[i].Value
		if tagName != "cask" {
			continue
		}
		seq := doc.Content[i+1]
		if seq.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range seq.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k < len(item.Content)-1; k += 2 {
				key := item.Content[k].Value
				if key == "app" || key == "urn" {
					result[item.Content[k+1].Value] = true
					break
				}
			}
		}
	}
	return result
}

// Remove uninstalls a brew package.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("brew uninstall", "package", resourceID)
	return p.runner.Uninstall(ctx, resourceID)
}

// List returns packages with diff between Hamsfile (desired) and state (observed).
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for the brew provider.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "install":
		return p.handleInstall(ctx, remaining, hamsFlags, flags)
	case "remove", "uninstall":
		return p.handleRemove(ctx, remaining, hamsFlags, flags)
	case "list":
		return p.handleList(hamsFlags, flags)
	case "tap":
		return p.handleTap(ctx, remaining, hamsFlags, flags)
	default:
		// Passthrough to brew.
		slog.Debug("passthrough to brew", "args", args)
		return provider.WrapExecPassthrough(ctx, "brew", args, nil)
	}
}

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return brewDisplayName }

func (p *Provider) handleList(hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	fmt.Println("Homebrew managed packages:")

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}

	cfg := p.effectiveConfig(flags)
	statePath := filepath.Join(cfg.StateDir(), "brew.state.yaml")
	sf, err := state.Load(statePath)
	if err != nil {
		// No state yet — show all desired as additions.
		sf = state.New("brew", cfg.MachineID)
	}

	diff := provider.DiffDesiredVsState(hf, sf)
	if flags.JSON {
		out, jsonErr := provider.FormatDiffJSON(&diff)
		if jsonErr != nil {
			return jsonErr
		}
		fmt.Println(out)
	} else {
		fmt.Print(provider.FormatDiff(&diff))
	}
	return nil
}

func (p *Provider) handleTap(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"brew tap requires a repository name",
			"Usage: hams brew tap <user/repo>",
		)
	}

	repo := args[0]
	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: brew tap %s\n", repo)
		return nil
	}

	if err := provider.WrapExecPassthrough(ctx, "brew", []string{"tap", repo}, nil); err != nil {
		return err
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}

	hf.AddApp("tap", repo, "")
	return hf.Write()
}

func (p *Provider) handleInstall(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"brew install requires a package name",
			"Usage: hams brew install <package> [--cask] [--hams-tag=<tag>]",
			"To install all recorded packages, use: hams apply --only=brew",
		)
	}

	packages := packageArgs(args)
	tag := parseInstallTag(hamsFlags)
	// If --cask is present in args and no explicit tag was set, use "cask" as the tag.
	if tag == tagCLI && hasCaskFlag(args) {
		tag = "cask"
	}
	// Auto-detect tap format (user/repo with exactly one slash, no formula suffix).
	if tag == tagCLI && len(packages) > 0 && isTapFormat(packages[0]) {
		tag = "tap"
	}
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

	if err := provider.WrapExecPassthrough(ctx, "brew", append([]string{"install"}, args...), nil); err != nil {
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

func (p *Provider) handleRemove(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
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

	if err := provider.WrapExecPassthrough(ctx, "brew", append([]string{"uninstall"}, args...), nil); err != nil {
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
	tag := tagCLI
	if raw := strings.TrimSpace(hamsFlags["tag"]); raw != "" {
		tag = strings.TrimSpace(strings.Split(raw, ",")[0])
	}

	if tag == "" {
		return tagCLI
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

func hasCaskFlag(args []string) bool {
	return slices.Contains(args, "--cask")
}
