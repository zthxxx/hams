package provider_test

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

func TestDiffDesiredVsState_PropertyBased(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		// Generate random sets of desired and observed resource IDs.
		desiredIDs := rapid.SliceOfN(rapid.StringMatching(`[a-z][a-z0-9-]{0,9}`), 0, 10).Draw(t, "desired")
		observedIDs := rapid.SliceOfN(rapid.StringMatching(`[a-z][a-z0-9-]{0,9}`), 0, 10).Draw(t, "observed")

		// Build a hamsfile with desired resources.
		hf := buildTestHamsfile(desiredIDs)
		sf := state.New("test", "machine")
		for _, id := range observedIDs {
			sf.SetResource(id, state.StateOK)
		}

		diff := provider.DiffDesiredVsState(hf, sf)

		// Invariant: every desired ID must appear in exactly one category.
		desiredSet := toSet(desiredIDs)
		observedSet := toSet(observedIDs)

		for _, e := range diff.Additions {
			if observedSet[e.ID] {
				t.Errorf("addition %q should not be in observed set", e.ID)
			}
			if !desiredSet[e.ID] {
				t.Errorf("addition %q should be in desired set", e.ID)
			}
		}
		for _, e := range diff.Removals {
			if desiredSet[e.ID] {
				t.Errorf("removal %q should not be in desired set", e.ID)
			}
			if !observedSet[e.ID] {
				t.Errorf("removal %q should be in observed set", e.ID)
			}
		}
		for _, e := range diff.Matched {
			if !desiredSet[e.ID] || !observedSet[e.ID] {
				t.Errorf("matched %q should be in both sets", e.ID)
			}
		}
		for _, e := range diff.Diverged {
			if !desiredSet[e.ID] || !observedSet[e.ID] {
				t.Errorf("diverged %q should be in both sets", e.ID)
			}
		}
	})
}

// TestFormatDiff_EmptyDiffShowsFriendlyHint locks in cycle 210:
// an empty diff (no additions, removals, matched, diverged) must
// emit a user-visible hint, not an empty string. Pre-cycle-210
// `hams git-config list` on a fresh/empty store printed NOTHING
// and exited 0 — indistinguishable from "command crashed before
// output" or "provider forgot to print anything". Users couldn't
// tell if it worked. Hint points the user at how to seed the state.
func TestFormatDiff_EmptyDiffShowsFriendlyHint(t *testing.T) {
	t.Parallel()
	diff := provider.DiffResult{}
	out := provider.FormatDiff(&diff)
	if out == "" {
		t.Fatal("empty diff should emit a friendly hint, not an empty string")
	}
	if !strings.Contains(out, "No entries tracked") {
		t.Errorf("empty-diff hint should mention 'No entries tracked'; got %q", out)
	}
	if !strings.Contains(out, "install") {
		t.Errorf("empty-diff hint should suggest the `install` subcommand; got %q", out)
	}
}

// TestFormatDiff_NonEmptyDiffSkipsHint asserts the inverse: any
// non-empty diff returns the normal +/~/-/ok lines WITHOUT the
// "No entries tracked" hint appearing as noise at the top.
func TestFormatDiff_NonEmptyDiffSkipsHint(t *testing.T) {
	t.Parallel()
	diff := provider.DiffResult{
		Matched: []provider.DiffEntry{{ID: "git", Type: "matched", Status: "ok"}},
	}
	out := provider.FormatDiff(&diff)
	if strings.Contains(out, "No entries tracked") {
		t.Errorf("non-empty diff should NOT include empty-hint text; got %q", out)
	}
	if !strings.Contains(out, "git") {
		t.Errorf("output should contain the tracked resource; got %q", out)
	}
}

func TestFormatDiff_ShowsMarkers(t *testing.T) {
	t.Parallel()
	diff := provider.DiffResult{
		Additions: []provider.DiffEntry{{ID: "curl", Type: "addition"}},
		Removals:  []provider.DiffEntry{{ID: "wget", Type: "removal", Status: "ok"}},
		Matched:   []provider.DiffEntry{{ID: "git", Type: "matched", Status: "ok"}},
		Diverged:  []provider.DiffEntry{{ID: "vim", Type: "diverged", Status: "failed"}},
	}

	out := provider.FormatDiff(&diff)

	if !strings.Contains(out, "+ curl") {
		t.Error("expected + marker for additions")
	}
	if !strings.Contains(out, "- wget") {
		t.Error("expected - marker for removals")
	}
	if !strings.Contains(out, "~ vim") {
		t.Error("expected ~ marker for diverged")
	}
	if !strings.Contains(out, "git") {
		t.Error("expected git in matched output")
	}
}

