// Package homebrew wraps the Homebrew package manager for macOS and Linux.
package homebrew

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	"github.com/zthxxx/hams/internal/provider/baseprovider"
	"github.com/zthxxx/hams/internal/state"
)

const (
	// cliName is the Homebrew provider's manifest + CLI name.
	cliName = "brew"
	// brewDisplayName is the human-readable display name.
	brewDisplayName = "Homebrew"
	// tagCLI is the default hamsfile tag for CLI (non-cask, non-tap) brew formulas.
	tagCLI = "cli"
	// tagCask is the hamsfile tag for GUI apps installed via `brew install --cask`.
	tagCask = "cask"
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
//
// Cycle 169: membership check splits PATH on the OS path-separator
// and compares each entry exactly. The pre-cycle-169 strings.Contains
// check incorrectly skipped augmentation when an UNRELATED PATH entry
// shared a prefix with a brew install location — e.g. user PATH
// containing `/usr/local/bin-old` falsely matched `/usr/local/bin`,
// so brew never made it onto $PATH and Bootstrap kept failing.
// Same sibling-substring bug class as cycle 161 (TildePath).
var envPathAugment = func(additions []string) {
	existing := os.Getenv("PATH")
	already := make(map[string]bool)
	for entry := range strings.SplitSeq(existing, string(os.PathListSeparator)) {
		already[entry] = true
	}
	for _, dir := range additions {
		if already[dir] {
			continue
		}
		existing = dir + string(os.PathListSeparator) + existing
		already[dir] = true
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
		if tagName != tagCask {
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

// Remove uninstalls a brew package, routing tap-format IDs
// (user/repo, no formula suffix) through `brew untap` instead —
// otherwise `brew uninstall user/repo` fails with "No installed keg
// or cask" and the tap is never actually removed. This is the
// correct inverse of the tap case in handleTap / isTapFormat.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	if isTapFormat(resourceID) {
		slog.Info("brew untap", "repo", resourceID)
		return p.runner.Untap(ctx, resourceID)
	}
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
	case "untap":
		return p.handleUntap(ctx, remaining, hamsFlags, flags)
	default:
		// Passthrough to real brew with stdio preserved and DryRun
		// honored, so `hams brew upgrade htop`, `hams brew --version`,
		// etc. behave identically to the unwrapped CLI.
		slog.Debug("passthrough to brew", "args", args)
		return provider.Passthrough(ctx, "brew", args, flags)
	}
}

// handleUntap executes `brew untap <repo>` via the runner and removes
// the matching hamsfile entry + marks the state resource StateRemoved.
// Without this verb, `hams brew untap user/repo` fell through to the
// raw passthrough which exec'd `brew untap` but never updated the
// hamsfile/state — drift accumulated. The cycle 52 fix routed taps
// through `brew untap` for the declarative apply path; this closes the
// loop on the CLI-first auto-record contract for taps.
func (p *Provider) handleUntap(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"brew untap requires a repository name",
			"Usage: hams brew untap <user/repo>",
		)
	}
	if len(args) != 1 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("brew untap takes exactly one repository (got %d args: %v)", len(args), args),
			"Usage: hams brew untap <user/repo>",
			"To untap multiple repos, run the command once per repo",
		)
	}

	repo := args[0]
	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: brew untap %s\n", repo)
		return nil
	}

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(baseprovider.EffectiveConfig(p.cfg, flags), flags, "brew untap")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	if err := p.runner.Untap(ctx, repo); err != nil {
		return err
	}

	hf, err := baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
	if err != nil {
		return err
	}
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}

	hf.RemoveApp(repo)
	sf.SetResource(repo, state.StateRemoved)

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return brewDisplayName }

