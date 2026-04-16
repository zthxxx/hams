package logging

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetup_CreatesLogFile(t *testing.T) {
	dataHome := t.TempDir()
	logFile, err := Setup(dataHome, false)
	if err != nil {
		t.Fatalf("Setup error: %v", err)
	}
	defer logFile.Close() //nolint:errcheck // test cleanup

	// Log file should exist.
	now := time.Now()
	expectedPath := filepath.Join(dataHome, now.Format("2006-01"),
		"hams."+now.Format("200601")+".log")
	if _, statErr := os.Stat(expectedPath); os.IsNotExist(statErr) {
		t.Errorf("log file not created at %s", expectedPath)
	}
}

func TestLogPath(t *testing.T) {
	path := LogPath("/home/user/.local/share/hams")
	now := time.Now()
	expected := filepath.Join("/home/user/.local/share/hams", now.Format("2006-01"),
		"hams."+now.Format("200601")+".log")
	if path != expected {
		t.Errorf("LogPath = %q, want %q", path, expected)
	}
}

func TestTildePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		input string
		want  string
	}{
		{home + "/.local/share/hams/log", "~/.local/share/hams/log"},
		{"/tmp/other/path", "/tmp/other/path"},
		{home, "~"},
	}

	for _, tt := range tests {
		got := TildePath(tt.input)
		if got != tt.want {
			t.Errorf("TildePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSessionLogPath(t *testing.T) {
	path := SessionLogPath("/data/hams", "homebrew")
	now := time.Now()

	if !strings.Contains(path, "provider/homebrew.") {
		t.Errorf("SessionLogPath = %q, want to contain provider/homebrew.", path)
	}
	if !strings.Contains(path, now.Format("2006-01")) {
		t.Errorf("SessionLogPath = %q, want to contain month dir", path)
	}
	if !strings.HasSuffix(path, ".session.log") {
		t.Errorf("SessionLogPath = %q, want .session.log suffix", path)
	}
}

func TestCreateSessionLog(t *testing.T) {
	dataHome := t.TempDir()
	f, logPath, err := CreateSessionLog(dataHome, "pnpm")
	if err != nil {
		t.Fatalf("CreateSessionLog error: %v", err)
	}
	defer f.Close() //nolint:errcheck // test cleanup

	if logPath == "" {
		t.Error("logPath should not be empty")
	}
	if _, statErr := os.Stat(logPath); os.IsNotExist(statErr) {
		t.Errorf("session log not created at %s", logPath)
	}
}

// TestSetup_DualSink locks in cycle-67: after Setup, slog output
// must reach BOTH the file AND stderr. Before the MultiWriter fix,
// `hams apply` appeared to hang because slog lines went ONLY to
// the file, leaving the terminal silent.
func TestSetup_DualSink(t *testing.T) {
	// Redirect stderr to a pipe so we can capture it.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	dataHome := t.TempDir()
	logFile, err := Setup(dataHome, false)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer logFile.Close() //nolint:errcheck // test cleanup

	// Emit a slog line — should land in BOTH sinks.
	slog.Info("dual-sink probe", "marker", "SIGIL")

	// Close writer so the reader can drain.
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe writer: %v", closeErr)
	}

	// Read captured stderr.
	var stderrBuf bytes.Buffer
	if _, copyErr := io.Copy(&stderrBuf, r); copyErr != nil {
		t.Fatalf("drain pipe: %v", copyErr)
	}
	stderrOut := stderrBuf.String()

	// Read captured file.
	now := time.Now()
	logPath := filepath.Join(dataHome, now.Format("2006-01"),
		"hams."+now.Format("200601")+".log")
	fileBytes, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log file: %v", readErr)
	}
	fileOut := string(fileBytes)

	// Both must contain the sentinel.
	if !strings.Contains(stderrOut, "SIGIL") {
		t.Errorf("stderr missing SIGIL; got %q", stderrOut)
	}
	if !strings.Contains(fileOut, "SIGIL") {
		t.Errorf("log file missing SIGIL; got %q", fileOut)
	}
}
