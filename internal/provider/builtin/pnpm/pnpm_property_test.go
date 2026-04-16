package pnpm

import (
	"encoding/json"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ParsePnpmList_NoPanicOnArbitraryInput asserts the JSON
// parser never panics regardless of upstream `pnpm list -g --json`
// output shape.
func TestProperty_ParsePnpmList_NoPanicOnArbitraryInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		_ = parsePnpmList(input)
	})
}

// TestProperty_ParsePnpmList_Idempotent asserts repeated calls return
// the same result on identical input.
func TestProperty_ParsePnpmList_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		first := parsePnpmList(input)
		second := parsePnpmList(input)
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

// TestProperty_ParsePnpmList_InvalidJSONReturnsEmpty asserts the
// parser fails closed on non-JSON input.
func TestProperty_ParsePnpmList_InvalidJSONReturnsEmpty(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.StringMatching(`[a-zA-Z !@#%^&*(),.?";|]{0,40}`).Draw(t, "trash")
		var probe map[string]any
		if json.Unmarshal([]byte(input), &probe) == nil {
			return
		}
		got := parsePnpmList(input)
		if len(got) != 0 {
			t.Fatalf("expected empty map on invalid JSON %q, got %d entries", input, len(got))
		}
	})
}

// TestProperty_ParsePnpmList_RoundtripWellFormedInput verifies that
// synthesized {"dependencies": {...}} JSON round-trips through the parser.
func TestProperty_ParsePnpmList_RoundtripWellFormedInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 8).Draw(t, "n")
		names := make(map[string]bool, n)
		for range n {
			name := rapid.StringMatching(`[a-z][a-z0-9_-]{2,15}`).Draw(t, "name")
			names[name] = true
		}

		deps := make(map[string]map[string]string, len(names))
		for name := range names {
			deps[name] = map[string]string{"version": "1.0.0"}
		}
		raw, err := json.Marshal(map[string]any{"dependencies": deps})
		if err != nil {
			t.Fatalf("JSON marshal failed: %v", err)
		}

		got := parsePnpmList(string(raw))
		if len(got) != len(names) {
			t.Fatalf("roundtrip: want %d entries, got %d", len(names), len(got))
		}
		for name := range names {
			if _, ok := got[name]; !ok {
				t.Fatalf("missing key %q", name)
			}
		}
		for k := range got {
			if !names[k] {
				t.Fatalf("unexpected key %q (probably leaked metadata)", k)
			}
		}
	})
}

// TestProperty_ParsePnpmList_KeysWellFormed asserts no parser-emitted
// key contains whitespace.
func TestProperty_ParsePnpmList_KeysWellFormed(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		result := parsePnpmList(input)
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
