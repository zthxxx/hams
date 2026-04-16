package defaults

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ParseDomainKey_NoPanic asserts the parser never
// panics on arbitrary input. Real input comes from state entries
// with shape "<domain>.<key>=<type>:<value>", but drift / manual
// edits / corrupt state files can feed anything here.
func TestProperty_ParseDomainKey_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		_, _ = parseDomainKey(input)
	})
}

// TestProperty_ParseDomainKey_Idempotent asserts repeated calls on
// the same input return the same (domain, key) pair.
func TestProperty_ParseDomainKey_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		d1, k1 := parseDomainKey(input)
		d2, k2 := parseDomainKey(input)
		if d1 != d2 || k1 != k2 {
			t.Fatalf("non-idempotent: (%q,%q) vs (%q,%q)", d1, k1, d2, k2)
		}
	})
}

// TestProperty_ParseDefaultsResource_NoPanic asserts the parser
// fails closed (returns error) but never panics on arbitrary input.
func TestProperty_ParseDefaultsResource_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		_, _, _, _, err := parseDefaultsResource(input)
		_ = err // only asserting no-panic; error is expected on malformed input
	})
}

// TestProperty_ParseDefaultsResource_WellFormedRoundTrip asserts a
// well-formed resource ID of shape `<domain>.<key>=<type>:<value>`
// with non-empty pieces round-trips: parse → reconstruct → parse
// produces identical fields.
func TestProperty_ParseDefaultsResource_WellFormedRoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		// Generate non-empty alphanumeric components (avoid `=`, `:`,
		// `.` in the key field so it doesn't collide with separators).
		domain := rapid.StringMatching(`[a-z]+(\.[a-z]+){1,3}`).Draw(t, "domain")
		key := rapid.StringMatching(`[a-zA-Z]+`).Draw(t, "key")
		typeStr := rapid.SampledFrom([]string{"bool", "int", "string", "float", "array"}).Draw(t, "type")
		value := rapid.StringMatching(`[a-zA-Z0-9]+`).Draw(t, "value")

		id := domain + "." + key + "=" + typeStr + ":" + value
		d, k, ts, v, err := parseDefaultsResource(id)
		if err != nil {
			t.Fatalf("well-formed parse failed: %v (id=%q)", err, id)
		}
		if d != domain || k != key || ts != typeStr || v != value {
			t.Fatalf("round-trip mismatch: got (%q,%q,%q,%q), want (%q,%q,%q,%q)",
				d, k, ts, v, domain, key, typeStr, value)
		}
	})
}

// TestProperty_ParseDefaultsResource_NoEqualsFailsClosed asserts an
// input without `=` returns an error rather than bogus output.
func TestProperty_ParseDefaultsResource_NoEqualsFailsClosed(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.StringMatching(`[a-zA-Z0-9. ]{0,40}`).Draw(t, "no-equals")
		if strings.Contains(input, "=") {
			return // skip — happens to contain `=`
		}
		if _, _, _, _, err := parseDefaultsResource(input); err == nil {
			t.Fatalf("expected error for input without `=`, got nil (input=%q)", input)
		}
	})
}
