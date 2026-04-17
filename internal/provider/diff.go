package provider

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// DiffType categorizes a resource's relationship between desired and observed state.
type DiffType string

const (
	// DiffAddition indicates a resource exists in desired but not in observed state.
	DiffAddition DiffType = "addition"
	// DiffRemoval indicates a resource exists in observed but not in desired state.
	DiffRemoval DiffType = "removal"
	// DiffMatched indicates a resource exists in both and is in sync.
	DiffMatched DiffType = "matched"
	// DiffDiverged indicates a resource exists in both but has drifted.
	DiffDiverged DiffType = "diverged"
)

// DiffEntry represents a single resource in the diff between desired and observed state.
type DiffEntry struct {
	ID     string   `json:"id"`
	Type   DiffType `json:"type"`
	Status string   `json:"status"` // State from state file (empty for additions).
}

// DiffResult holds the full diff between Hamsfile (desired) and state (observed).
type DiffResult struct {
	Additions []DiffEntry `json:"additions"`
	Removals  []DiffEntry `json:"removals"`
	Matched   []DiffEntry `json:"matched"`
	Diverged  []DiffEntry `json:"diverged"`
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
			continue
		}
		observedSet[id] = true
	}

	// Single pass over desired: classify as addition, matched, or diverged.
	for app := range desiredSet {
		if !observedSet[app] {
			result.Additions = append(result.Additions, DiffEntry{ID: app, Type: DiffAddition})
			continue
		}
		r := observed.Resources[app]
		if r.State == state.StateOK {
			result.Matched = append(result.Matched, DiffEntry{ID: app, Type: DiffMatched, Status: string(r.State)})
		} else {
			result.Diverged = append(result.Diverged, DiffEntry{ID: app, Type: DiffDiverged, Status: string(r.State)})
		}
	}

	for id := range observedSet {
		if !desiredSet[id] {
			r := observed.Resources[id]
			result.Removals = append(result.Removals, DiffEntry{ID: id, Type: DiffRemoval, Status: string(r.State)})
		}
	}

	// Sort each category by ID so `hams <provider> list` output is
	// stable across invocations. Without this, Go's non-deterministic
	// map iteration order shuffles the rows on every call, breaking
	// grep/diff/snapshot workflows over the output.
	sortByID := func(entries []DiffEntry) {
		sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	}
	sortByID(result.Additions)
	sortByID(result.Removals)
	sortByID(result.Matched)
	sortByID(result.Diverged)

	return result
}

// FormatDiff renders a DiffResult as a human-readable string with +/-/~ markers.
func FormatDiff(diff *DiffResult) string {
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
func FormatDiffJSON(diff *DiffResult) (string, error) {
	data, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling diff: %w", err)
	}
	return string(data), nil
}
