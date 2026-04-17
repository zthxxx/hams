// Package bash implements the escape-hatch script provider for arbitrary shell commands.
package bash

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
	"github.com/zthxxx/hams/internal/urn"
)

// bashProviderName is the canonical manifest name and URN namespace
// segment. Used in the Manifest + URN validation so a future rename
// (if any) is a one-line change.
const bashProviderName = "bash"

// bootstrapExecCommand is the exec seam used by RunScript. Replaced in
// tests that want to assert the command line without forking a real
// process. Production callers use exec.CommandContext.
var bootstrapExecCommand = exec.CommandContext

// bashResource holds parsed fields from a bash hamsfile entry.
type bashResource struct {
	Run    string
	Check  string
	Remove string
	Sudo   bool
}

// Provider implements the bash script provider.
type Provider struct {
	// cfg supplies store/profile paths used by the `list` CLI
	// subcommand. Optional: bash can be instantiated without cfg
	// for pure apply-path usage (tests, register.go providerOnly
	// fallback) — CLI verbs that need cfg will surface an ExitUsageError
	// when cfg is nil/empty.
	cfg *config.Config
	// removeCommands caches remove commands by resource ID, populated during Plan
	// so that Remove (which only receives a resource ID) can look them up.
	removeCommands map[string]string
}

// New creates a new bash provider with an optional config. Pass cfg
// to unlock CLI verbs (`hams bash list`). Apply-path usage can pass
// nil cfg.
func New(cfg *config.Config) *Provider {
	return &Provider{
		cfg:            cfg,
		removeCommands: make(map[string]string),
	}
}

// Manifest returns the bash provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "bash",
		DisplayName:   "Bash",
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassCheckBased,
		FilePrefix:    "bash",
	}
}

// Bootstrap is a no-op for bash (bash is always available).
func (p *Provider) Bootstrap(_ context.Context) error {
	return nil
}

// Probe runs the check command for each resource in state to detect drift.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		pr := provider.ProbeResult{
			ID:    id,
			State: state.StateOK,
		}

		// If a check command is stored in state, re-run it to detect drift.
		if r.CheckCmd != "" {
			stdout, passed := RunCheck(ctx, r.CheckCmd)
			if !passed {
				pr.State = state.StatePending
				slog.Info("bash probe: check failed, marking pending", "resource", id, "check", r.CheckCmd)
			} else {
				pr.Stdout = stdout
			}
		}

		results = append(results, pr)
	}
	return results, nil
}

// Plan computes actions based on desired hamsfile vs state.
// Enriches each action's Resource with the bashResource from the hamsfile.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	resourceByID, err := bashParseResources(desired)
	if err != nil {
		return nil, err
	}

	actions := provider.ComputePlan(desired.ListApps(), observed, observed.ConfigHash)
	for i := range actions {
		res, ok := resourceByID[actions[i].ID]
		if !ok {
			continue
		}
		actions[i].Resource = res

		// Persist check command into state so Probe can re-run it for drift detection.
		if res.Check != "" {
			actions[i].StateOpts = append(actions[i].StateOpts, state.WithCheckCmd(maybeAddSudo(res.Check, res.Sudo)))
		}

		// Cache remove commands so Remove() can look them up by ID.
		if res.Remove != "" {
			p.removeCommands[actions[i].ID] = maybeAddSudo(res.Remove, res.Sudo)
		}
	}

	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply executes a bash command for the given action.
// If a check command is present and passes, the run command is skipped.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	res, ok := action.Resource.(bashResource)
	if !ok || res.Run == "" {
		return fmt.Errorf("bash provider: action resource must be a bashResource with a non-empty run command")
	}

	// If a check command is provided, run it first to see if the resource
	// is already in the desired state.
	if res.Check != "" {
		checkCmd := maybeAddSudo(res.Check, res.Sudo)
		_, passed := RunCheck(ctx, checkCmd)
		if passed {
			slog.Info("check passed, skipping run", "resource", action.ID, "check", checkCmd)
			return nil
		}
	}

	cmd := maybeAddSudo(res.Run, res.Sudo)
	slog.Info("running bash command", "resource", action.ID, "command", cmd)
	return runBash(ctx, cmd)
}

