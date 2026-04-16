package homebrew

import (
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_IsTapFormat_NoPanic asserts isTapFormat never panics
// on arbitrary input. Real inputs come from user hamsfile entries
// and brew's own output — both of which can be malformed.
func TestProperty_IsTapFormat_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.String().Draw(t, "name")
		_ = isTapFormat(name)
	})
}

// TestProperty_IsTapFormat_Idempotent asserts repeated calls on the
// same input return the same answer (no hidden global state).
func TestProperty_IsTapFormat_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.String().Draw(t, "name")
		first := isTapFormat(name)
		second := isTapFormat(name)
		if first != second {
			t.Fatalf("non-idempotent for %q: first=%v second=%v", name, first, second)
		}
	})
}

// TestProperty_IsTapFormat_NoSlashMeansFalse asserts a name without
// any `/` can never be a tap. Property encodes the tap format
// contract: taps are `user/repo`, everything else isn't.
func TestProperty_IsTapFormat_NoSlashMeansFalse(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.StringMatching(`[a-zA-Z0-9._-]+`).Draw(t, "name")
		if strings.Contains(name, "/") {
			return // skip — has slash
		}
		if isTapFormat(name) {
			t.Fatalf("isTapFormat(%q) = true, want false (no slash)", name)
		}
	})
}

// TestProperty_HasCaskFlag_NoPanic asserts hasCaskFlag never panics
// on arbitrary args.
func TestProperty_HasCaskFlag_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		args := rapid.SliceOf(rapid.String()).Draw(t, "args")
		_ = hasCaskFlag(args)
	})
}

// TestProperty_HasCaskFlag_ContainmentSemantics asserts the function
// is just slices.Contains — true iff "--cask" appears exactly in args.
func TestProperty_HasCaskFlag_ContainmentSemantics(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		args := rapid.SliceOf(rapid.String()).Draw(t, "args")
		want := slices.Contains(args, "--cask")
		if got := hasCaskFlag(args); got != want {
			t.Fatalf("hasCaskFlag(%v) = %v, want %v", args, got, want)
		}
	})
}

// TestProperty_PackageArgs_NoPanic asserts packageArgs never panics.
func TestProperty_PackageArgs_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		args := rapid.SliceOf(rapid.String()).Draw(t, "args")
		_ = packageArgs(args)
	})
}

// TestProperty_PackageArgs_DropsFlags asserts packageArgs drops any
// arg starting with `-`. This is the core contract: only package
// names (non-flag args) are forwarded to the auto-record path.
func TestProperty_PackageArgs_DropsFlags(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		args := rapid.SliceOf(rapid.String()).Draw(t, "args")
		got := packageArgs(args)
		for _, g := range got {
			if strings.HasPrefix(g, "-") {
				t.Fatalf("packageArgs returned flag-like arg %q", g)
			}
		}
	})
}

// TestProperty_ParseBrewInfoJSON_NoPanic asserts the JSON parser
// never panics on arbitrary bytes. Production feeds it stdout from
// `brew info --json`; schema changes or garbled output must not
// crash the probe loop.
func TestProperty_ParseBrewInfoJSON_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.SliceOfN(rapid.Byte(), 0, 1024).Draw(t, "input")
		_, err := parseBrewInfoJSON(input)
		_ = err // only asserting no-panic; error is expected on malformed JSON
	})
}

// TestProperty_ParseInstallTag_NoPanic asserts parseInstallTag never
// panics on arbitrary hamsFlags.
func TestProperty_ParseInstallTag_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		flags := rapid.MapOf(rapid.String(), rapid.String()).Draw(t, "flags")
		_ = parseInstallTag(flags)
	})
}

// TestProperty_ParseInstallTag_AlwaysReturnsNonEmpty asserts the
// returned tag is never empty. Empty tags would cause apt-style
// `AddApp("", name, ...)` calls to be rejected or silently miscreate
// entries.
func TestProperty_ParseInstallTag_AlwaysReturnsNonEmpty(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		flags := rapid.MapOf(rapid.String(), rapid.String()).Draw(t, "flags")
		if got := parseInstallTag(flags); got == "" {
			t.Fatalf("parseInstallTag(%v) = empty", flags)
		}
	})
}
