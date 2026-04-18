// Package git implements providers for git config management and repository cloning.
package git

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

// configProviderName is the manifest name and CLI verb for this provider.
const configProviderName = "git-config"

// configDisplayName is the human-readable display name for the git config provider.
const configDisplayName = "git config"

// ConfigProvider implements the git config KV provider.
type ConfigProvider struct {
	cfg    *config.Config
	runner CmdRunner
}

// NewConfigProvider creates a new git config provider wired with the
// given config and a real CmdRunner.
func NewConfigProvider(cfg *config.Config) *ConfigProvider {
	return &ConfigProvider{cfg: cfg, runner: NewRealCmdRunner()}
}

// WithRunner replaces the CmdRunner on the provider. Exposed so tests
// can inject a fake runner that records calls and manipulates an
// in-memory KV store, without exec-ing the host's git binary.
func (p *ConfigProvider) WithRunner(r CmdRunner) *ConfigProvider {
	p.runner = r
	return p
}

// Manifest returns the git config provider metadata.
func (p *ConfigProvider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          configProviderName,
		DisplayName:   configDisplayName,
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassKVConfig,
		FilePrefix:    configProviderName,
	}
}

// Bootstrap checks if git is available.
func (p *ConfigProvider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH")
	}
	return nil
}

// Probe reads current git config values for tracked resources.
func (p *ConfigProvider) Probe(ctx context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		key := splitResourceKey(id)
		value, err := p.runner.GetGlobal(ctx, key)
		if err != nil {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed, ErrorMsg: err.Error()})
			continue
		}

		results = append(results, provider.ProbeResult{ID: id, State: state.StateOK, Value: value})
	}
	return results, nil
}

// Plan computes actions for git config entries.
func (p *ConfigProvider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	apps := desired.ListApps()
	return provider.ComputePlan(apps, observed, observed.ConfigHash), nil
}

// Apply sets a git config value.
func (p *ConfigProvider) Apply(ctx context.Context, action provider.Action) error {
	// Resource format: "key=value" e.g., "user.name=zthxxx".
	parts := strings.SplitN(action.ID, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("git config: resource ID must be 'scope.key=value', got %q", action.ID)
	}

	key := parts[0]
	value := parts[1]

	slog.Info("git config", "key", key, "value", value)
	return p.runner.SetGlobal(ctx, key, value)
}

// Remove unsets a git config value.
func (p *ConfigProvider) Remove(ctx context.Context, resourceID string) error {
	key := splitResourceKey(resourceID)
	slog.Info("git config --unset", "key", key)
	return p.runner.UnsetGlobal(ctx, key)
}

// List returns git config entries with diff between desired and observed.
func (p *ConfigProvider) List(_ context.Context, desired *hamsfile.File, sf *state.File) (string, error) {
	diff := provider.DiffDesiredVsState(desired, sf)
	return provider.FormatDiff(&diff), nil
}

// HandleCommand processes CLI subcommands for git config. Supported
// shapes (per builtin-providers.md §git-config):
//
//   - `hams git-config set <key> <value>` (canonical) — sets and
//     auto-records.
//   - `hams git-config <key> <value>` (bare, backward compat) —
//     same semantics as `set`.
//   - `hams git-config remove <key>` — unsets globally and deletes
//     the matching hamsfile entry.
//   - `hams git-config list` — prints the desired-vs-observed diff.
func (p *ConfigProvider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return p.usageError()
	}

	switch args[0] {
	case "set":
		if len(args) != 3 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"git-config set requires key and value",
				"Usage: hams git-config set <key> <value>",
				"Example: hams git-config set user.name zthxxx",
			)
		}
		return p.doSet(ctx, args[1], args[2], hamsFlags, flags)
	case verbRemove:
		if len(args) != 2 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"git-config remove requires a key",
				"Usage: hams git-config remove <key>",
				"Example: hams git-config remove user.name",
			)
		}
		return p.doRemove(ctx, args[1], hamsFlags, flags)
	case verbList:
		return p.doList(ctx, flags)
	}

	// Bare form: `hams git-config <key> <value>`.
	if len(args) != 2 {
		return p.usageError()
	}
	return p.doSet(ctx, args[0], args[1], hamsFlags, flags)
}

