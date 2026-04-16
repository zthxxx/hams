package npm

import (
	"encoding/json"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ParseNpmList_NoPanicOnArbitraryInput asserts the JSON
// parser never panics regardless of upstream `npm list -g --json`
// output shape — including invalid JSON, non-JSON, control bytes.
func TestProperty_ParseNpmList_NoPanicOnArbitraryInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		_ = parseNpmList(input)
	})
}

// TestProperty_ParseNpmList_Idempotent asserts repeated calls return
// the same result on identical input — no hidden global state.
func TestProperty_ParseNpmList_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		first := parseNpmList(input)
		second := parseNpmList(input)
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

// TestProperty_ParseNpmList_InvalidJSONReturnsEmpty asserts the
// parser fails closed (empty map) on non-JSON input — never crashes
// the calling probe loop with a parse-error map shape.
func TestProperty_ParseNpmList_InvalidJSONReturnsEmpty(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		// Generate strings that are NOT valid JSON by ensuring no `{` is
		// present (or the input is short trash); the parser MUST return
		// an empty map without panic.
		input := rapid.StringMatching(`[a-zA-Z !@#%^&*(),.?";|]{0,40}`).Draw(t, "trash")
		// Verify it's actually invalid JSON to make the property meaningful.
		var probe map[string]any
		if json.Unmarshal([]byte(input), &probe) == nil {
			return // skip — happened to be valid JSON
		}
		got := parseNpmList(input)
		if len(got) != 0 {
			t.Fatalf("expected empty map on invalid JSON %q, got %d entries", input, len(got))
		}
	})
}

// TestProperty_ParseNpmList_RoundtripWellFormedInput verifies that
// for any synthesized {"dependencies": {...}} JSON, the parser
// recovers the full set of package names. Versions are intentionally
// dropped by the parser (only names matter for the diff).
func TestProperty_ParseNpmList_RoundtripWellFormedInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 8).Draw(t, "n")
		names := make(map[string]bool, n)
		for range n {
			name := rapid.StringMatching(`[a-z][a-z0-9_-]{2,15}`).Draw(t, "name")
			names[name] = true
		}

		// Build a JSON object equivalent to npm's output.
		deps := make(map[string]map[string]string, len(names))
		for name := range names {
			deps[name] = map[string]string{
				"version":  "1.0.0",
				"resolved": "https://registry.npmjs.org/" + name,
			}
		}
		raw, err := json.Marshal(map[string]any{"dependencies": deps})
		if err != nil {
			t.Fatalf("JSON marshal failed: %v", err)
		}

		got := parseNpmList(string(raw))
		if len(got) != len(names) {
			t.Fatalf("roundtrip: want %d entries, got %d", len(names), len(got))
		}
		for name := range names {
			if _, ok := got[name]; !ok {
				t.Fatalf("roundtrip: missing key %q in result", name)
			}
		}
		// Every key in result MUST come from the input set — nested
		// metadata keys ("version", "resolved") MUST NOT leak.
		for k := range got {
			if !names[k] {
				t.Fatalf("roundtrip: unexpected key %q (probably leaked metadata)", k)
			}
		}
	})
}

// TestProperty_ParseNpmList_KeysWellFormed asserts no parser-emitted
// key contains whitespace (would corrupt the probed-vs-declared diff).
func TestProperty_ParseNpmList_KeysWellFormed(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		result := parseNpmList(input)
		for name := range result {
			if name == "" {
				t.Fatalf("empty key emitted from input %q", input)
			}
			if strings.ContainsAny(name, "\n\r\t") {
				t.Fatalf("key %q contains whitespace from input %q", name, input)
			}
		}
	})
}