// RunScript satisfies provider.BashScriptRunner. It executes an arbitrary
// shell script — used by the provider framework's RunBootstrap helper to
// honor a DependOn.Script declaration under explicit user consent. Stdin,
// stdout, and stderr are passed through to the user's terminal so that
// interactive prompts from the script (sudo password, installer
// confirmations) reach the user.
func (p *Provider) RunScript(ctx context.Context, shellScript string) error {
	if shellScript == "" {
		return nil
	}
	slog.Info("bash provider: running bootstrap script", "script", shellScript)
	cmd := bootstrapExecCommand(ctx, "/bin/bash", "-c", shellScript)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bash bootstrap script failed: %w", err)
	}
	return nil
}

// Remove executes the remove command for a bash resource, if one was defined
// in the hamsfile and cached during Plan. Otherwise it is a no-op.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	cmd, ok := p.removeCommands[resourceID]
	if !ok || cmd == "" {
		slog.Warn("bash provider: no remove command defined, skipping", "resource", resourceID)
		return nil
	}

	slog.Info("running bash remove command", "resource", resourceID, "command", cmd)
	return runBash(ctx, cmd)
}

// List returns a formatted list of bash resources.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// RunCheck executes a check command and returns (stdout, exit code 0 = ok).
// Now uses exec.CommandContext (was bitfield/script before cycle 160) so
// SIGINT/SIGTERM cancels the check promptly. Without this, a check
// command that hangs (e.g. `curl https://slow.example.com`) would
// keep running after Ctrl+C had unwound the rest of the apply.
func RunCheck(ctx context.Context, checkCmd string) (string, bool) {
	if checkCmd == "" {
		return "", false
	}
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", checkCmd) //nolint:gosec // hamsfile-declared check command
	out, err := cmd.Output()
	if err != nil {
		return string(out), false
	}
	return strings.TrimSpace(string(out)), true
}

// runBash executes a bash command, streaming stdout/stderr to the
// user's terminal. Now uses exec.CommandContext (was bitfield/script
// before cycle 160) so SIGINT/SIGTERM cancels the script promptly —
// previously a hanging install/check kept running after Ctrl+C
// because script.Exec didn't honor context cancellation.
func runBash(ctx context.Context, command string) error {
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", command) //nolint:gosec // hamsfile-declared run/remove command
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bash command failed: %w\n  command: %s", err, command)
	}
	return nil
}

// maybeAddSudo prefixes the command with "sudo " when sudo is true.
func maybeAddSudo(cmd string, sudo bool) string {
	if sudo {
		return "sudo " + cmd
	}
	return cmd
}

