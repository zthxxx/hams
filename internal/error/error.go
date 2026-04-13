// Package hamserr provides shared CLI types (flags, exit codes, errors)
// used across internal packages to avoid import cycles between cli and provider packages.
package hamserr

// ExitCodes per cli-architecture spec.
const (
	ExitSuccess        = 0
	ExitGeneralError   = 1
	ExitUsageError     = 2
	ExitLockError      = 3
	ExitPartialFailure = 4
	ExitSudoError      = 10
	ExitProviderBase   = 11
	ExitNotFound       = 126
	ExitNotExecutable  = 127
)

// UserFacingError is a structured error for CLI output.
type UserFacingError struct {
	Code        int      `json:"code"`
	Message     string   `json:"message"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// Error implements the error interface.
func (e *UserFacingError) Error() string {
	return e.Message
}

// NewUserError creates a UserFacingError with suggestions.
func NewUserError(code int, message string, suggestions ...string) *UserFacingError {
	return &UserFacingError{
		Code:        code,
		Message:     message,
		Suggestions: suggestions,
	}
}
