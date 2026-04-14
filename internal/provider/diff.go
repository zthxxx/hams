package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// DiffEntry represents a single resource in the diff between desired and observed state.
type DiffEntry struct {
	ID     string `json:"id"`
	Type   string `json:"type"`   // "addition", "removal", "matched", "diverged"
	Status string `json:"status"` // State from state file (empty for additions).
}

// DiffResult holds the full diff between Hamsfile (desired) and state (observed).
type DiffResult struct {
	Additions []DiffEntry `json:"additions"` // In Hamsfile but not in state.
	Removals  []DiffEntry `json:"removals"`  // In state but not in Hamsfile.
	Matched   []DiffEntry `json:"matched"`   // In both with consistent status.
	Diverged  []DiffEntry `json:"diverged"`  // In both but status differs from OK.
}

// DiffDesiredVsState compares Hamsfile resources against state resources.
func DiffDesiredVsState(desired *hamsfile.File, observed *state.File) DiffResult {
	var result DiffResult

	desiredSet := make(map[string]bool)
	for _, app := range desired.ListApps() {
		desiredSet[app] = true
	}

	observedSet := make(map[string]bool)
	for id := range observed.Resources {
		if observed.Resources[id].State == state.StateRemoved {
			continue // Skip removed resources.
		}
		observedSet[id] = true
	}

	// Additions: in desired but not in observed.
	for app := range desiredSet {
		if !observedSet[app] {
			result.Additions = append(result.Additions, DiffEntry{ID: app, Type: "addition"})
		}
	}

	// Removals: in observed but not in desired.
	for id := range observedSet {
		if !desiredSet[id] {
			r := observed.Resources[id]
			result.Removals = append(result.Removals, DiffEntry{ID: id, Type: "removal", Status: string(r.State)})
		}
	}

	// Matched/Diverged: in both.
	for app := range desiredSet {
		if !observedSet[app] {
			continue
		}
		r := observed.Resources[app]
		if r.State == state.StateOK {
			result.Matched = append(result.Matched, DiffEntry{ID: app, Type: "matched", Status: string(r.State)})
		} else {
			result.Diverged = append(result.Diverged, DiffEntry{ID: app, Type: "diverged", Status: string(r.State)})
		}
	}

	return result
}

// FormatDiff renders a DiffResult as a human-readable string with +/-/~ markers.
func FormatDiff(diff DiffResult) string {
	var sb strings.Builder

	for _, e := range diff.Additions {
		fmt.Fprintf(&sb, "  + %-30s (not installed)\n", e.ID)
	}
	for _, e := range diff.Diverged {
		fmt.Fprintf(&sb, "  ~ %-30s (%s)\n", e.ID, e.Status)
	}
	for _, e := range diff.Matched {
		fmt.Fprintf(&sb, "    %-30s ok\n", e.ID)
	}
	for _, e := range diff.Removals {
		fmt.Fprintf(&sb, "  - %-30s (%s, not in config)\n", e.ID, e.Status)
	}

	return sb.String()
}

// FormatDiffJSON renders a DiffResult as JSON.
func FormatDiffJSON(diff DiffResult) (string, error) {
	data, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling diff: %w", err)
	}
	return string(data), nil
}