func (p *Provider) handleList(hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	cfg := baseprovider.EffectiveConfig(p.cfg, flags)

	// Cycle 220/222: validate the resolved profile dir exists via the
	// shared helper. `hams brew list --profile=Typo` on an invalid
	// profile previously printed "Homebrew managed packages:\nNo
	// entries tracked..." — silent success despite the user's typo.
	// Matches the top-level `hams list --profile=Typo` (cycle 217),
	// HandleListCmd (cycle 220), and every other custom-list path
	// (cycle 222).
	if _, err := provider.ValidateProfileDirExists(cfg); err != nil {
		return err
	}

	hf, err := baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
	if err != nil {
		return err
	}

	statePath := filepath.Join(cfg.StateDir(), "brew.state.yaml")
	sf, err := state.Load(statePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			// Corrupt / unreadable state. Don't silently show "all
			// desired as additions" because that misrepresents drift.
			return fmt.Errorf("loading brew state %s: %w", statePath, err)
		}
		// No state yet — show all desired as additions.
		sf = state.New("brew", cfg.MachineID)
	}

	diff := provider.DiffDesiredVsState(hf, sf)
	if flags.JSON {
		// Cycle 186: emit PURE JSON with no prose header. The pre-
		// cycle-186 code printed "Homebrew managed packages:" on
		// stdout BEFORE the JSON object, making the output
		// unparseable via `hams brew list --json | jq`. Consumers
		// had to pipe through a heuristic stripper. Now the text
		// branch still prints the friendly header, but --json is
		// strictly machine-readable.
		out, jsonErr := provider.FormatDiffJSON(&diff)
		if jsonErr != nil {
			return jsonErr
		}
		fmt.Println(out)
		return nil
	}
	fmt.Println("Homebrew managed packages:")
	fmt.Print(provider.FormatDiff(&diff))
	return nil
}

