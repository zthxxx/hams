// Package state manages Terraform-style local state files for tracking resource status.
package state

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
)

// SchemaVersion is the current state file format version.
const SchemaVersion = 2

// timestampFormat is the compact ISO format used across hams YAML files.
const timestampFormat = "20060102T150405"

// ResourceState represents the status of a managed resource.
type ResourceState string

const (
	// StateOK indicates the resource is successfully installed/configured.
	StateOK ResourceState = "ok"
	// StateFailed indicates the last operation on this resource failed.
	StateFailed ResourceState = "failed"
	// StatePending indicates the resource is queued for installation.
	StatePending ResourceState = "pending"
	// StateRemoved indicates the resource was intentionally uninstalled.
	StateRemoved ResourceState = "removed"
	// StateHookFailed indicates the resource is installed but a post-hook failed.
	StateHookFailed ResourceState = "hook-failed"
)

// File represents a provider's state file.
type File struct {
	SchemaVersion int                  `yaml:"schema_version"`
	Provider      string               `yaml:"provider"`
	MachineID     string               `yaml:"machine_id"`
	LastApply     string               `yaml:"last_apply_session,omitempty"`
	ConfigHash    string               `yaml:"last_apply_config_hash,omitempty"`
	Resources     map[string]*Resource `yaml:"resources"`
}

// Resource represents the state of a single managed resource.
type Resource struct {
	State          ResourceState `yaml:"state"`
	Version        string        `yaml:"version,omitempty"`
	Value          string        `yaml:"value,omitempty"`
	CheckCmd       string        `yaml:"check_cmd,omitempty"`
	CheckStdout    string        `yaml:"check_stdout,omitempty"`
	FirstInstallAt string        `yaml:"first_install_at,omitempty"`
	UpdatedAt      string        `yaml:"updated_at,omitempty"`
	RemovedAt      string        `yaml:"removed_at,omitempty"`
	CheckedAt      string        `yaml:"checked_at,omitempty"`
	LastError      string        `yaml:"last_error,omitempty"`
}

// New creates a new empty state file for a provider.
func New(provider, machineID string) *File {
	return &File{
		SchemaVersion: SchemaVersion,
		Provider:      provider,
		MachineID:     machineID,
		Resources:     make(map[string]*Resource),
	}
}

// Load reads a state file from disk, auto-migrating legacy schemas.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path) //nolint:gosec // state paths are derived from store directory
	if err != nil {
		return nil, fmt.Errorf("reading state file %s: %w", path, err)
	}

	var legacy legacyFile
	if unmarshalErr := yaml.Unmarshal(data, &legacy); unmarshalErr != nil {
		return nil, fmt.Errorf("parsing state file %s: %w", path, unmarshalErr)
	}

	f, err := migrate(&legacy, path)
	if err != nil {
		return nil, err
	}

	if f.Resources == nil {
		f.Resources = make(map[string]*Resource)
	}

	return f, nil
}

// Save writes the state file to disk atomically.
func (f *File) Save(path string) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	return hamsfile.AtomicWrite(path, data)
}

// SetResource updates or creates a resource entry in state.
func (f *File) SetResource(id string, s ResourceState, opts ...ResourceOption) {
	r, ok := f.Resources[id]
	if !ok {
		r = &Resource{}
		f.Resources[id] = r
	}

	r.State = s
	now := time.Now().Format(timestampFormat)

	switch s {
	case StateOK:
		if r.FirstInstallAt == "" {
			r.FirstInstallAt = now
		}
		r.RemovedAt = ""
		r.UpdatedAt = now
		r.LastError = ""
	case StatePending:
		// No timestamp update for pending.
	case StateFailed:
		r.UpdatedAt = now
	case StateRemoved:
		r.RemovedAt = now
		r.UpdatedAt = now
	case StateHookFailed:
		r.UpdatedAt = now
	}

	for _, opt := range opts {
		opt(r)
	}
}

// ResourceOption is a functional option for SetResource.
type ResourceOption func(*Resource)

// WithVersion sets the version field.
func WithVersion(v string) ResourceOption {
	return func(r *Resource) { r.Version = v }
}

// WithValue sets the value field (for KV config resources).
func WithValue(v string) ResourceOption {
	return func(r *Resource) { r.Value = v }
}

// WithError sets the last_error field.
func WithError(e string) ResourceOption {
	return func(r *Resource) { r.LastError = e }
}

// WithCheckCmd sets the check command to re-run during probe.
func WithCheckCmd(s string) ResourceOption {
	return func(r *Resource) { r.CheckCmd = s }
}

// WithCheckStdout sets the check_stdout fingerprint.
func WithCheckStdout(s string) ResourceOption {
	return func(r *Resource) { r.CheckStdout = s }
}

// PendingResources returns all resource IDs with a non-ok state.
func (f *File) PendingResources() []string {
	var ids []string
	for id, r := range f.Resources {
		if r.State != StateOK && r.State != StateRemoved {
			ids = append(ids, id)
		}
	}
	return ids
}
