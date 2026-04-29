package homebrew

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CmdRunner is the DI seam for every outbound invocation of `brew`.
// Homebrew's surface is larger than other package providers (formulae
// + casks + taps + bootstrap install.sh), so the interface exposes the
// full set of verbs the provider needs. JSON parsing of `brew info
// --json=v2` stays inside the real runner — the fake returns parsed
// maps directly, and parseBrewInfoJSON has its own unit tests.
type CmdRunner interface {
	// ListFormulae runs `brew info --json=v2 --installed --formula`
	// and returns a map of formula name → installed version.
	ListFormulae(ctx context.Context) (map[string]string, error)

	// ListCasks runs `brew info --json=v2 --installed --cask` and
	// returns a map of cask token → installed version. Errors here
	// are non-fatal at the call site (brew returns non-zero when no
	// casks are installed); the caller logs and continues.
	ListCasks(ctx context.Context) (map[string]string, error)

	// ListTaps runs `brew tap` and returns the installed tap names.
	ListTaps(ctx context.Context) ([]string, error)

	// Install runs `brew install [--cask] <name>`. When isCask is
	// true, --cask is appended to the args so brew routes to the
	// casks cellar.
	Install(ctx context.Context, name string, isCask bool) error

	// Describe runs `brew info --json=v2 [--cask] <name>` and returns
	// the short human-readable description — Homebrew's canonical
	// `desc` field from the formula/cask manifest. Zero network
	// round-trip: brew reads the tap data that was already synced
	// before the install. Returns ("", nil) when the tool responded
	// but the desc field is empty — the caller decides whether to
	// fall back; non-nil error means we couldn't talk to brew at all
	// (which the intro-record flow swallows to stay best-effort).
	Describe(ctx context.Context, name string, isCask bool) (string, error)

	// Uninstall runs `brew uninstall <name>`.
	Uninstall(ctx context.Context, name string) error

	// Tap runs `brew tap <repo>`.
	Tap(ctx context.Context, repo string) error

	// Untap runs `brew untap <repo>`. Separate verb from Uninstall
	// because brew rejects `brew uninstall user/repo`: taps are
	// metadata, not installed packages.
	Untap(ctx context.Context, repo string) error
}

// NewRealCmdRunner returns the production CmdRunner that shells out
// to the real brew binary.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) ListFormulae(ctx context.Context) (map[string]string, error) {
	return r.listByType(ctx, "--formula")
}

func (r *realCmdRunner) ListCasks(ctx context.Context) (map[string]string, error) {
	return r.listByType(ctx, "--cask")
}

func (r *realCmdRunner) listByType(ctx context.Context, typeFlag string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, "brew", "info", "--json=v2", "--installed", typeFlag) //nolint:gosec // typeFlag is --formula or --cask, not user input
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("brew info %s: %w", typeFlag, err)
	}
	return parseBrewInfoJSON(output)
}

func (r *realCmdRunner) ListTaps(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "brew", "tap")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("brew tap: %w", err)
	}
	var taps []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			taps = append(taps, line)
		}
	}
	return taps, nil
}

func (r *realCmdRunner) Install(ctx context.Context, name string, isCask bool) error {
	args := []string{"install"}
	if isCask {
		args = append(args, "--cask")
	}
	args = append(args, name)
	cmd := exec.CommandContext(ctx, "brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew install %s: %w", name, err)
	}
	return nil
}

func (r *realCmdRunner) Describe(ctx context.Context, name string, isCask bool) (string, error) {
	args := []string{"info", "--json=v2"}
	if isCask {
		args = append(args, "--cask")
	}
	args = append(args, name)
	cmd := exec.CommandContext(ctx, "brew", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("brew info %s: %w", name, err)
	}
	return parseBrewDescJSON(output)
}

func (r *realCmdRunner) Uninstall(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "brew", "uninstall", name) //nolint:gosec // name from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew uninstall %s: %w", name, err)
	}
	return nil
}

func (r *realCmdRunner) Tap(ctx context.Context, repo string) error {
	cmd := exec.CommandContext(ctx, "brew", "tap", repo) //nolint:gosec // repo from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew tap %s: %w", repo, err)
	}
	return nil
}

func (r *realCmdRunner) Untap(ctx context.Context, repo string) error {
	cmd := exec.CommandContext(ctx, "brew", "untap", repo) //nolint:gosec // repo from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew untap %s: %w", repo, err)
	}
	return nil
}

// parseBrewInfoJSON parses the `brew info --json=v2 --installed` output
// shape. Exposed for unit testing in isolation (property-based tests
// verify the parser's robustness against arbitrary JSON).
func parseBrewInfoJSON(output []byte) (map[string]string, error) {
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

// parseBrewDescJSON extracts the `desc` field from the output of
// `brew info --json=v2 [--cask] <name>`. For a formula query, the
// response has shape `{"formulae":[{...}], "casks":[]}` — we take
// formulae[0].desc. For a cask query it's `{"formulae":[], "casks":[{...}]}`.
// Both arrays may be empty when `name` was not found in either set;
// we return ("", nil) rather than error so the intro-record flow
// can fall back silently. Exposed for unit testing.
func parseBrewDescJSON(output []byte) (string, error) {
	var data struct {
		Formulae []struct {
			Desc string `json:"desc"`
		} `json:"formulae"`
		Casks []struct {
			Desc string `json:"desc"`
		} `json:"casks"`
	}
	if err := json.Unmarshal(output, &data); err != nil {
		return "", fmt.Errorf("parsing brew JSON: %w", err)
	}
	if len(data.Formulae) > 0 && data.Formulae[0].Desc != "" {
		return data.Formulae[0].Desc, nil
	}
	if len(data.Casks) > 0 && data.Casks[0].Desc != "" {
		return data.Casks[0].Desc, nil
	}
	return "", nil
}
