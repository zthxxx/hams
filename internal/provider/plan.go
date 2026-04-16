package provider

import (
	"github.com/zthxxx/hams/internal/state"
)

// ComputePlan diffs desired resources (from hamsfile) against observed state.
// Returns a list of actions to execute. Duplicate entries in `desired`
// (e.g. the same app listed under two tags after a move-between-tags
// edit) are folded into a single action in first-occurrence order —
// emitting two actions for the same ID would cause the apply loop to
// install twice and the summary to double-count.
func ComputePlan(desired []string, observed *state.File, lastConfigHash string) []Action {
	var actions []Action

	desiredSet := make(map[string]bool, len(desired))
	for _, id := range desired {
		desiredSet[id] = true
	}

	// Resources in desired but not in state (or failed/pending) → install.
	// Iterate the raw slice (preserves first-occurrence order) but skip
	// any ID we've already emitted an action for — hamsfile drift can
	// produce the same app under two tags, and the apply loop must
	// process each resource once.
	seen := make(map[string]bool, len(desired))
	for _, id := range desired {
		if seen[id] {
			continue
		}
		seen[id] = true
		r, exists := observed.Resources[id]
		if !exists {
			actions = append(actions, Action{ID: id, Type: ActionInstall})
			continue
		}

		switch r.State {
		case state.StateOK:
			actions = append(actions, Action{ID: id, Type: ActionSkip})
		case state.StateFailed, state.StatePending, state.StateHookFailed:
			actions = append(actions, Action{ID: id, Type: ActionInstall})
		case state.StateRemoved:
			// Was removed but now back in config → reinstall.
			actions = append(actions, Action{ID: id, Type: ActionInstall})
		}
	}

	// Resources in state but not in desired → remove candidates.
	// Only consider resources that were in the last-applied config (baseline).
	if lastConfigHash != "" {
		for id, r := range observed.Resources {
			if desiredSet[id] {
				continue
			}
			if r.State == state.StateRemoved {
				continue
			}
			actions = append(actions, Action{ID: id, Type: ActionRemove})
		}
	}

	return actions
}

// FilterActions returns only actions of the specified types.
func FilterActions(actions []Action, types ...ActionType) []Action {
	typeSet := make(map[ActionType]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}

	var filtered []Action
	for _, a := range actions {
		if typeSet[a.Type] {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

// CountActions returns counts by action type.
func CountActions(actions []Action) map[ActionType]int {
	counts := make(map[ActionType]int)
	for _, a := range actions {
		counts[a.Type]++
	}
	return counts
}