func TestFormatDiffJSON_ValidJSON(t *testing.T) {
	t.Parallel()
	diff := provider.DiffResult{
		Additions: []provider.DiffEntry{{ID: "curl", Type: "addition"}},
	}
	out, err := provider.FormatDiffJSON(&diff)
	if err != nil {
		t.Fatalf("FormatDiffJSON error: %v", err)
	}
	if !strings.Contains(out, `"curl"`) {
		t.Error("expected curl in JSON output")
	}
}

// TestDiffDesiredVsState_DeterministicOrder asserts that each
// category in the returned DiffResult is sorted by ID. Without
// this guarantee, Go's non-deterministic map iteration would
// shuffle the rows of `hams <provider> list` on every call —
// any user piping the output through grep/diff/snapshot tools
// would see flaky results.
func TestDiffDesiredVsState_DeterministicOrder(t *testing.T) {
	t.Parallel()
	hf := buildTestHamsfile([]string{"zsh", "git", "vim", "curl", "htop"})
	sf := state.New("test", "machine")
	// Mix of matched, diverged, and removed-from-desired so all 4
	// categories are populated.
	sf.SetResource("git", state.StateOK)
	sf.SetResource("vim", state.StateFailed)
	sf.SetResource("htop", state.StateOK)
	sf.SetResource("orphan-a", state.StateOK)
	sf.SetResource("orphan-b", state.StateOK)
	sf.SetResource("orphan-c", state.StateOK)

	first := provider.DiffDesiredVsState(hf, sf)

	// Run 20 more times; every run must produce identical slices in
	// every category. A non-deterministic order would flap somewhere
	// across these reps.
	for range 20 {
		again := provider.DiffDesiredVsState(hf, sf)
		assertEntrySliceEqual(t, "Additions", first.Additions, again.Additions)
		assertEntrySliceEqual(t, "Removals", first.Removals, again.Removals)
		assertEntrySliceEqual(t, "Matched", first.Matched, again.Matched)
		assertEntrySliceEqual(t, "Diverged", first.Diverged, again.Diverged)
	}

	// Also assert each category is actually sorted, not merely stable
	// (a stable but non-sorted result would still satisfy the loop
	// above but break user expectations of alphabetical output).
	assertSortedByID(t, "Additions", first.Additions)
	assertSortedByID(t, "Removals", first.Removals)
	assertSortedByID(t, "Matched", first.Matched)
	assertSortedByID(t, "Diverged", first.Diverged)
}

func assertEntrySliceEqual(t *testing.T, name string, a, b []provider.DiffEntry) {
	t.Helper()
	if len(a) != len(b) {
		t.Errorf("%s length differs across runs: %d vs %d", name, len(a), len(b))
		return
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Errorf("%s[%d].ID differs across runs: %q vs %q", name, i, a[i].ID, b[i].ID)
		}
	}
}

func assertSortedByID(t *testing.T, name string, entries []provider.DiffEntry) {
	t.Helper()
	for i := 1; i < len(entries); i++ {
		if entries[i-1].ID > entries[i].ID {
			t.Errorf("%s not sorted: %q > %q at indices %d,%d",
				name, entries[i-1].ID, entries[i].ID, i-1, i)
		}
	}
}

func buildTestHamsfile(apps []string) *hamsfile.File {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, app := range apps {
		entry := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "app", Tag: "!!str"},
				{Kind: yaml.ScalarNode, Value: app, Tag: "!!str"},
			},
		}
		seq.Content = append(seq.Content, entry)
	}

	mapping := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "test-tag", Tag: "!!str"},
			seq,
		},
	}

	return &hamsfile.File{
		Path: "/tmp/test.hams.yaml",
		Root: &yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{mapping},
		},
	}
}

func toSet(ids []string) map[string]bool {
	s := make(map[string]bool)
	for _, id := range ids {
		s[id] = true
	}
	return s
}
