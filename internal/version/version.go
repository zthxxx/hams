// Package version holds build metadata injected via ldflags at compile time.
package version

import (
	"fmt"
	"runtime"
)

// Build-time variables injected via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// Info returns a formatted version string including build metadata.
func Info() string {
	return fmt.Sprintf("hams %s (%s) built %s %s/%s",
		version, commit, date, runtime.GOOS, runtime.GOARCH)
}

// Brief returns the short form used by `hams --version`: "<version> (<commit>)".
// Dev builds: "dev (a6f4218)" when ldflags inject commit, "dev (unknown)" otherwise.
// Release builds: "v1.2.4 (a6f4218)".
func Brief() string {
	return fmt.Sprintf("%s (%s)", version, commit)
}

// Version returns the semantic version string.
func Version() string {
	return version
}

// Commit returns the git commit hash used for the build.
func Commit() string {
	return commit
}

// Date returns the build date string.
func Date() string {
	return date
}
