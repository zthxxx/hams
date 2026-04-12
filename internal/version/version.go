// Package version holds build metadata injected via ldflags at compile time.
package version

import (
	"fmt"
	"runtime"
)

// Build-time variables injected via -ldflags.
// Example: go build -ldflags "-X github.com/zthxxx/hams/internal/version.version=1.0.0"
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
