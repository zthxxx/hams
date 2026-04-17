// Package defaults wraps the macOS defaults command for managing application preferences.
package defaults

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// cliName is the macOS `defaults` CLI binary and provider name.
const cliName = "defaults"

// Provider implements the macOS defaults provider.
type Provider struct {
	cfg    *config.Config
	runner CmdRunner
}

// New creates a new defaults provider wired with a real CmdRunner.
// Pass NewFakeCmdRunner from tests for DI-isolated unit testing.
func New(cfg *config.Config, runner CmdRunner) *Provider {
	return &Provider{cfg: cfg, runner: runner}
}

// Manifest returns the defaults provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          cliName,
		DisplayName:   cliName,
		Platforms:     []provider.Platform{provider.PlatformDarwin},
		ResourceClass: provider.ClassKVConfig,
		FilePrefix:    cliName,
	}
}

// Bootstrap checks if defaults is available (always on macOS).
func (p *Provider) Bootstrap(_ context.Context) error {
	return p.runner.LookPath()
}

// Probe reads current values for tracked defaults entries.
func (p *Provider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		domain, key := parseDomainKey(id)
		if domain == "" || key == "" {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed, ErrorMsg: "invalid format"})
			continue
		}

		value, err := p.runner.Read(ctx, domain, key)
		if err != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
			continue
		}

		results = append(results, provider.ProbeResult{
			ID:    id,
			State: state.StateOK,
			Value: value,
		})
	}
	return results, nil
}

// Plan computes actions for defaults entries and attaches any
// hamsfile-declared hooks to each action.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	return provider.PopulateActionHooks(actions, desired), nil
}

// Apply writes a defaults value.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	// Resource ID format: "domain.key=type:value" e.g., "com.apple.dock.autohide=bool:true"
	domain, key, typeStr, value, err := parseDefaultsResource(action.ID)
	if err != nil {
		return err
	}

	slog.Info("defaults write", "domain", domain, "key", key, "type", typeStr, "value", value)
	return p.runner.Write(ctx, domain, key, typeStr, value)
}

// Remove deletes a defaults key.
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	domain, key := parseDomainKey(resourceID)
	slog.Info("defaults delete", "domain", domain, "key", key)
	return p.runner.Delete(ctx, domain, key)
}

// List returns defaults entries with status.
func (p *Provider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for defaults. The two
// recognized shapes — `defaults write <domain> <key> -<type> <value>`
// and `defaults delete <domain> <key>` — are auto-recorded into the
// hamsfile and state so subsequent `hams apply` runs reproduce the
// mutation. Other `defaults` verbs (e.g., `read`, `domains`) are
// passed through to the real binary without bookkeeping.
func (p *Provider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	// Cycle 214: list must be recognized before the len<3 gate below
	// because `hams defaults list` intentionally takes no further args.
	// The spec promises diff view for list; pre-cycle-214 the len<3
	// gate rejected it with a misleading "write <domain> <key>" usage
	// error.
	if len(args) == 1 && args[0] == "list" { //nolint:gosec // len(args)==1 guards the index
		return provider.HandleListCmd(ctx, p, p.effectiveConfig(flags))
	}
	if len(args) < 3 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"defaults requires: write <domain> <key> -<type> <value>",
			"Usage: hams defaults write com.apple.dock autohide -bool true",
			"       hams defaults list",
		)
	}

	verb := args[0]
	switch verb {
	case "write":
		return p.handleWrite(ctx, args, hamsFlags, flags)
	case "delete":
		return p.handleDelete(ctx, args, hamsFlags, flags)
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: defaults %s\n", strings.Join(args, " "))
		return nil
	}
	cmd := exec.CommandContext(ctx, cliName, args...) //nolint:gosec // defaults args from CLI input
	return cmd.Run()
}

// handleWrite executes `defaults write` via the CmdRunner and records
// the resulting (domain, key, type, value) tuple into the hamsfile
// and state. A re-write with the same domain+key but a different
// value replaces the old hamsfile entry in place (old → StateRemoved,
// new → StateOK) so the hamsfile stays single-valued per key.
func (p *Provider) handleWrite(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	// Strict arg count — same UX class as cycle 156/163. A user typing
	// `hams defaults write com.apple.dock SetText -string Hello World`
	// (forgot to quote a multi-word value) had "World" silently dropped:
	// only "Hello" was passed to defaults AND recorded. Far worse than
	// a typo because the user believed the full string was set. Now:
	// surface the mismatch with a quoting hint.
	if len(args) != 5 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("defaults write requires exactly 4 args after `write` (got %d): write <domain> <key> -<type> <value>", len(args)-1),
			"Usage: hams defaults write com.apple.dock autohide -bool true",
			"Quote multi-word values: hams defaults write <domain> <key> -string \"<value with spaces>\"",
		)
	}

	domain := args[1]
	key := args[2]
	typeStr := strings.TrimPrefix(args[3], "-")
	value := args[4]

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: defaults %s\n", strings.Join(args, " "))
		return nil
	}

	if err := p.runner.Write(ctx, domain, key, typeStr, value); err != nil {
		return err
	}

	resourceID := fmt.Sprintf("%s.%s=%s:%s", domain, key, typeStr, value)
	previewCmd := "defaults " + strings.Join(args, " ")
	return p.recordWrite(resourceID, previewCmd, domain, key, value, hamsFlags, flags)
}

