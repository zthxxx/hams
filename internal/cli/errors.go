package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Exit codes per cli-architecture spec.
const (
	ExitSuccess        = 0  // Operation completed successfully.
	ExitGeneralError   = 1  // General error.
	ExitUsageError     = 2  // Invalid usage (bad flags, missing args).
	ExitLockError      = 3  // Could not acquire lock (another hams is running).
	ExitPartialFailure = 4  // Some resources succeeded, some failed.
	ExitSudoError      = 10 // Sudo credential acquisition failed.
	ExitProviderBase   = 11 // Provider-specific errors: 11-19.
	ExitNotFound       = 126
	ExitNotExecutable  = 127
)

// UserFacingError is a structured error for CLI output.
// It provides a machine-readable code, human-readable message,
// and actionable suggestions for the user or AI agent.
type UserFacingError struct {
	Code        int      `json:"code"`
	Message     string   `json:"message"`
	Suggestions []string `json:"suggestions,omitempty"`
}

func (e *UserFacingError) Error() string {
	return e.Message
}

// PrintError outputs a UserFacingError in the appropriate format.
// In JSON mode, outputs a structured JSON object.
// In text mode, outputs human-readable error with suggestions.
func PrintError(err error, jsonMode bool) {
	var ufe *UserFacingError
	if !errors.As(err, &ufe) {
		ufe = &UserFacingError{
			Code:    ExitGeneralError,
			Message: err.Error(),
		}
	}

	if jsonMode {
		data, jsonErr := json.MarshalIndent(ufe, "", "  ")
		if jsonErr != nil {
			fmt.Fprintf(os.Stderr, `{"code":%d,"message":"%s"}`+"\n", ufe.Code, ufe.Message)
			return
		}
		fmt.Fprintln(os.Stderr, string(data))
		return
	}

	fmt.Fprintf(os.Stderr, "Error: %s\n", ufe.Message)
	for _, s := range ufe.Suggestions {
		fmt.Fprintf(os.Stderr, "  suggestion: %s\n", s)
	}
}

// NewUserError creates a UserFacingError with suggestions.
func NewUserError(code int, message string, suggestions ...string) *UserFacingError {
	return &UserFacingError{
		Code:        code,
		Message:     message,
		Suggestions: suggestions,
	}
}
