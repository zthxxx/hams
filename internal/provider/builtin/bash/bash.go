// Package bash implements the escape-hatch script provider for arbitrary shell commands.
package bash

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/bitfield/script"
	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// bashResource holds parsed fields from a bash hamsfile entry.
type bashResource struct {
	Run    string
	Check  string
	Remove string
	Sudo   bool
}

// Provider implements the bash script provider.
type Provider struct {
	// removeCommands caches remove commands by resource ID, populated during Plan
	// so that Remove (which only receives a resource ID) can look them up.
	removeCommands map[string]string
}

// New creates a new bash provider.
func New() *Provider {
	return &Provider{
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
func (p *Provider) Probe(_ context.Context, sf *state.File) ([]provider.ProbeResult, error) {
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
			stdout, passed := RunCheck(context.Background(), r.CheckCmd)
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

	return actions, nil
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
func (p *Provider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	var sb strings.Builder
	for id, r := range sf.Resources {
		fmt.Fprintf(&sb, "  %-40s %s\n", id, r.State)
	}
	return sb.String(), nil
}

// RunCheck executes a check command and returns (stdout, exit code 0 = ok).
// Uses bitfield/script for shell execution.
func RunCheck(_ context.Context, checkCmd string) (string, bool) {
	if checkCmd == "" {
		return "", false
	}

	output, err := script.Exec(checkCmd).String()
	if err != nil {
		return output, false
	}
	return strings.TrimSpace(output), true
}

func runBash(_ context.Context, command string) error {
	p := script.Exec(command).WithStdout(os.Stdout).WithStderr(os.Stderr)
	_, err := p.String()
	if err != nil {
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
				continue
			}
			resourceByID[id] = res
		}
	}

	return resourceByID, nil
}
