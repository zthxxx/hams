package provider

import (
	"errors"
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
