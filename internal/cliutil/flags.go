// Package cliutil provides shared CLI types used across internal packages to avoid import cycles.
package cliutil

import "strings"

// GlobalFlags holds flags that appear between `hams` and the provider name.
type GlobalFlags struct {
	Debug   bool
	DryRun  bool
	JSON    bool
	NoColor bool
	Config  string
	Store   string
	Profile string
}

// SplitHamsFlags separates --hams: prefixed flags from passthrough args.
// Also handles the -- separator: everything after -- goes to passthrough.
func SplitHamsFlags(args []string) (hamsFlags map[string]string, passthrough []string) {
	hamsFlags = make(map[string]string)
	forceForward := false

	for _, arg := range args {
		if forceForward {
			passthrough = append(passthrough, arg)
			continue
		}

		if arg == "--" {
			forceForward = true
			continue
		}

		if strings.HasPrefix(arg, "--hams:") {
			key, value := parseFlag(arg[7:]) // strip "--hams:" prefix (7 chars)
			hamsFlags[key] = value
			continue
		}

		passthrough = append(passthrough, arg)
	}

	return hamsFlags, passthrough
}

func parseFlag(s string) (key, value string) {
	if k, v, ok := strings.Cut(s, "="); ok {
		return k, v
	}
	return s, ""
}

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
