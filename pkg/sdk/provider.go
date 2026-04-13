// Package sdk provides the public API for external provider authors using the go-plugin framework.
package sdk

import "context"

// ProviderPlugin is the interface that external (go-plugin) providers must implement.
// This mirrors the internal Provider interface but uses only stdlib + SDK types
// to avoid importing internal packages across the plugin boundary.
//
// In v1, only builtin providers (compiled into the hams binary) are supported.
// External plugin support via hashicorp/go-plugin is designed here for future use.
// The gRPC service definition will use protobuf (.proto files in pkg/sdk/proto/)
// so that TypeScript/Bun SDK clients can be generated from the same schema.
type ProviderPlugin interface {
	// Manifest returns the provider's metadata as a PluginManifest.
	Manifest() PluginManifest

	// Probe queries the environment for resource states.
	Probe(ctx context.Context, resourceIDs []string) ([]PluginProbeResult, error)

	// Apply installs or updates a resource.
	Apply(ctx context.Context, resourceID string, action string) error

	// Remove uninstalls a resource.
	Remove(ctx context.Context, resourceID string) error
}

// PluginManifest holds metadata for a plugin provider.
type PluginManifest struct {
	Name          string         `json:"name" yaml:"name"`
	DisplayName   string         `json:"display_name" yaml:"display_name"`
	Platform      string         `json:"platform" yaml:"platform"`
	ResourceClass string         `json:"resource_class" yaml:"resource_class"`
	DependsOn     []PluginDepend `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	FilePrefix    string         `json:"file_prefix" yaml:"file_prefix"`
}

// PluginDepend declares a plugin dependency.
type PluginDepend struct {
	Provider string `json:"provider" yaml:"provider"`
	Package  string `json:"package,omitempty" yaml:"package,omitempty"`
	Script   string `json:"script,omitempty" yaml:"script,omitempty"`
	Platform string `json:"if,omitempty" yaml:"if,omitempty"`
}

// PluginProbeResult holds the probe result for a single resource.
type PluginProbeResult struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Version  string `json:"version,omitempty"`
	Value    string `json:"value,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	ErrorMsg string `json:"error,omitempty"`
}

// PluginDiscoveryPaths returns the default paths where hams looks for plugins.
func PluginDiscoveryPaths() []string {
	return []string{
		"~/.config/hams/plugins",
		"/usr/local/lib/hams/plugins",
	}
}
