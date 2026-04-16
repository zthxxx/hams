package provider

import (
	"testing"

	"github.com/zthxxx/hams/internal/state"

	"pgregory.net/rapid"
)

func TestComputePlan_AllNew(t *testing.T) {
	observed := state.New("brew", "test")
	actions := ComputePlan([]string{"htop", "jq", "curl"}, observed, "")

	installs := FilterActions(actions, ActionInstall)
	if len(installs) != 3 {
		t.Errorf("installs = %d, want 3", len(installs))
	}
}

func TestComputePlan_AllInstalled(t *testing.T) {
	observed := state.New("brew", "test")
	observed.SetResource("htop", state.StateOK)
	observed.SetResource("jq", state.StateOK)

	actions := ComputePlan([]string{"htop", "jq"}, observed, "")
	skips := FilterActions(actions, ActionSkip)
	if len(skips) != 2 {
		t.Errorf("skips = %d, want 2", len(skips))
	}
}

func TestComputePlan_RetryFailed(t *testing.T) {
	observed := state.New("brew", "test")
	observed.SetResource("htop", state.StateOK)
	observed.SetResource("jq", state.StateFailed)

	actions := ComputePlan([]string{"htop", "jq"}, observed, "")
	installs := FilterActions(actions, ActionInstall)
	if len(installs) != 1 || installs[0].ID != "jq" {
		t.Errorf("installs = %v, want [jq]", installs)
	}
}

func TestComputePlan_RemoveFromConfig(t *testing.T) {
	observed := state.New("brew", "test")
	observed.SetResource("htop", state.StateOK)
	observed.SetResource("jq", state.StateOK)
	observed.SetResource("curl", state.StateOK)

	// curl removed from config.
	actions := ComputePlan([]string{"htop", "jq"}, observed, "previous-hash")
	removes := FilterActions(actions, ActionRemove)
	if len(removes) != 1 || removes[0].ID != "curl" {
		t.Errorf("removes = %v, want [curl]", removes)
	}
}

func TestComputePlan_NoRemoveWithoutBaseline(t *testing.T) {
	observed := state.New("brew", "test")
	observed.SetResource("htop", state.StateOK)
	observed.SetResource("extra", state.StateOK)

	// No baseline → no removes.
	actions := ComputePlan([]string{"htop"}, observed, "")
	removes := FilterActions(actions, ActionRemove)
	if len(removes) != 0 {
		t.Errorf("removes = %d, want 0 (no baseline)", len(removes))
	}
}

func TestComputePlan_ReinstallRemoved(t *testing.T) {
	observed := state.New("brew", "test")
	observed.SetResource("htop", state.StateRemoved)

	actions := ComputePlan([]string{"htop"}, observed, "")
	installs := FilterActions(actions, ActionInstall)
	if len(installs) != 1 {
		t.Errorf("installs = %d, want 1 (reinstall removed)", len(installs))
	}
}

func TestCountActions(t *testing.T) {
	actions := []Action{
		{ID: "a", Type: ActionInstall},
		{ID: "b", Type: ActionInstall},
		{ID: "c", Type: ActionSkip},
		{ID: "d", Type: ActionRemove},
	}

	counts := CountActions(actions)
	if counts[ActionInstall] != 2 {
		t.Errorf("install count = %d, want 2", counts[ActionInstall])
	}
	if counts[ActionSkip] != 1 {
		t.Errorf("skip count = %d, want 1", counts[ActionSkip])
	}
	if counts[ActionRemove] != 1 {
		t.Errorf("remove count = %d, want 1", counts[ActionRemove])
	}
}

// TestComputePlan_DedupsDuplicateDesired asserts that when the same
// app ID appears twice in the desired list (e.g. a user accidentally
// left htop under both `cli:` and `dev:` tags after a move), the
// resulting plan has exactly ONE action for that ID — not two.
// Without dedup, the apply loop would run `apt install htop` twice
// (idempotent but wasteful) and the final summary would show
// `installed=2` instead of 1.
func TestComputePlan_DedupsDuplicateDesired(t *testing.T) {
	observed := state.New("apt", "test")
	actions := ComputePlan([]string{"htop", "htop", "jq", "htop"}, observed, "")

	// Expect 2 actions total: one for htop, one for jq.
	if len(actions) != 2 {
		t.Errorf("len(actions) = %d, want 2 (one per unique ID)", len(actions))
	}

	seen := map[string]int{}
	for _, a := range actions {
		seen[a.ID]++
	}
	if seen["htop"] != 1 {
		t.Errorf("htop appeared %d times, want 1", seen["htop"])
	}
	if seen["jq"] != 1 {
		t.Errorf("jq appeared %d times, want 1", seen["jq"])
	}
}

// TestComputePlan_DedupsPreservesFirstOccurrenceOrder asserts the
// iteration order of the returned actions matches the order of the
// FIRST occurrence of each ID in the desired slice. Users rely on
// provider-level ordering for hook ordering + preview output.
func TestComputePlan_DedupsPreservesFirstOccurrenceOrder(t *testing.T) {
	observed := state.New("apt", "test")
	actions := ComputePlan([]string{"c", "a", "b", "a", "c"}, observed, "")

	if len(actions) != 3 {
		t.Fatalf("len(actions) = %d, want 3", len(actions))
	}
	want := []string{"c", "a", "b"}
	for i, a := range actions {
		if a.ID != want[i] {
			t.Errorf("actions[%d].ID = %q, want %q (first-occurrence order)", i, a.ID, want[i])
		}
	}
}

// Property: every desired resource appears in the action list exactly once.
func TestProperty_PlanCoversAllDesired(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 20).Draw(t, "n")
		desired := make([]string, n)
		for i := range n {
			desired[i] = rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "app")
		}

		observed := state.New("test", "test")
		actions := ComputePlan(desired, observed, "")

		actionIDs := make(map[string]bool)
		for _, a := range actions {
			actionIDs[a.ID] = true
		}

		for _, d := range desired {
			if !actionIDs[d] {
				t.Errorf("desired %q missing from actions", d)
			}
		}
	})
}
