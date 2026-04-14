package hamserr

import (
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
