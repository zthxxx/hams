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

// Provider implements the bash script provider.
type Provider struct{}

// New creates a new bash provider.
func New() *Provider {
	return &Provider{}
}

// Manifest returns the bash provider metadata.
func (p *Provider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "bash",
		DisplayName:   "Bash",
		Platform:      provider.PlatformAll,
		ResourceClass: provider.ClassCheckBased,
		FilePrefix:    "bash",
	}
}

// Bootstrap is a no-op for bash (bash is always available).
func (p *Provider) Bootstrap(_ context.Context) error {
	return nil
}

// Probe runs the check command for each resource in state.
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

		// If we have check_stdout from a previous run, use it for comparison.
		if r.CheckStdout != "" {
			pr.Stdout = r.CheckStdout
		}

		results = append(results, pr)
	}
	return results, nil
}

// Plan computes actions based on desired hamsfile vs state.
// Enriches each action's Resource with the run command from the hamsfile.
func (p *Provider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	runByID, err := bashRunCommands(desired)
	if err != nil {
		return nil, err
	}

	actions := provider.ComputePlan(desired.ListApps(), observed, observed.ConfigHash)
	for i := range actions {
		run, ok := runByID[actions[i].ID]
		if !ok || run == "" {
			continue
		}
		actions[i].Resource = run
	}

	return actions, nil
}

// Apply executes a bash command for the given action.
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	cmd, ok := action.Resource.(string)
	if !ok || cmd == "" {
		return fmt.Errorf("bash provider: action resource must be a non-empty command string")
	}

	slog.Info("running bash command", "resource", action.ID, "command", cmd)
	return runBash(ctx, cmd)
}

// Remove is a no-op for bash — scripts don't have a natural "uninstall".
func (p *Provider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("bash provider: remove is a no-op for scripts", "resource", resourceID)
	return nil
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

func bashRunCommands(f *hamsfile.File) (map[string]string, error) {
	if f.Root == nil || len(f.Root.Content) == 0 {
		return map[string]string{}, nil
	}

	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("bash provider: hamsfile root must be a mapping")
	}

	runByID := make(map[string]string)
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
			var run string
			for j := 0; j < len(item.Content)-1; j += 2 {
				key := item.Content[j].Value
				value := item.Content[j+1].Value
				switch key {
				case "urn":
					id = value
				case "run":
					run = strings.TrimSpace(value)
				}
			}

			if id == "" {
				continue
			}
			runByID[id] = run
		}
	}

	return runByID, nil
}