func (p *Provider) handleTap(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"brew tap requires a repository name",
			"Usage: hams brew tap <user/repo>",
		)
	}
	// Strict arg count — same UX class as cycle 156 (config strict
	// args). The pre-cycle-163 code only used args[0] and silently
	// dropped the rest, so `hams brew tap user1/repo user2/repo` only
	// tapped user1/repo and the second tap was lost. Brew CLI itself
	// supports multi-tap, but the wrapper didn't forward beyond [0].
	// Surface the mismatch with a hint about repeating the command.
	if len(args) != 1 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("brew tap takes exactly one repository (got %d args: %v)", len(args), args),
			"Usage: hams brew tap <user/repo>",
			"To tap multiple repos, run the command once per repo",
		)
	}

	repo := args[0]
	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: brew tap %s\n", repo)
		return nil
	}

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(baseprovider.EffectiveConfig(p.cfg, flags), flags, "brew tap")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	// Cycle 212: route via the CmdRunner seam (which already exists
	// for handleUntap cycle 177) instead of WrapExecPassthrough.
	// Two benefits: (a) the state-write path becomes DI-testable, (b)
	// symmetric with every other brew CLI handler (install / remove /
	// untap). The real runner still exec's brew via exec.CommandContext
	// internally, so the end-user behavior is identical.
	if err := p.runner.Tap(ctx, repo); err != nil {
		return err
	}

	hf, err := baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
	if err != nil {
		return err
	}
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}

	hf.AddApp("tap", repo, "")
	// Cycle 212: state write matches cycle 96 (handleInstall) and
	// cycle 177's handleUntap fix. Without this, `hams list --only=brew`
	// showed nothing after a successful `hams brew tap <user/repo>`
	// because `list` reads state only.
	sf.SetResource(repo, state.StateOK)

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
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
	caskFlag := hasCaskFlag(args)
	// Cycle 175: --cask MUST land under the "cask" tag because
	// caskApps() (Plan-side) only marks entries under that tag with
	// IsCask=true. Without this guard, `hams brew install iterm2
	// --cask --hams-tag=apps` would auto-record under "apps" with
	// no cask metadata. Next `hams apply` would then run
	// `brew install iterm2` (no --cask), which fails because iterm2
	// has no formula. Surface the conflict instead of silently
	// breaking the apply replay.
	if caskFlag && tag != tagCLI && tag != tagCask {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("--cask is incompatible with --hams-tag=%q (cask entries must live under the %q tag so apply replay invokes brew --cask)", tag, tagCask),
			"Use --hams-tag=cask explicitly, or omit --hams-tag entirely (defaults to cask when --cask is set)",
		)
	}
	// If --cask is present in args and no explicit tag was set, use "cask" as the tag.
	if tag == tagCLI && caskFlag {
		tag = tagCask
	}
	// Cycle 176/179: reject tap-format args in `brew install`. A user
	// typing `hams brew install user/repo` almost always intends a
	// tap, but `brew install user/repo` actually triggers a `brew
	// tap` as a side effect THEN tries to install a formula named
	// "repo" from that tap (which usually doesn't exist) — leaving
	// the host tapped but with no hamsfile/state record of it.
	//
	// Cycle 179: scan ALL packages, not just packages[0]. Pre-cycle-179
	// the guard only fired when the FIRST arg was tap-format; a mixed
	// invocation like `hams brew install htop user/repo` slipped past
	// the guard and the user/repo arg leaked the tap as before.
	for _, pkg := range packages {
		if isTapFormat(pkg) {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				fmt.Sprintf("brew install does not support tap-format args (%q looks like user/repo)", pkg),
				"Use `hams brew tap "+pkg+"` instead — it taps the repo AND auto-records it",
				"To install a formula from a tap, first tap it then install: hams brew tap user/repo && hams brew install <formula>",
			)
		}
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

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(baseprovider.EffectiveConfig(p.cfg, flags), flags, "brew install")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	isCask := hasCaskFlag(args)
	// Drive the runner per-package so the flow is DI-testable
	// (previously `provider.WrapExecPassthrough` shelled out directly,
	// so unit tests couldn't cover handleInstall without a real brew).
	// Fail fast on first error to preserve apt-style atomic semantics:
	// partial install → no recording.
	for _, pkg := range packages {
		if err := p.runner.Install(ctx, pkg, isCask); err != nil {
			return err
		}
	}

	hf, err := baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
	if err != nil {
		return err
	}
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}

	for _, pkg := range packages {
		hf.AddApp(tag, pkg, "")
		// State write is additive — apt's U12-U15 pattern. Without
		// this, `hams list --only=brew` showed nothing after a
		// `hams brew install git` because `list` reads state files
		// only. The hamsfile record alone is not enough for the
		// list / refresh / drift paths.
		sf.SetResource(pkg, state.StateOK)
	}

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
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

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(baseprovider.EffectiveConfig(p.cfg, flags), flags, "brew remove")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	// Cycle 177: route tap-format IDs through Untap so `hams brew
	// remove user/repo` works symmetrically with the apply path
	// (Provider.Remove already does this routing). Pre-cycle-177 the
	// CLI handler always called runner.Uninstall, which fails with
	// "No installed keg or cask" for tap names — user couldn't
	// remove a tap via the CLI without going through `hams apply`
	// (forcing a full reconcile just to drop one tap).
	for _, pkg := range packages {
		if isTapFormat(pkg) {
			if err := p.runner.Untap(ctx, pkg); err != nil {
				return err
			}
			continue
		}
		if err := p.runner.Uninstall(ctx, pkg); err != nil {
			return err
		}
	}

	hf, err := baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, hamsFlags, flags)
	if err != nil {
		return err
	}
	sf, err := p.loadOrCreateStateFile(flags)
	if err != nil {
		return err
	}

	for _, pkg := range packages {
		hf.RemoveApp(pkg)
		sf.SetResource(pkg, state.StateRemoved)
	}

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// statePath returns the absolute path to brew.state.yaml for the
// active machine. Mirrors apt.statePath.
func (p *Provider) statePath(flags *provider.GlobalFlags) string {
	cfg := baseprovider.EffectiveConfig(p.cfg, flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads the brew state file or returns a fresh
// one when the file is absent. Non-ErrNotExist load failures (corrupt
// YAML, permission denied) propagate so the CLI handler surfaces a
// user-facing error instead of silently overwriting unparseable state.
// Mirrors apt.loadOrCreateStateFile.
func (p *Provider) loadOrCreateStateFile(flags *provider.GlobalFlags) (*state.File, error) {
	cfg := baseprovider.EffectiveConfig(p.cfg, flags)
	sf, err := state.Load(p.statePath(flags))
	if err == nil {
		return sf, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return state.New(p.Name(), cfg.MachineID), nil
	}
	return nil, fmt.Errorf("loading brew state %s: %w", p.statePath(flags), err)
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
