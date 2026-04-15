package state

import "fmt"

// legacyFile mirrors File but also accepts the pre-v2 `install_at` field on
// each resource so v1 state files can be migrated forward on load.
type legacyFile struct {
	SchemaVersion int                        `yaml:"schema_version"`
	Provider      string                     `yaml:"provider"`
	MachineID     string                     `yaml:"machine_id"`
	LastApply     string                     `yaml:"last_apply_session,omitempty"`
	ConfigHash    string                     `yaml:"last_apply_config_hash,omitempty"`
	Resources     map[string]*legacyResource `yaml:"resources"`
}

type legacyResource struct {
	State          ResourceState `yaml:"state"`
	Version        string        `yaml:"version,omitempty"`
	Value          string        `yaml:"value,omitempty"`
	CheckCmd       string        `yaml:"check_cmd,omitempty"`
	CheckStdout    string        `yaml:"check_stdout,omitempty"`
	InstallAt      string        `yaml:"install_at,omitempty"`
	FirstInstallAt string        `yaml:"first_install_at,omitempty"`
	UpdatedAt      string        `yaml:"updated_at,omitempty"`
	RemovedAt      string        `yaml:"removed_at,omitempty"`
	CheckedAt      string        `yaml:"checked_at,omitempty"`
	LastError      string        `yaml:"last_error,omitempty"`
}

// migrate converts a loaded legacyFile into the current File shape, promoting
// schema_version and renaming install_at → first_install_at where needed.
// Files written at SchemaVersion (or higher in the future) are passed through;
// files newer than this binary understands are rejected.
func migrate(lf *legacyFile, path string) (*File, error) {
	if lf.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf(
			"state file %s uses schema version %d, but this hams binary only supports up to version %d. Run 'hams self-upgrade' to update",
			path, lf.SchemaVersion, SchemaVersion,
		)
	}

	resources := make(map[string]*Resource, len(lf.Resources))
	for id, lr := range lf.Resources {
		if lr == nil {
			continue
		}
		firstInstallAt := lr.FirstInstallAt
		if firstInstallAt == "" {
			firstInstallAt = lr.InstallAt
		}
		resources[id] = &Resource{
			State:          lr.State,
			Version:        lr.Version,
			Value:          lr.Value,
			CheckCmd:       lr.CheckCmd,
			CheckStdout:    lr.CheckStdout,
			FirstInstallAt: firstInstallAt,
			UpdatedAt:      lr.UpdatedAt,
			RemovedAt:      lr.RemovedAt,
			CheckedAt:      lr.CheckedAt,
			LastError:      lr.LastError,
		}
	}

	return &File{
		SchemaVersion: SchemaVersion,
		Provider:      lf.Provider,
		MachineID:     lf.MachineID,
		LastApply:     lf.LastApply,
		ConfigHash:    lf.ConfigHash,
		Resources:     resources,
	}, nil
}
