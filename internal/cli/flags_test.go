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

		// Invariant 1: all --hams- flags before the first -- are captured,
		// EXCEPT explicit-false-y values (cycle 162: `--hams-X=false`/`=0`
		// disables the flag, leaving the key absent from the map).
		beforeSep := args
		if firstSep >= 0 {
			beforeSep = args[:firstSep]
		}
		for _, a := range beforeSep {
			if strings.HasPrefix(a, hamsFlagPrefix) {
				key, value := parseHamsFlag(a[7:])
				if hamsFlagFalsey(value) {
					if _, ok := hams[key]; ok {
						t.Errorf("hams flag %q with false-y value should NOT be captured", a)
					}
					continue
				}
				if _, ok := hams[key]; !ok {
					t.Errorf("hams flag %q before separator not captured", a)
				}
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
