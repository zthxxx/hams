package config

import (
	"errors"
	"testing"

	"pgregory.net/rapid"

	hamserr "github.com/zthxxx/hams/internal/error"
)

// TestResolveCLITagOverride_ReturnsSingleValue asserts that when only
// one of --tag / --profile is supplied, its value is returned
// unchanged. Covers the base case every caller depends on.
func TestResolveCLITagOverride_ReturnsSingleValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		cliTag     string
		cliProfile string
		want       string
	}{
		{"tag only", "macOS", "", "macOS"},
		{"profile only", "", "linux", "linux"},
		{"neither", "", "", ""},
		{"same values", "dev", "dev", "dev"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveCLITagOverride(c.cliTag, c.cliProfile)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("ResolveCLITagOverride(%q, %q) = %q, want %q",
					c.cliTag, c.cliProfile, got, c.want)
			}
		})
	}
}

// TestResolveCLITagOverride_ConflictIsUsageError covers the disagree
// branch: both flags supplied with different values — loud rejection.
// Regression gate for the "user mixed --profile=X --tag=Y in a script"
// scenario where a silent pick-one-and-run would corrupt the profile
// directory selection on a fresh machine.
func TestResolveCLITagOverride_ConflictIsUsageError(t *testing.T) {
	t.Parallel()
	_, err := ResolveCLITagOverride("macOS", "linux")
	if err == nil {
		t.Fatal("expected usage error when --tag and --profile disagree")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T: %v", err, err)
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("Code = %d, want %d", ufe.Code, hamserr.ExitUsageError)
	}
}

// TestResolveActiveTag_Precedence exercises the full precedence chain:
// CLI override > config field > "default". Property-based on arbitrary
// nonempty CLI values + config values so any overlap combination
// shows up; asserts the precedence invariant holds.
func TestResolveActiveTag_Precedence(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		// Short ASCII-letter strings — enough to drive precedence
		// without tripping sanitizePathSegment.
		genTag := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9_-]{0,15}`)
		maybeTag := rapid.OneOf(
			rapid.Just(""),
			genTag,
		)

		cliTag := maybeTag.Draw(t, "cliTag")
		cliProfile := maybeTag.Draw(t, "cliProfile")
		cfgTag := maybeTag.Draw(t, "cfgTag")

		// Skip the "conflicting CLI flags" branch — it's covered by
		// a dedicated test; mixing the precedence test with the
		// error branch blurs the signal.
		if cliTag != "" && cliProfile != "" && cliTag != cliProfile {
			return
		}

		cfg := &Config{ProfileTag: cfgTag}
		got, err := ResolveActiveTag(cfg, cliTag, cliProfile)
		if err != nil {
			t.Fatalf("ResolveActiveTag: %v", err)
		}

		// Independently compute the expected winner.
		var want string
		switch {
		case cliTag != "":
			want = cliTag
		case cliProfile != "":
			want = cliProfile
		case cfgTag != "":
			want = cfgTag
		default:
			want = defaultProfileTag
		}

		if got != want {
			t.Errorf("ResolveActiveTag(cfg={%q}, tag=%q, profile=%q) = %q, want %q",
				cfgTag, cliTag, cliProfile, got, want)
		}
	})
}

// TestResolveActiveTag_NilConfigFallsBackToDefault asserts that the
// pre-config-load path (which calls with cfg=nil) is safe and yields
// "default" when no CLI flag is supplied. Mirrors the bootstrap path
// where the config is not yet loaded.
func TestResolveActiveTag_NilConfigFallsBackToDefault(t *testing.T) {
	t.Parallel()
	got, err := ResolveActiveTag(nil, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != defaultProfileTag {
		t.Errorf("ResolveActiveTag(nil, \"\", \"\") = %q, want %q", got, defaultProfileTag)
	}
}

// TestDeriveMachineID_UsesHostnameLookupSeam asserts the DI seam is
// honored so tests can return deterministic values.
func TestDeriveMachineID_UsesHostnameLookupSeam(t *testing.T) {
	orig := HostnameLookup
	defer func() { HostnameLookup = orig }()

	// Clear env to avoid interference from $HAMS_MACHINE_ID in CI.
	t.Setenv("HAMS_MACHINE_ID", "")

	HostnameLookup = func() (string, error) { return "testbox", nil }
	if got := DeriveMachineID(); got != "testbox" {
		t.Errorf("DeriveMachineID = %q, want %q", got, "testbox")
	}
}

// TestDeriveMachineID_RejectsPathSeparators asserts that a hostname
// containing `/` or `\` is rejected (sanitizePathSegment returns the
// fallback). Prevents the `.state/<machine_id>/…` path escape when a
// hostname-like value ever contains a separator.
func TestDeriveMachineID_RejectsPathSeparators(t *testing.T) {
	orig := HostnameLookup
	defer func() { HostnameLookup = orig }()
	t.Setenv("HAMS_MACHINE_ID", "")

	HostnameLookup = func() (string, error) { return "bad/host", nil }
	if got := DeriveMachineID(); got != defaultProfileTag {
		t.Errorf("hostname with slash: DeriveMachineID = %q, want fallback %q",
			got, defaultProfileTag)
	}
}

// TestDeriveMachineID_EnvOverridesHostname locks in the precedence
// order: $HAMS_MACHINE_ID > os.Hostname > "default". Matches the
// spec scenarios under 2026-04-18-apply-tag-and-auto-init.
func TestDeriveMachineID_EnvOverridesHostname(t *testing.T) {
	orig := HostnameLookup
	defer func() { HostnameLookup = orig }()
	HostnameLookup = func() (string, error) { return "hostnameval", nil }

	t.Setenv("HAMS_MACHINE_ID", "envval")
	if got := DeriveMachineID(); got != "envval" {
		t.Errorf("DeriveMachineID with env set = %q, want %q", got, "envval")
	}
}

// TestDeriveMachineID_FallsBackToDefault covers the no-env, failing-
// hostname branch.
func TestDeriveMachineID_FallsBackToDefault(t *testing.T) {
	orig := HostnameLookup
	defer func() { HostnameLookup = orig }()
	HostnameLookup = func() (string, error) { return "", errors.New("simulated failure") }
	t.Setenv("HAMS_MACHINE_ID", "")

	if got := DeriveMachineID(); got != defaultProfileTag {
		t.Errorf("DeriveMachineID with no env + hostname error = %q, want %q",
			got, defaultProfileTag)
	}
}
