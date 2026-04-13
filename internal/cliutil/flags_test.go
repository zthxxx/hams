package cliutil

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestSplitHamsFlags_Empty(t *testing.T) {
	t.Parallel()
	hams, pass := SplitHamsFlags(nil)
	if len(hams) != 0 {
		t.Errorf("expected empty hams flags, got %v", hams)
	}
	if len(pass) != 0 {
		t.Errorf("expected empty passthrough, got %v", pass)
	}
}

func TestSplitHamsFlags_NoHamsFlags(t *testing.T) {
	t.Parallel()
	hams, pass := SplitHamsFlags([]string{"install", "htop", "--cask"})
	if len(hams) != 0 {
		t.Errorf("expected no hams flags, got %v", hams)
	}
	if len(pass) != 3 {
		t.Errorf("expected 3 passthrough args, got %d: %v", len(pass), pass)
	}
}

func TestSplitHamsFlags_KeyValue(t *testing.T) {
	t.Parallel()
	hams, pass := SplitHamsFlags([]string{"install", "htop", "--hams:tag=devtools"})
	if hams["tag"] != "devtools" {
		t.Errorf("hams[tag] = %q, want devtools", hams["tag"])
	}
	if len(pass) != 2 {
		t.Errorf("expected 2 passthrough args, got %v", pass)
	}
}

func TestSplitHamsFlags_BooleanFlags(t *testing.T) {
	t.Parallel()
	hams, _ := SplitHamsFlags([]string{"--hams:lucky", "--hams:local"})
	if _, ok := hams["lucky"]; !ok {
		t.Error("hams[lucky] should exist")
	}
	if _, ok := hams["local"]; !ok {
		t.Error("hams[local] should exist")
	}
}

func TestSplitHamsFlags_ForceForward(t *testing.T) {
	t.Parallel()
	hams, pass := SplitHamsFlags([]string{"install", "--", "--hams:tag=foo", "--cask"})
	if len(hams) != 0 {
		t.Errorf("hams flags should be empty after --, got %v", hams)
	}
	if len(pass) != 3 {
		t.Errorf("expected 3 passthrough args, got %v", pass)
	}
	if pass[0] != "install" || pass[1] != "--hams:tag=foo" || pass[2] != "--cask" {
		t.Errorf("passthrough = %v", pass)
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
			case 0: // boolean hams flag
				key := rapid.StringMatching(`[a-z]{1,5}`).Draw(t, "boolKey")
				args = append(args, "--hams:"+key)
			case 1: // key=value hams flag
				key := rapid.StringMatching(`[a-z]{1,5}`).Draw(t, "kvKey")
				val := rapid.StringMatching(`[a-z0-9]{1,5}`).Draw(t, "kvVal")
				args = append(args, "--hams:"+key+"="+val)
			case 2: // separator
				args = append(args, "--")
			default: // ordinary arg (may include dashes)
				word := rapid.StringMatching(`-{0,2}[a-z]{1,8}`).Draw(t, "word")
				args = append(args, word)
			}
		}

		hams, pass := SplitHamsFlags(args)

		// Find the index of the first "--" separator in the original args.
		firstSep := -1
		for i, a := range args {
			if a == "--" {
				firstSep = i
				break
			}
		}

		// Invariant 1: hams flags before the separator must all be captured.
		beforeSep := args
		if firstSep >= 0 {
			beforeSep = args[:firstSep]
		}
		for _, a := range beforeSep {
			if strings.HasPrefix(a, "--hams:") {
				key, _ := parseFlag(a[7:])
				if _, ok := hams[key]; !ok {
					t.Errorf("hams flag %q before separator not captured in hamsFlags", a)
				}
			}
		}

		// Invariant 2: in the passthrough prefix (before the separator's tail),
		// no --hams: prefixed arg may appear. After the separator, they are allowed.
		prefixLen := len(pass)
		if firstSep >= 0 {
			afterSep := args[firstSep+1:]
			prefixLen = len(pass) - len(afterSep)
		}
		for _, p := range pass[:prefixLen] {
			if strings.HasPrefix(p, "--hams:") {
				t.Errorf("hams flag %q leaked into passthrough prefix (before separator tail)", p)
			}
		}

		// Invariant 3: after the first separator, everything is preserved
		// byte-for-byte in passthrough (including --hams: flags and extra --).
		if firstSep >= 0 {
			afterSep := args[firstSep+1:]
			// passthrough tail must equal afterSep exactly.
			tailStart := len(pass) - len(afterSep)
			if tailStart < 0 {
				t.Fatalf("passthrough (%d) shorter than after-separator args (%d)", len(pass), len(afterSep))
			}
			tail := pass[tailStart:]
			for i, a := range afterSep {
				if tail[i] != a {
					t.Errorf("after separator: pass[%d]=%q, want %q", tailStart+i, tail[i], a)
				}
			}
		}

		// Invariant 4: the first separator is consumed (not in passthrough prefix).
		// Subsequent -- after the first separator are ordinary args and allowed.
		for _, p := range pass[:prefixLen] {
			if p == "--" {
				t.Error("bare -- separator leaked into passthrough prefix")
			}
		}

		// Invariant 5: non-hams args before the separator are in passthrough prefix.
		var expectedPrefix []string
		for _, a := range beforeSep {
			if !strings.HasPrefix(a, "--hams:") {
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

func TestNewUserError(t *testing.T) {
	t.Parallel()
	err := NewUserError(ExitUsageError, "bad input", "try --help")
	if err.Code != ExitUsageError {
		t.Errorf("Code = %d, want %d", err.Code, ExitUsageError)
	}
	if err.Message != "bad input" {
		t.Errorf("Message = %q, want %q", err.Message, "bad input")
	}
	if len(err.Suggestions) != 1 || err.Suggestions[0] != "try --help" {
		t.Errorf("Suggestions = %v", err.Suggestions)
	}
	if err.Error() != "bad input" {
		t.Errorf("Error() = %q, want %q", err.Error(), "bad input")
	}
}

func TestNewUserError_NoSuggestions(t *testing.T) {
	t.Parallel()
	err := NewUserError(ExitGeneralError, "something broke")
	if len(err.Suggestions) != 0 {
		t.Errorf("expected no suggestions, got %v", err.Suggestions)
	}
}

func TestExitCodeConstants(t *testing.T) {
	t.Parallel()
	// Verify exit codes match the spec.
	if ExitSuccess != 0 {
		t.Error("ExitSuccess should be 0")
	}
	if ExitGeneralError != 1 {
		t.Error("ExitGeneralError should be 1")
	}
	if ExitUsageError != 2 {
		t.Error("ExitUsageError should be 2")
	}
	if ExitLockError != 3 {
		t.Error("ExitLockError should be 3")
	}
	if ExitPartialFailure != 4 {
		t.Error("ExitPartialFailure should be 4")
	}
	if ExitSudoError != 10 {
		t.Error("ExitSudoError should be 10")
	}
	if ExitProviderBase != 11 {
		t.Error("ExitProviderBase should be 11")
	}
	if ExitNotFound != 126 {
		t.Error("ExitNotFound should be 126")
	}
	if ExitNotExecutable != 127 {
		t.Error("ExitNotExecutable should be 127")
	}
}
