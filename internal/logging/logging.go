// Package logging provides structured slog setup with file rotation and session log management.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SetupDebugOnly is a lightweight slog initializer for short commands
// (per-provider CLI dispatch, `hams config get`, `hams version`, etc.)
// that don't justify a per-invocation log file. Routes slog to stderr
// only and sets the level based on the --debug flag. No disk write,
// no monthly rotation — just makes `--debug` produce visible output
// for debug-level slog calls (e.g. wrap.go's "executing wrapped
// command" line that's useful when debugging `hams cargo install foo
// --debug`).
//
// Pre-cycle-241 only Setup (full logging with file rotation) honored
// --debug. Short commands parsed --debug into flags.Debug but never
// applied it, so the user got no extra output despite asking for it.
//
// Cycle 241.
func SetupDebugOnly(debug bool) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

// Setup initializes the global slog logger with file AND stderr output.
// Creates the monthly log directory and log file if they don't exist.
// Writing to both destinations means:
//   - Live stderr feedback for users watching the command run.
//   - Persisted file log for later inspection (spec'd path per
//     tui-logging/spec.md §"Sticky header shows log file path").
//
// Before this change, Setup redirected slog to the file ONLY — which
// made `hams apply` appear to hang from the user's perspective
// (no live log lines), even though the command was doing real work
// and writing to the file in the background.
func Setup(dataHome string, debug bool) (*os.File, error) {
	now := time.Now()
	monthDir := filepath.Join(dataHome, now.Format("2006-01"))
	if err := os.MkdirAll(monthDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating log directory %s: %w", monthDir, err)
	}

	logPath := filepath.Join(monthDir, fmt.Sprintf("hams.%s.log", now.Format("200601")))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640) //nolint:gosec // log file path is derived from XDG data directory
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", logPath, err)
	}

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	// MultiWriter fans out every slog call to both sinks. No-color /
	// structured parity: the TextHandler emits the same format on
	// both, so `grep ERROR` works on the file and the terminal output
	// stays in sync with what was persisted.
	sink := io.MultiWriter(os.Stderr, logFile)
	handler := slog.NewTextHandler(sink, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))

	return logFile, nil
}

// LogPath returns the current month's log file path.
func LogPath(dataHome string) string {
	now := time.Now()
	monthDir := filepath.Join(dataHome, now.Format("2006-01"))
	return filepath.Join(monthDir, fmt.Sprintf("hams.%s.log", now.Format("200601")))
}

// TildePath replaces the user's home directory with ~/  in a path.
// Only matches when the path equals home exactly OR continues with a
// path separator. Without this guard, `home=/home/alice`,
// `path=/home/alice2/foo` would naively prefix-match and return the
// bogus `~2/foo` instead of leaving the path untouched. Real risk on
// systems where multiple users share a parent (e.g. `/home/alice` and
// `/home/alice2` exist side by side).
func TildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

// SessionLogPath returns the path for a third-party tool session log.
func SessionLogPath(dataHome, providerName string) string {
	now := time.Now()
	monthDir := filepath.Join(dataHome, now.Format("2006-01"), "provider")
	return filepath.Join(monthDir, fmt.Sprintf("%s.%s.session.log", providerName, now.Format("20060102T150405")))
}

// CreateSessionLog creates a session log file for a provider, ensuring the directory exists.
func CreateSessionLog(dataHome, providerName string) (*os.File, string, error) {
	logPath := SessionLogPath(dataHome, providerName)
	dir := filepath.Dir(logPath)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, "", fmt.Errorf("creating session log directory: %w", err)
	}

	f, err := os.Create(logPath) //nolint:gosec // session log path is derived from XDG data directory
	if err != nil {
		return nil, "", fmt.Errorf("creating session log %s: %w", logPath, err)
	}

	return f, logPath, nil
}
