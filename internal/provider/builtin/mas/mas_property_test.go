package mas

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ParseMasList_NoPanicOnArbitraryInput asserts the parser
// never panics regardless of upstream `mas list` output format.
func TestProperty_ParseMasList_NoPanicOnArbitraryInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		_ = parseMasList(input)
	})
}

// TestProperty_ParseMasList_Idempotent asserts repeated calls return
// the same result on identical input.
func TestProperty_ParseMasList_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		first := parseMasList(input)
		second := parseMasList(input)
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

// TestProperty_ParseMasList_KeysWellFormed asserts no parser-emitted
// key contains whitespace.
func TestProperty_ParseMasList_KeysWellFormed(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		result := parseMasList(input)
		for appID := range result {
			if appID == "" {
				t.Fatalf("empty key emitted from input %q", input)
			}
			if strings.ContainsAny(appID, " \t\n\r") {
				t.Fatalf("key %q contains whitespace from input %q", appID, input)
			}
		}
	})
}

// TestProperty_ParseMasList_RoundtripWellFormedInput verifies that
// synthesized "<appID>  Name (version)" lines round-trip through the
// parser as documented at mas.go:152.
func TestProperty_ParseMasList_RoundtripWellFormedInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 8).Draw(t, "n")
		entries := make(map[string]string, n)
		for range n {
			// mas appIDs are numeric (Mac App Store catalog IDs).
			appID := rapid.StringMatching(`[1-9][0-9]{6,10}`).Draw(t, "appID")
			ver := rapid.StringMatching(`[0-9]{1,3}\.[0-9]{1,3}\.?[0-9]{0,3}`).Draw(t, "version")
			entries[appID] = ver
		}

		var b strings.Builder
		i := 0
		for appID, ver := range entries {
			// Format: "<appID>  AppName (version)"
			// Use a single-token AppName so the parser's "last token in
			// parens" extraction unambiguously hits the version.
			b.WriteString(appID + "  Name" + b.String()[:0] + " (" + ver + ")\n")
			i++
		}

		got := parseMasList(b.String())
		if len(got) != len(entries) {
			t.Fatalf("synth output should round-trip %d entries; got %d (input was %q)", len(entries), len(got), b.String())
		}
		for appID, ver := range entries {
			if got[appID] != ver {
				t.Fatalf("appID %q: want version %q, got %q", appID, ver, got[appID])
			}
		}
	})
}
