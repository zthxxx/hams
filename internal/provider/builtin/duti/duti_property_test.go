package duti

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ParseResourceID_NoPanic asserts the parser never
// panics on arbitrary input. State entries can carry malformed IDs
// after manual edits / merge conflicts.
func TestProperty_ParseResourceID_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		_, _, err := parseResourceID(input)
		_ = err // only asserting no-panic; error is expected on malformed input
	})
}

// TestProperty_ParseResourceID_Idempotent asserts repeated calls on
// the same input return the same outputs.
func TestProperty_ParseResourceID_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		e1, b1, err1 := parseResourceID(input)
		e2, b2, err2 := parseResourceID(input)
		if e1 != e2 || b1 != b2 || (err1 == nil) != (err2 == nil) {
			t.Fatalf("non-idempotent: (%q,%q,%v) vs (%q,%q,%v)", e1, b1, err1, e2, b2, err2)
		}
	})
}

// TestProperty_ParseResourceID_WellFormedRoundTrip asserts a
// well-formed `<ext>=<bundle>` id with non-empty pieces parses
// successfully with the expected components.
func TestProperty_ParseResourceID_WellFormedRoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		ext := rapid.StringMatching(`[a-z]{2,5}`).Draw(t, "ext")
		bundle := rapid.StringMatching(`[a-z]+(\.[a-z]+){2,4}`).Draw(t, "bundle")

		id := ext + "=" + bundle
		gotExt, gotBundle, err := parseResourceID(id)
		if err != nil {
			t.Fatalf("well-formed parse failed: %v (id=%q)", err, id)
		}
		if gotExt != ext || gotBundle != bundle {
			t.Fatalf("parse mismatch: (%q,%q), want (%q,%q)", gotExt, gotBundle, ext, bundle)
		}
	})
}

// TestProperty_ParseResourceID_NoEqualsFailsClosed asserts an input
// without `=` returns an error.
func TestProperty_ParseResourceID_NoEqualsFailsClosed(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.StringMatching(`[a-zA-Z0-9.]{0,20}`).Draw(t, "no-equals")
		if strings.Contains(input, "=") {
			return // skip — has `=`
		}
		if _, _, err := parseResourceID(input); err == nil {
			t.Fatalf("expected error for input without `=`, got nil (input=%q)", input)
		}
	})
}

// TestProperty_ParseDutiOutput_NoPanic asserts parseDutiOutput never
// panics on arbitrary input. Production fed this is raw `duti -x`
// stdout — could be anything if duti's format changes.
func TestProperty_ParseDutiOutput_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "output")
		_ = parseDutiOutput(input)
	})
}

// TestProperty_ParseDutiOutput_FirstNonBlankLine asserts the parser
// returns the first non-blank (trimmed) line, or "" if all lines are
// blank. To keep the expected-value computation consistent with the
// implementation, we split the raw input ourselves rather than
// trusting the pre-join `lines` slice (which could have embedded
// newlines that re-split after joining).
func TestProperty_ParseDutiOutput_FirstNonBlankLine(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "output")
		got := parseDutiOutput(input)

		var want string
		for line := range strings.SplitSeq(input, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				want = trimmed
				break
			}
		}
		if got != want {
			t.Fatalf("parseDutiOutput(%q) = %q, want %q", input, got, want)
		}
	})
}
