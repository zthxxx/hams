package provider

import (
	"errors"
	"runtime"
	"testing"
)

// TestResourceClass_String pins the human-readable strings emitted by
// the ResourceClass Stringer. The strings are used in log messages and
// diff output; a silent rename would garble them in user-facing logs.
func TestResourceClass_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		class ResourceClass
		want  string
	}{
		{ClassPackage, "package"},
		{ClassKVConfig, "kv-config"},
		{ClassCheckBased, "check-based"},
		{ClassFilesystem, "filesystem"},
		{ResourceClass(99), "ResourceClass(99)"},
	}
	for _, tc := range cases {
		if got := tc.class.String(); got != tc.want {
			t.Errorf("ResourceClass(%d).String() = %q, want %q", tc.class, got, tc.want)
		}
	}
}

// TestActionType_String pins the action strings shown in dry-run
// preview output and the `hams apply` per-action summary. Same
// rationale as TestResourceClass_String.
func TestActionType_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		action ActionType
		want   string
	}{
		{ActionInstall, "install"},
		{ActionUpdate, "update"},
		{ActionRemove, "remove"},
		{ActionSkip, "skip"},
		{ActionType(99), "ActionType(99)"},
	}
	for _, tc := range cases {
		if got := tc.action.String(); got != tc.want {
			t.Errorf("ActionType(%d).String() = %q, want %q", tc.action, got, tc.want)
		}
	}
}

// TestBootstrapRequiredError_Error asserts the user-facing message
// mentions the missing binary AND the remediation (--bootstrap). The
// CLI consent flow relies on this string being stable so test golden
// outputs and user-facing error expectations stay aligned.
func TestBootstrapRequiredError_Error(t *testing.T) {
	t.Parallel()
	err := &BootstrapRequiredError{Provider: "duti", Binary: "duti", Script: "brew install duti"}
	msg := err.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	// The message must surface the missing binary name and the
	// remediation flag; the consent flow and docs rely on these two
	// signals.
	for _, substr := range []string{"duti", "--bootstrap"} {
		if !contains(msg, substr) {
			t.Errorf("Error() = %q, want substring %q", msg, substr)
		}
	}
}

// TestBootstrapRequiredError_UnwrapsToSentinel asserts
// errors.Is(err, ErrBootstrapRequired) returns true so the consent
// path can detect this class of error without type-asserting on the
// concrete type at every call site.
func TestBootstrapRequiredError_UnwrapsToSentinel(t *testing.T) {
	t.Parallel()
	err := &BootstrapRequiredError{Provider: "brew", Binary: "brew", Script: "curl | bash"}
	if !errors.Is(err, ErrBootstrapRequired) {
		t.Error("errors.Is(err, ErrBootstrapRequired) = false, want true")
	}
	// Also verify errors.As into the concrete type works — apply.go
	// uses As to pull out the Script/Binary fields for the prompt.
	var target *BootstrapRequiredError
	if !errors.As(err, &target) {
		t.Error("errors.As(err, *BootstrapRequiredError) = false, want true")
	}
	if target != nil && target.Binary != "brew" {
		t.Errorf("unwrapped target.Binary = %q, want brew", target.Binary)
	}
}

// contains is a small substring helper used by TestBootstrapRequiredError_Error.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestIsPlatformsMatch_EmptyMatchesAll pins the "no platform filter
// means compatible everywhere" contract. Providers with empty
// Platforms lists MUST dispatch on every OS — a change that breaks
// this would silently hide every such provider.
func TestIsPlatformsMatch_EmptyMatchesAll(t *testing.T) {
	t.Parallel()
	if !IsPlatformsMatch(nil) {
		t.Error("IsPlatformsMatch(nil) = false, want true (empty filter matches all)")
	}
	if !IsPlatformsMatch([]Platform{}) {
		t.Error("IsPlatformsMatch([]) = false, want true (empty filter matches all)")
	}
}

// TestIsPlatformsMatch_PlatformAllWildcard asserts the PlatformAll
// sentinel always matches regardless of runtime.GOOS.
func TestIsPlatformsMatch_PlatformAllWildcard(t *testing.T) {
	t.Parallel()
	if !IsPlatformsMatch([]Platform{PlatformAll}) {
		t.Error("PlatformAll should match any GOOS")
	}
}

