// Package logging provides structured slog setup with file rotation and session log management.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Setup initializes the global slog logger with file and optional console output.
// Creates the monthly log directory and log file if they don't exist.
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

	handler := slog.NewTextHandler(logFile, &slog.HandlerOptions{
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
func TildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
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
