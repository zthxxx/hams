package git

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_SplitResourceKey_NoPanic asserts splitResourceKey
// never panics regardless of input — including empty strings, inputs
// without `=`, inputs with multiple `=`, or control bytes. The
// production caller feeds this arbitrary state-file resource IDs, so
// a panic here would crash the refresh loop.
func TestProperty_SplitResourceKey_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		_ = splitResourceKey(input)
	})
}

// TestProperty_SplitResourceKey_Idempotent asserts calling
// splitResourceKey twice on the same input returns the same string.
// No hidden global state.
func TestProperty_SplitResourceKey_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		if got1, got2 := splitResourceKey(input), splitResourceKey(input); got1 != got2 {
			t.Fatalf("non-idempotent: %q vs %q", got1, got2)
		}
	})
}

// TestProperty_SplitResourceKey_PrefixInvariant asserts the output is
// always a prefix of the input. This is the core semantic:
// "everything before the first `=`".
func TestProperty_SplitResourceKey_PrefixInvariant(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		got := splitResourceKey(input)
		if !strings.HasPrefix(input, got) {
			t.Fatalf("output %q is not a prefix of input %q", got, input)
		}
	})
}

// TestProperty_SplitResourceKey_NoEqualsInOutput asserts `=` never
// appears in the returned key — it's the boundary character by
// definition.
func TestProperty_SplitResourceKey_NoEqualsInOutput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "resourceID")
		got := splitResourceKey(input)
		if strings.Contains(got, "=") {
			t.Fatalf("output %q contains `=` (should have been split out)", got)
		}
	})
}
