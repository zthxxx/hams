package goinstall

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_BinaryNameFromPkg_NoPanic asserts binaryNameFromPkg
// never panics on arbitrary go package paths.
func TestProperty_BinaryNameFromPkg_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		pkg := rapid.String().Draw(t, "pkg")
		_ = binaryNameFromPkg(pkg)
	})
}

// TestProperty_BinaryNameFromPkg_Idempotent asserts repeated calls
// produce the same result (no hidden global state).
func TestProperty_BinaryNameFromPkg_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		pkg := rapid.String().Draw(t, "pkg")
		first := binaryNameFromPkg(pkg)
		second := binaryNameFromPkg(pkg)
		if first != second {
			t.Fatalf("non-idempotent for %q: first=%q second=%q", pkg, first, second)
		}
	})
}

// TestProperty_BinaryNameFromPkg_NoSlashOrVersion asserts the
// binary name never contains `/` or `@` — those are package path
// separators, not valid filename characters.
func TestProperty_BinaryNameFromPkg_NoSlashOrVersion(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		pkg := rapid.String().Draw(t, "pkg")
		got := binaryNameFromPkg(pkg)
		if strings.ContainsAny(got, "/@") {
			t.Fatalf("binaryNameFromPkg(%q) = %q contains `/` or `@`", pkg, got)
		}
	})
}

// TestProperty_InjectLatest_NoPanic asserts injectLatest never
// panics on arbitrary resource IDs.
func TestProperty_InjectLatest_NoPanic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		id := rapid.String().Draw(t, "resourceID")
		_ = injectLatest(id)
	})
}

// TestProperty_InjectLatest_AlwaysContainsAt asserts the output
// always has an `@` somewhere — injectLatest's job is to ensure
// a version tag is present.
func TestProperty_InjectLatest_AlwaysContainsAt(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		// Generate inputs that don't already contain `@` so the
		// function has work to do.
		id := rapid.StringMatching(`[a-z]+(\.[a-z]+){1,3}/[a-z/-]+`).Draw(t, "pkg")
		if strings.Contains(id, "@") {
			return
		}
		got := injectLatest(id)
		if !strings.Contains(got, "@") {
			t.Fatalf("injectLatest(%q) = %q, expected `@` injected", id, got)
		}
	})
}

// TestProperty_InjectLatest_Idempotent asserts injectLatest is
// idempotent: injecting into an already-pinned ID must preserve
// the existing pin (not double-append).
func TestProperty_InjectLatest_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		id := rapid.String().Draw(t, "resourceID")
		once := injectLatest(id)
		twice := injectLatest(once)
		if once != twice {
			t.Fatalf("non-idempotent: injectLatest(%q)=%q; injectLatest(%q)=%q",
				id, once, once, twice)
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

// TestProperty_PackageArgs_DropsFlags asserts packageArgs drops
// anything starting with `-`.
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
