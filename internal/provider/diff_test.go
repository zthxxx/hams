package provider_test

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
	"gopkg.in/yaml.v3"
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