// TestIsPlatformsMatch_EmptyStringTreatedAsWildcard asserts that a
// literal empty-string Platform entry is treated identically to
// PlatformAll. This matches the YAML-loader's default behavior for
// unset `if:` fields.
func TestIsPlatformsMatch_EmptyStringTreatedAsWildcard(t *testing.T) {
	t.Parallel()
	if !IsPlatformsMatch([]Platform{Platform("")}) {
		t.Error(`IsPlatformsMatch([""]) = false, want true (empty-string treated as wildcard)`)
	}
}

// TestIsPlatformsMatch_NonMatchingPlatformFalse asserts a platform
// filter that names only OSes the current runtime isn't returns
// false. Using a clearly-bogus platform name avoids runtime.GOOS
// dependence in the test.
func TestIsPlatformsMatch_NonMatchingPlatformFalse(t *testing.T) {
	t.Parallel()
	if IsPlatformsMatch([]Platform{Platform("plan9")}) {
		t.Error("IsPlatformsMatch([plan9]) should be false on this runtime")
	}
}

// TestHasAny_Nil and the other three TestHasAny_* tests cover each
// branch of HookSet.HasAny — previously at 66.7% coverage with no
// direct tests (indirectly exercised through other hooks tests).
func TestHasAny_Nil(t *testing.T) {
	t.Parallel()
	var hs *HookSet
	if hs.HasAny() {
		t.Error("nil HookSet.HasAny() = true, want false")
	}
}

func TestHasAny_EmptyHookSet(t *testing.T) {
	t.Parallel()
	hs := &HookSet{}
	if hs.HasAny() {
		t.Error("empty HookSet.HasAny() = true, want false")
	}
}

func TestHasAny_PreInstallOnly(t *testing.T) {
	t.Parallel()
	hs := &HookSet{PreInstall: []Hook{{Command: "echo pre"}}}
	if !hs.HasAny() {
		t.Error("HookSet with PreInstall.HasAny() = false, want true")
	}
}

func TestHasAny_PostUpdateOnly(t *testing.T) {
	t.Parallel()
	hs := &HookSet{PostUpdate: []Hook{{Command: "echo post"}}}
	if !hs.HasAny() {
		t.Error("HookSet with PostUpdate.HasAny() = false, want true")
	}
}

// TestMatchesPlatform_Wildcards asserts the two wildcard cases
// (empty string and PlatformAll) both return true regardless of
// runtime.GOOS. These power the `DependOn` spec semantic: "dep
// applies to all platforms". A regression that inverts this would
// silently drop every unfiltered dep from bootstrap.
func TestMatchesPlatform_Wildcards(t *testing.T) {
	t.Parallel()
	if !matchesPlatform(Platform("")) {
		t.Error(`matchesPlatform("") = false, want true (empty = all)`)
	}
	if !matchesPlatform(PlatformAll) {
		t.Error(`matchesPlatform(PlatformAll) = false, want true`)
	}
}

// TestMatchesPlatform_CurrentGOOS asserts the runtime.GOOS match
// branch. Uses the actual runtime.GOOS so the test passes on any
// supported host.
func TestMatchesPlatform_CurrentGOOS(t *testing.T) {
	t.Parallel()
	if !matchesPlatform(Platform(runtime.GOOS)) {
		t.Errorf("matchesPlatform(%q) = false, want true (current GOOS)", runtime.GOOS)
	}
}

// TestMatchesPlatform_OtherGOOSFalse asserts a platform string that
// can never match the current host (a bogus name) returns false.
// Guards the fall-through branch that power the "skip deps for
// wrong platform" path in ResolveDAG + RunBootstrap.
func TestMatchesPlatform_OtherGOOSFalse(t *testing.T) {
	t.Parallel()
	if matchesPlatform(Platform("plan9-inferno")) {
		t.Error(`matchesPlatform("plan9-inferno") = true, want false (bogus GOOS)`)
	}
}
