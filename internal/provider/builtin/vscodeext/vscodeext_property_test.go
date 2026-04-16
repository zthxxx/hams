package vscodeext

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ParseExtensionList_NoPanicOnArbitraryInput asserts the
// parser never panics regardless of upstream `code --list-extensions
// --show-versions` output.
func TestProperty_ParseExtensionList_NoPanicOnArbitraryInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		_ = parseExtensionList(input)
	})
}

// TestProperty_ParseExtensionList_Idempotent asserts repeated calls
// return the same result on identical input.
func TestProperty_ParseExtensionList_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		first := parseExtensionList(input)
		second := parseExtensionList(input)
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

// TestProperty_ParseExtensionList_KeysAlwaysLowercase asserts that the
// parser lowercases every emitted key. VS Code extension IDs are
// case-insensitive on the marketplace, but the on-disk install name
// from `code --list-extensions` is always lowercased; the parser MUST
// not regress on this normalization or the diff will spuriously flag
// "drift" between desired (mixed-case) and observed (lowercase).
func TestProperty_ParseExtensionList_KeysAlwaysLowercase(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		result := parseExtensionList(input)
		for k := range result {
			if k != strings.ToLower(k) {
				t.Fatalf("key %q is not lowercased (input was %q)", k, input)
			}
		}
	})
}

// TestProperty_ParseExtensionList_KeysWellFormed asserts no parser-
// emitted key contains tab/newline/carriage-return characters.
func TestProperty_ParseExtensionList_KeysWellFormed(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		result := parseExtensionList(input)
		for k := range result {
			if k == "" {
				t.Fatalf("empty key emitted from input %q", input)
			}
			if strings.ContainsAny(k, "\t\n\r") {
				t.Fatalf("key %q contains whitespace from input %q", k, input)
			}
		}
	})
}

// TestProperty_ParseExtensionList_RoundtripWellFormedInput verifies
// that synthesized "publisher.extension@version" lines round-trip
// through the parser. Names round-trip case-insensitively because the
// parser lowercases on emit.
func TestProperty_ParseExtensionList_RoundtripWellFormedInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 8).Draw(t, "n")
		entries := make(map[string]string, n)
		for range n {
			pub := rapid.StringMatching(`[a-z][a-z0-9-]{2,15}`).Draw(t, "pub")
			ext := rapid.StringMatching(`[a-z][a-z0-9-]{2,15}`).Draw(t, "ext")
			ver := rapid.StringMatching(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`).Draw(t, "version")
			entries[pub+"."+ext] = ver
		}

		var b strings.Builder
		for name, ver := range entries {
			b.WriteString(name + "@" + ver + "\n")
		}

		got := parseExtensionList(b.String())
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
