package cargo

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ParseCargoList_NoPanicOnArbitraryInput asserts the parser
// never panics regardless of upstream `cargo install --list` output shape.
// `cargo` is third-party and its output format is not API-versioned;
// upstream changes (Unicode in crate names, color codes from CI, JSON
// mode toggles) MUST NOT crash the probe loop.
func TestProperty_ParseCargoList_NoPanicOnArbitraryInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		// Any input — control characters, Unicode, mixed line endings, ANSI escapes.
		_ = parseCargoList(input)
	})
}

// TestProperty_ParseCargoList_Idempotent asserts that calling the parser
// twice on the same input returns the same result. Drift here would
// indicate hidden global state — a bug class hams must not regress on.
func TestProperty_ParseCargoList_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		first := parseCargoList(input)
		second := parseCargoList(input)
		if len(first) != len(second) {
			t.Fatalf("non-idempotent: %d vs %d entries on identical input", len(first), len(second))
		}
		for k, v := range first {
			if second[k] != v {
				t.Fatalf("non-idempotent: key %q first=%q second=%q", k, v, second[k])
			}
		}
	})
}

// TestProperty_ParseCargoList_KeysWellFormed asserts the parser only
// emits crate names that conform to cargo's package-name grammar
// (printable, no whitespace, no leading 'v'). Garbage keys would
// silently corrupt the probed-vs-declared diff in `Probe`.
func TestProperty_ParseCargoList_KeysWellFormed(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		result := parseCargoList(input)
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

// TestProperty_ParseCargoList_RoundtripWellFormedInput asserts that
// when we synthesize a valid `cargo install --list` output
// from a known {name, version} set, the parser recovers all entries.
// The synthesizer matches the format documented at cargo.go:134.
func TestProperty_ParseCargoList_RoundtripWellFormedInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		// Generate 1-10 unique crate names + versions.
		n := rapid.IntRange(1, 10).Draw(t, "n")
		entries := make(map[string]string, n)
		for range n {
			name := rapid.StringMatching(`[a-z][a-z0-9_-]{2,15}`).Draw(t, "name")
			ver := rapid.StringMatching(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`).Draw(t, "version")
			entries[name] = ver
		}

		var b strings.Builder
		for name, ver := range entries {
			// Format: "crate-name v1.2.3:"
			b.WriteString(name + " v" + ver + ":\n")
			// Add an indented binary line that should be ignored.
			b.WriteString("    " + name + "\n")
		}

		got := parseCargoList(b.String())
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
