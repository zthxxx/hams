package cli

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestSplitHamsFlags_Empty(t *testing.T) {
	t.Parallel()
	hams, pass := splitHamsFlags(nil)
	if len(hams) != 0 {
		t.Errorf("expected empty hams flags, got %v", hams)
	}
	if len(pass) != 0 {
		t.Errorf("expected empty passthrough, got %v", pass)
	}
}

func TestSplitHamsFlags_NoHamsFlags(t *testing.T) {
	t.Parallel()
	hams, pass := splitHamsFlags([]string{"install", "htop", "--cask"})
	if len(hams) != 0 {
		t.Errorf("expected no hams flags, got %v", hams)
	}
	if len(pass) != 3 {
		t.Errorf("expected 3 passthrough args, got %d: %v", len(pass), pass)
	}
}

func TestSplitHamsFlags_KeyValue(t *testing.T) {
	t.Parallel()
	hams, pass := splitHamsFlags([]string{"install", "htop", "--hams-tag=devtools"})
	if hams["tag"] != "devtools" {
		t.Errorf("hams[tag] = %q, want devtools", hams["tag"])
	}
	if len(pass) != 2 {
		t.Errorf("expected 2 passthrough args, got %v", pass)
	}
}

func TestSplitHamsFlags_BooleanFlags(t *testing.T) {
	t.Parallel()
	hams, _ := splitHamsFlags([]string{"--hams-lucky", "--hams-local"})
	if _, ok := hams["lucky"]; !ok {
		t.Error("hams[lucky] should exist")
	}
	if _, ok := hams["local"]; !ok {
		t.Error("hams[local] should exist")
	}
}

// TestSplitHamsFlags_ExplicitFalseDisablesFlag locks in cycle 162:
// `--hams-local=false` previously added the "local" key with value
// "false"; downstream presence-checks (`if _, ok := hamsFlags["local"]`)
// interpreted it as truthy — so `=false` did the opposite of what
// the user asked. Now: hamsFlagFalsey strips false-y values from
// the map entirely, so the presence-check correctly returns ok=false.
func TestSplitHamsFlags_ExplicitFalseDisablesFlag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		key  string
	}{
		{"local-false", []string{"--hams-local=false"}, "local"},
		{"local-zero", []string{"--hams-local=0"}, "local"},
		{"local-FALSE-uppercase", []string{"--hams-local=FALSE"}, "local"},
		{"lucky-false", []string{"--hams-lucky=false"}, "lucky"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hams, _ := splitHamsFlags(tc.args)
			if _, ok := hams[tc.key]; ok {
				t.Errorf("hams[%q] should NOT exist after =false; got value %q", tc.key, hams[tc.key])
			}
		})
	}
}

// TestSplitHamsFlags_ExplicitTrueKeepsFlag asserts the truthy-value
// branch still routes through correctly. `--hams-local=true` and
// `--hams-local=1` keep the key in the map.
func TestSplitHamsFlags_ExplicitTrueKeepsFlag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		key  string
	}{
		{"local-true", []string{"--hams-local=true"}, "local"},
		{"local-1", []string{"--hams-local=1"}, "local"},
		{"local-yes", []string{"--hams-local=yes"}, "local"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hams, _ := splitHamsFlags(tc.args)
			if _, ok := hams[tc.key]; !ok {
				t.Errorf("hams[%q] should exist after truthy value; got map %v", tc.key, hams)
			}
		})
	}
}

// TestSplitHamsFlags_LastOccurrenceWinsWhenFalsey locks in cycle 201:
// when a key appears more than once and the LAST occurrence is false-y,
// the key must NOT appear in the resulting map. Pre-cycle-201 the
// `hamsFlagFalsey` branch just did `continue`, leaving the map entry
// from an earlier bare (truthy) occurrence — so `--hams-local --hams-local=false`
// wrongly enabled `local`, flipping the user's last-stated intent.
// Rapid's property test caught this as: "last-occurrence of 'a' is false-y but
// key is in map".
func TestSplitHamsFlags_LastOccurrenceWinsWhenFalsey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		key  string
	}{
		{"bare-then-false", []string{"--hams-local", "--hams-local=false"}, "local"},
		{"bare-then-zero", []string{"--hams-local", "--hams-local=0"}, "local"},
		{"true-then-false", []string{"--hams-local=true", "--hams-local=false"}, "local"},
		{"many-bares-then-zero", []string{"--hams-a", "--hams-a", "--hams-a", "--hams-a", "--hams-a=0"}, "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hams, _ := splitHamsFlags(tc.args)
			if _, ok := hams[tc.key]; ok {
				t.Errorf("hams[%q] should NOT exist (last occurrence is false-y); got map %v", tc.key, hams)
			}
		})
	}
}

