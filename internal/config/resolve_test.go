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
// CLI override > config field > DefaultProfileTag. Property-based on
// arbitrary non-empty CLI values + config values so every overlap
// combination shows up; asserts the precedence invariant holds.
func TestResolveActiveTag_Precedence(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
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

		var want string
		switch {
		case cliTag != "":
			want = cliTag
		case cliProfile != "":
			want = cliProfile
		case cfgTag != "":
			want = cfgTag
		default:
			want = DefaultProfileTag
		}
		if got != want {
			t.Errorf("ResolveActiveTag(cfg=%q, cliTag=%q, cliProfile=%q) = %q, want %q",
				cfgTag, cliTag, cliProfile, got, want)
		}
	})
}

// TestDeriveMachineID_EnvWins asserts that HAMS_MACHINE_ID, when set,
// overrides the hostname lookup. Uses the HostnameLookup seam to force
// a deterministic hostname without affecting the host.
func TestDeriveMachineID_EnvWins(t *testing.T) {
	origHost := HostnameLookup
	t.Cleanup(func() { HostnameLookup = origHost })

	HostnameLookup = func() (string, error) { return "hostbox", nil }
	t.Setenv("HAMS_MACHINE_ID", "env-override")

	got := DeriveMachineID()
	if got != "env-override" {
		t.Errorf("DeriveMachineID() = %q, want env-override", got)
	}
}

// TestDeriveMachineID_HostFallback exercises the "no env → use hostname"
// branch and checks the sanitizer preserves a non-empty result. Env
// setup happens once (testing.T-scoped) so rapid.Check can focus on
// hostname shape.
func TestDeriveMachineID_HostFallback(t *testing.T) {
	origHost := HostnameLookup
	t.Cleanup(func() { HostnameLookup = origHost })
	t.Setenv("HAMS_MACHINE_ID", "")

	rapid.Check(t, func(rt *rapid.T) {
		host := rapid.StringMatching(`[a-zA-Z0-9._-]{1,32}`).Draw(rt, "hostname")

		HostnameLookup = func() (string, error) { return host, nil }

		got := DeriveMachineID()
		// Invariant: result is non-empty — either the sanitized
		// hostname or the DefaultProfileTag fallback.
		if got == "" {
			rt.Errorf("DeriveMachineID() returned empty string for host=%q", host)
		}
	})
}

// TestDeriveMachineID_HostnameError falls back to DefaultProfileTag
// when os.Hostname returns an error (or an empty string).
func TestDeriveMachineID_HostnameError(t *testing.T) {
	origHost := HostnameLookup
	t.Cleanup(func() { HostnameLookup = origHost })

	HostnameLookup = func() (string, error) { return "", errors.New("no hostname") }
	t.Setenv("HAMS_MACHINE_ID", "")

	got := DeriveMachineID()
	if got != DefaultProfileTag {
		t.Errorf("DeriveMachineID() with hostname error = %q, want %q", got, DefaultProfileTag)
	}
}
