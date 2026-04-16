package uv

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ParseUvToolList_NoPanicOnArbitraryInput asserts the
// parser never panics regardless of upstream `uv tool list` output.
func TestProperty_ParseUvToolList_NoPanicOnArbitraryInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		_ = parseUvToolList(input)
	})
}

// TestProperty_ParseUvToolList_Idempotent asserts repeated calls
// return the same result on identical input.
func TestProperty_ParseUvToolList_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		first := parseUvToolList(input)
		second := parseUvToolList(input)
		if len(first) != len(second) {
			t.Fatalf("non-idempotent: %d vs %d entries", len(first), len(second))
		}
		for k, v := range first {
			if second[k] != v {
				t.Fatalf("non-idempotent key %q: %q vs %q", k, v, second[k])
			}
		}
	})
}

// TestProperty_ParseUvToolList_KeysWellFormed asserts no parser-emitted
// key contains whitespace.
func TestProperty_ParseUvToolList_KeysWellFormed(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		result := parseUvToolList(input)
		for name := range result {
			if name == "" {
				t.Fatalf("empty key emitted from input %q", input)
			}
			if strings.ContainsAny(name, " \t\n\r") {
				t.Fatalf("key %q contains whitespace from input %q", name, input)
			}
		}
	})
}

// TestProperty_ParseUvToolList_RoundtripWellFormedInput verifies that
// synthesized "tool-name v1.2.3" lines round-trip through the parser
// as documented at uv.go:125.
func TestProperty_ParseUvToolList_RoundtripWellFormedInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 8).Draw(t, "n")
		entries := make(map[string]string, n)
		for range n {
			name := rapid.StringMatching(`[a-z][a-z0-9_-]{2,15}`).Draw(t, "name")
			ver := rapid.StringMatching(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`).Draw(t, "version")
			entries[name] = ver
		}

		var b strings.Builder
		for name, ver := range entries {
			b.WriteString(name + " v" + ver + "\n")
		}

		got := parseUvToolList(b.String())
		if len(got) != len(entries) {
			t.Fatalf("synth output should round-trip %d entries; got %d", len(entries), len(got))
		}
		for name, ver := range entries {
			if got[name] != ver {
				t.Fatalf("key %q: want version %q, got %q", name, ver, got[name])
			}
		}
	})
}
