// Package provider defines the provider interface, registry, DAG resolver, and execution engine.
package provider

import (
	"context"
	"fmt"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// ResourceClass identifies how a provider's resources are probed.
type ResourceClass int

const (
	// ClassPackage uses native list commands (brew list, pnpm list -g, etc.).
	ClassPackage ResourceClass = iota
	// ClassKVConfig uses read-back commands (defaults read, git config --get, etc.).
	ClassKVConfig
	// ClassCheckBased uses user-supplied check commands with exit code + stdout.
	ClassCheckBased
	// ClassFilesystem checks file/directory existence.
	ClassFilesystem
)

// String returns the human-readable name of the resource class.
func (c ResourceClass) String() string {
	switch c {
	case ClassPackage:
		return "package"
	case ClassKVConfig:
		return "kv-config"
	case ClassCheckBased:
		return "check-based"
	case ClassFilesystem:
		return "filesystem"
	default:
		return fmt.Sprintf("ResourceClass(%d)", int(c))
	}
}

// Platform represents a supported operating system.
type Platform string

const (
	// PlatformDarwin matches macOS systems.
	PlatformDarwin Platform = "darwin"
	// PlatformLinux matches Linux systems.
	PlatformLinux Platform = "linux"
	// PlatformAll matches any platform.
	PlatformAll Platform = "all"
)

// DependOn declares a provider dependency for self-bootstrapping.
type DependOn struct {
	Provider string   `yaml:"provider"`
	Package  string   `yaml:"package,omitempty"`
	Script   string   `yaml:"script,omitempty"`
	Platform Platform `yaml:"if,omitempty"` // Empty means all platforms.
}

// VerbRoute maps a user-facing verb to the provider action.
type VerbRoute struct {
	Verb   string `yaml:"verb"`   // e.g., "install", "remove", "list"
	Action string `yaml:"action"` // Provider-specific action to invoke.
}

// FlagDef defines a provider-specific --hams- flag.
type FlagDef struct {
	Name        string `yaml:"name"`        // Flag name without --hams- prefix.
	Description string `yaml:"description"` // Human-readable description.
	Default     string `yaml:"default"`     // Default value (empty for boolean).
}

// Manifest holds provider metadata used for registration and discovery.
type Manifest struct {
	Name          string            `yaml:"name"`
	DisplayName   string            `yaml:"display_name"`
	Platforms     []Platform        `yaml:"platforms"`
	ResourceClass ResourceClass     `yaml:"resource_class"`
	DependsOn     []DependOn        `yaml:"depends_on,omitempty"`
	FilePrefix    string            `yaml:"file_prefix"`            // e.g., "Homebrew" → Homebrew.hams.yaml
	VerbRouting   []VerbRoute       `yaml:"verb_routing,omitempty"` // Maps verbs to provider actions.
	AutoInject    map[string]string `yaml:"auto_inject,omitempty"`  // Flags auto-injected per verb.
	HamsFlags     []FlagDef         `yaml:"hams_flags,omitempty"`   // Provider-specific --hams- flags.
}

// ProbeResult represents the outcome of probing a single resource.
type ProbeResult struct {
	ID       string
	State    state.ResourceState
	Version  string
	Value    string
	Stdout   string
	ErrorMsg string
}

// Action represents a planned operation on a resource.
type Action struct {
	ID        string
	Type      ActionType
	Resource  any                    // Provider-specific resource data.
	StateOpts []state.ResourceOption // Extra state options applied after successful execution.
	Hooks     *HookSet               // Optional hooks to run around this action.
}

// ActionType categorizes what will happen to a resource during apply.
type ActionType int

const (
	// ActionInstall creates a new resource.
	ActionInstall ActionType = iota
	// ActionUpdate modifies an existing resource.
	ActionUpdate
	// ActionRemove deletes a resource.
	ActionRemove
	// ActionSkip leaves the resource unchanged.
	ActionSkip
)

// String returns the human-readable name of the action type.
func (a ActionType) String() string {
	switch a {
	case ActionInstall:
		return "install"
	case ActionUpdate:
		return "update"
	case ActionRemove:
		return "remove"
	case ActionSkip:
		return "skip"
	default:
		return fmt.Sprintf("ActionType(%d)", int(a))
	}
}

// Provider is the interface that all providers (builtin and external) must implement.
type Provider interface {
	// Manifest returns the provider's metadata.
	Manifest() Manifest

	// Bootstrap ensures the provider's runtime is available.
	// Called before any other operations if the provider has DependsOn entries.
	Bootstrap(ctx context.Context) error

	// Probe queries the environment for the current state of known resources.
	// Returns one ProbeResult per resource found in the state file.
	Probe(ctx context.Context, stateFile *state.File) ([]ProbeResult, error)

	// Plan computes the diff between desired (hamsfile) and observed (state).
	Plan(ctx context.Context, desired *hamsfile.File, observed *state.File) ([]Action, error)

	// Apply executes a single action (install, update, or remove).
	Apply(ctx context.Context, action Action) error

	// Remove uninstalls a resource and marks it as removed.
	Remove(ctx context.Context, resourceID string) error

	// List returns a human-readable list of managed resources with their status.
	List(ctx context.Context, desired *hamsfile.File, observed *state.File) (string, error)
}

// Enricher is an optional interface for providers that support LLM-driven enrichment.
type Enricher interface {
	// Enrich uses an LLM to generate or update tags and intro for a resource.
	Enrich(ctx context.Context, resourceID string) error
}
