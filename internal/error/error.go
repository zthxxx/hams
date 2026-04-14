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

// ErrorCode is a machine-readable error code string for AI-agent consumption.
type ErrorCode string

// Machine-readable error codes per cli-architecture spec.
const (
	CodeGeneralError    ErrorCode = "GENERAL_ERROR"
	CodeUsageError      ErrorCode = "USAGE_ERROR"
	CodeLockConflict    ErrorCode = "LOCK_CONFLICT"
	CodePartialFailure  ErrorCode = "PARTIAL_FAILURE"
	CodeSudoError       ErrorCode = "SUDO_ERROR"
	CodeProviderError   ErrorCode = "PROVIDER_ERROR"
	CodeNotFound        ErrorCode = "NOT_FOUND"
	CodeNotExecutable   ErrorCode = "NOT_EXECUTABLE"
	CodePackageNotFound ErrorCode = "PACKAGE_NOT_FOUND"
	CodeConfigError     ErrorCode = "CONFIG_ERROR"
)

// UserFacingError is a structured error for CLI output.
type UserFacingError struct {
	Code        int       `json:"code"`
	ErrorCode   ErrorCode `json:"error_code,omitempty"`
	Message     string    `json:"message"`
	Suggestions []string  `json:"suggestions,omitempty"`
}

// Error implements the error interface.
func (e *UserFacingError) Error() string {
	return e.Message
}

// NewUserError creates a UserFacingError with suggestions.
func NewUserError(code int, message string, suggestions ...string) *UserFacingError {
	return &UserFacingError{
		Code:        code,
		ErrorCode:   errorCodeFromExit(code),
		Message:     message,
		Suggestions: suggestions,
	}
}

// NewUserErrorWithCode creates a UserFacingError with an explicit error code.
func NewUserErrorWithCode(code int, errorCode ErrorCode, message string, suggestions ...string) *UserFacingError {
	return &UserFacingError{
		Code:        code,
		ErrorCode:   errorCode,
		Message:     message,
		Suggestions: suggestions,
	}
}

func errorCodeFromExit(code int) ErrorCode {
	switch code {
	case ExitGeneralError:
		return CodeGeneralError
	case ExitUsageError:
		return CodeUsageError
	case ExitLockError:
		return CodeLockConflict
	case ExitPartialFailure:
		return CodePartialFailure
	case ExitSudoError:
		return CodeSudoError
	case ExitNotFound:
		return CodeNotFound
	case ExitNotExecutable:
		return CodeNotExecutable
	default:
		if code >= ExitProviderBase && code < ExitNotFound {
			return CodeProviderError
		}
		return CodeGeneralError
	}
}
