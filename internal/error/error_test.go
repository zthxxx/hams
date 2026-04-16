package hamserr

import (
	"errors"
	"testing"
)

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

// TestErrorCodeFromExit_AllBranches covers every documented exit
// code's mapping to a machine-readable ErrorCode (consumed by AI
// agents per the cli-architecture spec). The provider-error range
// (11..125) gets a single representative.
func TestErrorCodeFromExit_AllBranches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		exit int
		want ErrorCode
	}{
		{ExitGeneralError, CodeGeneralError},
		{ExitUsageError, CodeUsageError},
		{ExitLockError, CodeLockConflict},
		{ExitPartialFailure, CodePartialFailure},
		{ExitSudoError, CodeSudoError},
		{ExitNotFound, CodeNotFound},
		{ExitNotExecutable, CodeNotExecutable},
		{ExitProviderBase, CodeProviderError},      // boundary low (11)
		{ExitProviderBase + 50, CodeProviderError}, // boundary mid (61)
		{ExitNotFound - 1, CodeProviderError},      // boundary high (125 — last in range)
		{0, CodeGeneralError},                      // unknown → fallback
		{5, CodeGeneralError},                      // gap (5..10 minus 10) → fallback
		{255, CodeGeneralError},                    // post-127 → fallback
	}
	for _, tc := range cases {
		got := errorCodeFromExit(tc.exit)
		if got != tc.want {
			t.Errorf("errorCodeFromExit(%d) = %q, want %q", tc.exit, got, tc.want)
		}
	}
}

// TestNewUserErrorWithCode covers the explicit-error-code constructor
// path that the auto-derived constructor does not exercise.
func TestNewUserErrorWithCode(t *testing.T) {
	t.Parallel()
	err := NewUserErrorWithCode(ExitGeneralError, CodePackageNotFound,
		"package nginx not found", "try `apt search nginx`")
	if err.Code != ExitGeneralError {
		t.Errorf("Code = %d, want %d", err.Code, ExitGeneralError)
	}
	if err.ErrorCode != CodePackageNotFound {
		t.Errorf("ErrorCode = %q, want %q", err.ErrorCode, CodePackageNotFound)
	}
	if err.Message != "package nginx not found" {
		t.Errorf("Message = %q", err.Message)
	}
	if len(err.Suggestions) != 1 || err.Suggestions[0] != "try `apt search nginx`" {
		t.Errorf("Suggestions = %v", err.Suggestions)
	}
}

// TestNewUserError_AutoDerivesErrorCodeFromExit asserts the
// auto-derivation path in NewUserError populates ErrorCode based on
// the exit code, even when no ErrorCode is explicitly passed.
func TestNewUserError_AutoDerivesErrorCodeFromExit(t *testing.T) {
	t.Parallel()
	// ExitLockError → CodeLockConflict
	err := NewUserError(ExitLockError, "another hams apply is running")
	if err.ErrorCode != CodeLockConflict {
		t.Errorf("auto-derived ErrorCode = %q, want %q", err.ErrorCode, CodeLockConflict)
	}
}

// TestUserFacingError_AsTargetType asserts UserFacingError satisfies
// the standard error interface and is recoverable via errors.As.
func TestUserFacingError_AsTargetType(t *testing.T) {
	t.Parallel()
	wrapped := NewUserError(ExitUsageError, "bad flag")
	var ufe *UserFacingError
	if !errorsAs(wrapped, &ufe) {
		t.Fatalf("expected errors.As to recover *UserFacingError, got false")
	}
	if ufe.Code != ExitUsageError {
		t.Errorf("recovered Code = %d, want %d", ufe.Code, ExitUsageError)
	}
}

// errorsAs delegates to stdlib errors.As; lifted into a helper for
// readability and to satisfy errorlint (which forbids raw type
// assertions on errors).
func errorsAs(err error, target **UserFacingError) bool {
	return errors.As(err, target)
}