// handleDelete executes `defaults delete` via the CmdRunner and
// removes the matching hamsfile entry (by `<domain>.<key>` prefix),
// marking the state resource as StateRemoved.
func (p *Provider) handleDelete(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	// Strict arg count — same UX class as handleWrite. A user typing
	// `hams defaults delete com.apple.dock autohide other-key` would
	// previously have silently deleted only the first (domain, key)
	// pair and dropped "other-key". Surface the mismatch.
	if len(args) != 3 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("defaults delete requires exactly 2 args after `delete` (got %d): delete <domain> <key>", len(args)-1),
			"Usage: hams defaults delete com.apple.dock autohide",
			"To delete multiple keys, run the command once per key",
		)
	}

	domain := args[1]
	key := args[2]

	if flags.DryRun {
		fmt.Printf("[dry-run] Would run: defaults %s\n", strings.Join(args, " "))
		return nil
	}

	if err := p.runner.Delete(ctx, domain, key); err != nil {
		return err
	}

	return p.recordDelete(domain, key, hamsFlags, flags)
}

// recordWrite persists an auto-record entry from a `defaults write`
// invocation. Same-key-different-value invocations remove the stale
// entry in place so the hamsfile never accumulates out-of-date values
// for a single (domain, key) pair.
func (p *Provider) recordWrite(resourceID, previewCmd, domain, key, value string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	sf := p.loadOrCreateStateFile(flags)

	prefix := domain + "." + key + "="
	for _, existing := range hf.ListApps() {
		if existing == resourceID {
			continue
		}
		if strings.HasPrefix(existing, prefix) {
			hf.RemoveApp(existing)
			sf.SetResource(existing, state.StateRemoved)
		}
	}

	hf.AddApp(tagCLI, resourceID, "")
	hf.SetPreviewCmd(resourceID, previewCmd)
	sf.SetResource(resourceID, state.StateOK, state.WithValue(value))

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// recordDelete removes any hamsfile entry for (domain, key) and marks
// the state resource as removed. A delete that matches no hamsfile
// entry still updates state for auditability.
func (p *Provider) recordDelete(domain, key string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	sf := p.loadOrCreateStateFile(flags)

	prefix := domain + "." + key + "="
	removed := false
	for _, existing := range hf.ListApps() {
		if strings.HasPrefix(existing, prefix) {
			hf.RemoveApp(existing)
			sf.SetResource(existing, state.StateRemoved)
			removed = true
		}
	}

	if !removed {
		// No matching hamsfile entry. Still record the delete in
		// state for audit purposes so a future `hams apply` sees the
		// tombstone and doesn't attempt to re-assert the old value.
		tombstoneID := prefix
		sf.SetResource(tombstoneID, state.StateRemoved)
	}

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// effectiveConfig returns the config with flag overrides applied.
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

// Name returns the CLI name.
func (p *Provider) Name() string { return cliName }

// DisplayName returns the display name.
func (p *Provider) DisplayName() string { return cliName }

// parseDomainKey splits "domain.key" from a resource ID like "domain.key=type:value".
func parseDomainKey(resourceID string) (domain, key string) {
	id := strings.SplitN(resourceID, "=", 2)[0]
	lastDot := strings.LastIndex(id, ".")
	if lastDot < 0 {
		return "", ""
	}
	return id[:lastDot], id[lastDot+1:]
}

// parseDefaultsResource parses "domain.key=type:value".
func parseDefaultsResource(id string) (domain, key, typeStr, value string, err error) {
	parts := strings.SplitN(id, "=", 2)
	if len(parts) != 2 {
		return "", "", "", "", fmt.Errorf("defaults: resource ID must be 'domain.key=type:value', got %q", id)
	}

	domain, key = parseDomainKey(id)
	if domain == "" || key == "" {
		return "", "", "", "", fmt.Errorf("defaults: invalid domain.key in %q", id)
	}

	tv := strings.SplitN(parts[1], ":", 2)
	if len(tv) != 2 {
		return "", "", "", "", fmt.Errorf("defaults: value must be 'type:value', got %q", parts[1])
	}

	return domain, key, tv[0], tv[1], nil
}
