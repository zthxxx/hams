// Package tui implements the BubbleTea terminal user interface with alternate screen and interactive popups.
package tui

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"
)

// IsInteractive returns true if stdout is connected to a terminal.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) //nolint:gosec // Fd() returns uintptr which safely fits in int on all supported platforms
}

// Renderer handles output formatting based on TTY availability.
type Renderer struct {
	Interactive bool
	NoColor     bool
}

// NewRenderer creates a renderer based on the current terminal state.
func NewRenderer(noColor bool) *Renderer {
	return &Renderer{
		Interactive: IsInteractive(),
		NoColor:     noColor,
	}
}

// PrintProgress shows a progress line. In interactive mode, uses ANSI.
// In non-interactive mode, uses plain text.
func (r *Renderer) PrintProgress(current, total int, message string) {
	if r.Interactive && !r.NoColor {
		fmt.Printf("\r\033[K[%d/%d] %s", current, total, message)
	} else {
		fmt.Printf("[%d/%d] %s\n", current, total, message)
	}
}

// PrintStatus shows a status message with optional color.
func (r *Renderer) PrintStatus(status, message string) {
	if r.Interactive && !r.NoColor {
		switch status {
		case "ok":
			fmt.Printf("\033[32m✓\033[0m %s\n", message)
		case "fail":
			fmt.Printf("\033[31m✗\033[0m %s\n", message)
		case "skip":
			fmt.Printf("\033[33m-\033[0m %s\n", message)
		default:
			fmt.Printf("  %s\n", message)
		}
	} else {
		fmt.Printf("[%s] %s\n", status, message)
	}
}

// PrintHeader shows a section header.
func (r *Renderer) PrintHeader(title string) {
	if r.Interactive && !r.NoColor {
		fmt.Printf("\n\033[1m%s\033[0m\n", title)
		fmt.Println(strings.Repeat("─", len(title)))
	} else {
		fmt.Printf("\n=== %s ===\n", title)
	}
}

// PrintLogPath shows the log file path in the sticky top area.
func (r *Renderer) PrintLogPath(logPath string) {
	if r.Interactive && !r.NoColor {
		fmt.Printf("\033[2m📋 %s\033[0m\n", logPath)
	} else {
		fmt.Printf("Log: %s\n", logPath)
	}
}

// WarnNonInteractive logs a warning when interactive features are requested in non-TTY mode.
func WarnNonInteractive(feature string) {
	slog.Warn("interactive feature unavailable in non-TTY mode", "feature", feature)
	fmt.Fprintf(os.Stderr, "Warning: %s requires an interactive terminal\n", feature)
}