func bashParseResources(f *hamsfile.File) (map[string]bashResource, error) {
	if f.Root == nil || len(f.Root.Content) == 0 {
		return map[string]bashResource{}, nil
	}

	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("bash provider: hamsfile root must be a mapping")
	}

	resourceByID := make(map[string]bashResource)
	for i := 1; i < len(doc.Content); i += 2 {
		seq := doc.Content[i]
		if seq.Kind != yaml.SequenceNode {
			continue
		}

		for _, item := range seq.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}

			var id string
			var res bashResource
			for j := 0; j < len(item.Content)-1; j += 2 {
				key := item.Content[j].Value
				value := item.Content[j+1].Value
				switch key {
				case "urn":
					id = value
				case "run":
					res.Run = strings.TrimSpace(value)
				case "check":
					res.Check = strings.TrimSpace(value)
				case "remove":
					res.Remove = strings.TrimSpace(value)
				case "sudo":
					res.Sudo = strings.EqualFold(value, "true")
				}
			}

			if id == "" {
				// Cycle 180: emit a slog.Warn when an entry has fields
				// like `run:` but no `urn:` — this is a common user typo
				// (forgot to add the URN line). Pre-cycle-180 the entry
				// was silently dropped: ListApps skipped it (no app/urn
				// field), bashParseResources skipped it here, so the
				// user's script never ran and they had no clue why.
				if res.Run != "" || res.Check != "" || res.Remove != "" {
					slog.Warn("bash provider: entry has run/check/remove but no urn — silently ignored",
						"run", res.Run, "check", res.Check)
				}
				continue
			}
			// Cycle 229: warn on URN shape mismatch per schema-design
			// spec §"Malformed URN is rejected" / "URN with colon in
			// resource ID is rejected". The spec SHALL-rejects these
			// at the hamsfile SDK level; we soft-warn (continue
			// processing) to stay backwards-compatible with hamsfiles
			// that pre-date this guard. Users see a clear slog.Warn in
			// the session log (cycle 65/67 dual-sink) instead of
			// silently accumulating malformed entries.
			if u, parseErr := urn.Parse(id); parseErr != nil || u.Provider != bashProviderName {
				var reason string
				if parseErr != nil {
					reason = parseErr.Error()
				} else {
					reason = fmt.Sprintf("urn provider is %q, expected %q", u.Provider, bashProviderName)
				}
				slog.Warn("bash provider: urn does not match 'urn:hams:bash:<id>' shape — processing anyway, but consider fixing",
					"urn", id, "reason", reason)
			}
			// Cycle 193: warn on duplicate URNs. Silent last-write-
			// wins means ComputePlan's first-occurrence-wins dedup
			// (cycle 111) and bashParseResources's last-wins storage
			// disagree — Apply would run the LAST entry's `run`
			// command even though `hams list` / `hams apply` dry-run
			// preview iterate via ListApps (first). The user's first
			// entry silently loses. Surface the collision so they can
			// deduplicate.
			if prior, exists := resourceByID[id]; exists && (prior.Run != res.Run || prior.Check != res.Check || prior.Remove != res.Remove) {
				slog.Warn("bash provider: duplicate urn in hamsfile — only the last entry wins, preceding entries are silently lost",
					"urn", id, "kept", res.Run, "discarded", prior.Run)
			}
			resourceByID[id] = res
		}
	}

	return resourceByID, nil
}

// Name returns the CLI name.
func (p *Provider) Name() string { return "bash" }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return "Bash" }

// HandleCommand processes CLI subcommands for bash.
//
// Per builtin-providers.md §"Bash Provider" CLI wrapping:
//
//   - `hams bash list` — show all steps with status from state.
//   - `hams bash run <urn-id>` — execute a single step by URN suffix.
//   - `hams bash remove <urn-id>` — remove the step from the hamsfile.
//
// Cycle 215 wires `list` via the shared HandleListCmd helper (same
// pattern as cargo / npm / pnpm / uv / mas / vscodeext / goinstall /
// apt / duti / defaults from cycle 214). The `run` and `remove` verbs
// require URN-resolution + hamsfile-edit support that v1 has not yet
// shipped — they return an ExitUsageError pointing the user at
// `hams apply --only=bash` or hand-editing the bash hamsfile.
//
// Any non-verb input produces a usage error: `hams bash <playbook.yml>`
// makes no sense for bash (unlike ansible there is no sensible
// passthrough since bash scripts must be URN-tracked to run via hams).
func (p *Provider) HandleCommand(ctx context.Context, args []string, _ map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return p.bashUsageError()
	}

	switch args[0] {
	case "list":
		cfg := p.effectiveConfig(flags)
		return provider.HandleListCmd(ctx, p, cfg)
	case "run", "remove":
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("bash %s is planned for v1.1 (URN-editing on the CLI is not yet wired)", args[0]),
			"Use 'hams apply --only=bash' to run all tracked bash scripts",
			"Or hand-edit the bash hamsfile: <profile-dir>/bash.hams.yaml",
		)
	}
	return p.bashUsageError()
}

func (p *Provider) bashUsageError() error {
	return hamserr.NewUserError(hamserr.ExitUsageError,
		"bash requires a subcommand",
		"Usage: hams bash list",
		"       hams bash run <urn-id>              (planned v1.1)",
		"       hams bash remove <urn-id>           (planned v1.1)",
	)
}

// effectiveConfig returns the provider's config overlaid with any
// per-invocation flags (--store, --profile). Mirrors the helper used
// by other providers' CLI paths.
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