// TestSplitHamsFlags_LastOccurrenceWinsWhenTruthy is the symmetric
// invariant: if a falsey occurrence appears FIRST and a truthy one
// LAST, the key must be present with the truthy value — so
// `--hams-local=false --hams-local` correctly enables the flag.
func TestSplitHamsFlags_LastOccurrenceWinsWhenTruthy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		key  string
		want string
	}{
		{"false-then-bare", []string{"--hams-local=false", "--hams-local"}, "local", ""},
		{"zero-then-true", []string{"--hams-local=0", "--hams-local=true"}, "local", "true"},
		{"false-then-value", []string{"--hams-tag=false", "--hams-tag=devtools"}, "tag", "devtools"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hams, _ := splitHamsFlags(tc.args)
			got, ok := hams[tc.key]
			if !ok {
				t.Fatalf("hams[%q] should exist (last occurrence is truthy); got map %v", tc.key, hams)
			}
			if got != tc.want {
				t.Errorf("hams[%q] = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestSplitHamsFlags_ForceForward(t *testing.T) {
	t.Parallel()
	hams, pass := splitHamsFlags([]string{"install", "--", "--hams-tag=foo", "--cask"})
	if len(hams) != 0 {
		t.Errorf("hams flags should be empty after --, got %v", hams)
	}
	// The "--" separator must be preserved in passthrough for the underlying CLI.
	if len(pass) != 4 {
		t.Errorf("expected 4 passthrough args, got %v", pass)
	}
	if pass[0] != "install" || pass[1] != "--" || pass[2] != "--hams-tag=foo" || pass[3] != "--cask" {
		t.Errorf("passthrough = %v, want [install -- --hams-tag=foo --cask]", pass)
	}
}

func TestSplitHamsFlags_Property_PartitionInvariants(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		nArgs := rapid.IntRange(0, 12).Draw(t, "nArgs")
		args := make([]string, 0, nArgs)
		for range nArgs {
			choice := rapid.IntRange(0, 3).Draw(t, "choice")
			switch choice {
			case 0:
				key := rapid.StringMatching(`[a-z]{1,5}`).Draw(t, "boolKey")
				args = append(args, hamsFlagPrefix+key)
			case 1:
				key := rapid.StringMatching(`[a-z]{1,5}`).Draw(t, "kvKey")
				val := rapid.StringMatching(`[a-z0-9]{1,5}`).Draw(t, "kvVal")
				args = append(args, hamsFlagPrefix+key+"="+val)
			case 2:
				args = append(args, "--")
			default:
				word := rapid.StringMatching(`-{0,2}[a-z]{1,8}`).Draw(t, "word")
				args = append(args, word)
			}
		}

		hams, pass := splitHamsFlags(args)

		firstSep := -1
		for i, a := range args {
			if a == "--" {
				firstSep = i
				break
			}
		}

		// Invariant 1: for each KEY appearing in --hams- flags before the
		// first --, the LAST occurrence determines presence. Last-occurrence
		// false-y → absent; last-occurrence truthy → present (cycle 162's
		// strip-on-false interacts naturally with later truthy overrides
		// because the parser walks args in order). The pre-cycle-178 test
		// only checked per-arg invariants, which broke when the same key
		// appeared with both a false-y AND truthy value (last-wins).
		beforeSep := args
		if firstSep >= 0 {
			beforeSep = args[:firstSep]
		}
		// Build last-occurrence map: key → last (key, value) pair.
		lastByKey := make(map[string]struct {
			arg   string
			value string
		})
		for _, a := range beforeSep {
			if strings.HasPrefix(a, hamsFlagPrefix) {
				k, v := parseHamsFlag(a[7:])
				lastByKey[k] = struct {
					arg   string
					value string
				}{a, v}
			}
		}
		for k, last := range lastByKey {
			if hamsFlagFalsey(last.value) {
				if _, ok := hams[k]; ok {
					t.Errorf("last-occurrence of %q is false-y (%q) but key is in map", k, last.arg)
				}
				continue
			}
			if _, ok := hams[k]; !ok {
				t.Errorf("last-occurrence of %q is truthy (%q) but key is NOT in map", k, last.arg)
			}
		}

		// Invariants 2 & 3: separator preserved and tail verbatim.
		if firstSep >= 0 {
			checkSeparatorInvariants(t, args, beforeSep, pass, firstSep)
		}

		// Invariant 4: no --hams- flags in the prefix portion (before --).
		prefixEnd := len(pass)
		if firstSep >= 0 {
			// Find the -- in passthrough.
			for i, p := range pass {
				if p == "--" {
					prefixEnd = i
					break
				}
			}
		}
		for _, p := range pass[:prefixEnd] {
			if strings.HasPrefix(p, hamsFlagPrefix) {
				t.Errorf("hams flag %q leaked into passthrough prefix", p)
			}
		}

		// Invariant 5: non-hams args before -- appear in order.
		var expectedPrefix []string
		for _, a := range beforeSep {
			if !strings.HasPrefix(a, hamsFlagPrefix) {
				expectedPrefix = append(expectedPrefix, a)
			}
		}
		for i, want := range expectedPrefix {
			if i >= len(pass) || pass[i] != want {
				got := "<missing>"
				if i < len(pass) {
					got = pass[i]
				}
				t.Errorf("passthrough prefix[%d]=%q, want %q", i, got, want)
			}
		}
	})
}

// checkSeparatorInvariants asserts that "--" is preserved in passthrough at the right
// position, and that every arg after the first "--" in args is present verbatim in pass.
func checkSeparatorInvariants(t *rapid.T, args, beforeSep, pass []string, firstSep int) {
	t.Helper()
	afterSep := args[firstSep+1:]
	// passthrough = prefix (non-hams args before --) + "--" + afterSep
	expectedPrefixLen := 0
	for _, a := range beforeSep {
		if !strings.HasPrefix(a, hamsFlagPrefix) {
			expectedPrefixLen++
		}
	}
	sepIdx := expectedPrefixLen
	if sepIdx >= len(pass) || pass[sepIdx] != "--" {
		t.Errorf("expected -- at passthrough[%d], got %v", sepIdx, pass)
	}

	tailStart := sepIdx + 1
	if tailStart+len(afterSep) != len(pass) {
		t.Errorf("passthrough tail length mismatch: got %d, want %d", len(pass)-tailStart, len(afterSep))
		return
	}
	for i, a := range afterSep {
		if pass[tailStart+i] != a {
			t.Errorf("after separator: pass[%d]=%q, want %q", tailStart+i, pass[tailStart+i], a)
		}
	}
}