func (p *ConfigProvider) usageError() error {
	return hamserr.NewUserError(hamserr.ExitUsageError,
		"git-config requires a subcommand or key/value pair",
		"Usage: hams git-config set <key> <value>",
		"       hams git-config <key> <value>",
		"       hams git-config remove <key>",
		"       hams git-config list",
	)
}

// doSet runs `git config --global <key> <value>` via the runner and
// persists the entry into the hamsfile + state (via recordSet).
func (p *ConfigProvider) doSet(ctx context.Context, key, value string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if flags.DryRun {
		fmt.Printf("[dry-run] Would set: git config --global %s %s\n", key, value)
		return nil
	}

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(p.effectiveConfig(flags), flags, "git-config set")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	if err := p.runner.SetGlobal(ctx, key, value); err != nil {
		return err
	}
	return p.recordSet(key, value, hamsFlags, flags)
}

// doRemove runs `git config --global --unset <key>` via the runner
// and deletes any hamsfile entry starting with `<key>=` (marking the
// matching state resource StateRemoved).
func (p *ConfigProvider) doRemove(ctx context.Context, key string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if flags.DryRun {
		fmt.Printf("[dry-run] Would unset: git config --global --unset %s\n", key)
		return nil
	}

	// Cycle 222: acquire single-writer state lock per cli-architecture spec.
	release, lockErr := provider.AcquireMutationLockFromCfg(p.effectiveConfig(flags), flags, "git-config remove")
	if lockErr != nil {
		return lockErr
	}
	defer release()

	if err := p.runner.UnsetGlobal(ctx, key); err != nil {
		return err
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	sf := p.loadOrCreateStateFile(flags)

	prefix := key + "="
	removed := false
	for _, existing := range hf.ListApps() {
		if strings.HasPrefix(existing, prefix) {
			hf.RemoveApp(existing)
			sf.SetResource(existing, state.StateRemoved)
			removed = true
		}
	}
	if !removed {
		// No prior hamsfile entry. Record a tombstone so `hams apply`
		// doesn't try to re-assert a stale value from state.
		sf.SetResource(prefix, state.StateRemoved)
	}

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// doList prints the desired-vs-observed diff for the git-config
// provider (same output as `hams list --only=git-config`).
//
// Cycle 222: fail fast on typo'd --profile via the shared helper.
func (p *ConfigProvider) doList(ctx context.Context, flags *provider.GlobalFlags) error {
	if _, err := provider.ValidateProfileDirExists(p.effectiveConfig(flags)); err != nil {
		return err
	}

	hf, err := p.loadOrCreateHamsfile(nil, flags)
	if err != nil {
		return err
	}
	sf := p.loadOrCreateStateFile(flags)

	output, err := p.List(ctx, hf, sf)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

// recordSet persists the key=value pair into the hamsfile and state
// file. If an entry with the same key but a different value already
// exists in the hamsfile, it is replaced in-place (via remove+add)
// so the hamsfile stays aligned with what `git config` actually has
// on disk. Returns the first write error encountered.
func (p *ConfigProvider) recordSet(key, value string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}

	sf := p.loadOrCreateStateFile(flags)

	newEntry := key + "=" + value
	// Remove any pre-existing entry for this key with a different
	// value so the hamsfile is single-valued per key.
	for _, existing := range hf.ListApps() {
		if existing == newEntry {
			continue
		}
		if splitResourceKey(existing) == key {
			hf.RemoveApp(existing)
			sf.SetResource(existing, state.StateRemoved)
		}
	}

	hf.AddApp(tagCLI, newEntry, "")
	sf.SetResource(newEntry, state.StateOK, state.WithValue(value))

	if writeErr := hf.Write(); writeErr != nil {
		return writeErr
	}
	return sf.Save(p.statePath(flags))
}

// Name returns the CLI name.
func (p *ConfigProvider) Name() string { return configProviderName }

// DisplayName returns the display name.
func (p *ConfigProvider) DisplayName() string { return configDisplayName }

// splitResourceKey returns the part of a resource ID before the first
// `=`. The git-config resource ID convention is `<key>=<value>` (with
// `global` scope implied); Probe/Remove need the key alone to invoke
// `git config --get` / `--unset`, and the auto-record path needs it to
// detect "same key, different value" drift.
func splitResourceKey(resourceID string) string {
	return strings.SplitN(resourceID, "=", 2)[0]
}
