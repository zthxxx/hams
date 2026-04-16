package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	hamserr "github.com/zthxxx/hams/internal/error"
)

// TestParseCSV_BasicSplit asserts the parser splits comma-separated
// values into a lowercase set, trimming whitespace and dropping
// empty parts. Used by --only/--except parsing in apply.
func TestParseCSV_BasicSplit(t *testing.T) {
	t.Parallel()
	got := parseCSV("brew, APT,pnpm,,  npm  ")
	want := []string{"brew", "apt", "pnpm", "npm"}
	if len(got) != len(want) {
		t.Fatalf("size = %d, want %d (got %v)", len(got), len(want), got)
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %q in result %v", w, got)
		}
	}
}

// TestParseCSV_Empty returns an empty (but non-nil) map, so
// downstream code can call `if set[name]` without nil-checks.
func TestParseCSV_Empty(t *testing.T) {
	t.Parallel()
	got := parseCSV("")
	if got == nil {
		t.Error("expected non-nil empty map")
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

// TestParseCSV_OnlyWhitespaceAndCommas drops everything; result is
// safe to iterate over as zero entries.
func TestParseCSV_OnlyWhitespaceAndCommas(t *testing.T) {
	t.Parallel()
	got := parseCSV(",  ,,, ")
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

// TestValidateProviderNames_AllKnown is the happy path: all
// requested names appear in the known set, no error returned.
func TestValidateProviderNames_AllKnown(t *testing.T) {
	t.Parallel()
	requested := map[string]bool{"brew": true, "apt": true}
	known := map[string]bool{"brew": true, "apt": true, "pnpm": true}
	if err := validateProviderNames(requested, known, []string{"brew", "apt", "pnpm"}); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// TestValidateProviderNames_UnknownReportedWithSuggestion asserts
// the error message names the unknown providers AND lists the
// available providers as a suggestion. Per cli-architecture spec,
// errors for unknown providers MUST include the suggestion list.
func TestValidateProviderNames_UnknownReportedWithSuggestion(t *testing.T) {
	t.Parallel()
	requested := map[string]bool{"brew": true, "typo": true, "another-typo": true}
	known := map[string]bool{"brew": true, "apt": true, "pnpm": true}
	knownNames := []string{"brew", "apt", "pnpm"}
	err := validateProviderNames(requested, known, knownNames)
	if err == nil {
		t.Fatal("expected error for unknown providers")
	}

	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T", err)
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("Code = %d, want ExitUsageError", ufe.Code)
	}
	if !strings.Contains(ufe.Message, "typo") {
		t.Errorf("error should name unknown providers; got %q", ufe.Message)
	}
	suggestion := strings.Join(ufe.Suggestions, " ")
	if !strings.Contains(suggestion, "brew") || !strings.Contains(suggestion, "apt") || !strings.Contains(suggestion, "pnpm") {
		t.Errorf("suggestions should list available providers; got %v", ufe.Suggestions)
	}
}

// TestPrintError_TextMode asserts text-mode output writes
// "Error: <message>\n" + per-suggestion lines to stderr.
func TestPrintError_TextMode(t *testing.T) {
	got := captureStderr(t, func() {
		err := hamserr.NewUserError(hamserr.ExitUsageError, "bad flag", "use --help")
		PrintError(err, false)
	})
	if !strings.Contains(got, "Error: bad flag") {
		t.Errorf("missing error prefix; got %q", got)
	}
	if !strings.Contains(got, "suggestion: use --help") {
		t.Errorf("missing suggestion line; got %q", got)
	}
}

// TestPrintError_JSONMode asserts JSON-mode output writes a parseable
// JSON object including code, message, and suggestions per the
// cli-architecture spec.
func TestPrintError_JSONMode(t *testing.T) {
	got := captureStderr(t, func() {
		err := hamserr.NewUserError(hamserr.ExitUsageError, "bad flag", "use --help")
		PrintError(err, true)
	})
	// Should contain JSON-shape fields.
	if !strings.Contains(got, `"message"`) || !strings.Contains(got, "bad flag") {
		t.Errorf("JSON output missing message field; got %q", got)
	}
	if !strings.Contains(got, `"code"`) {
		t.Errorf("JSON output missing code field; got %q", got)
	}
	if !strings.Contains(got, `"suggestions"`) || !strings.Contains(got, "use --help") {
		t.Errorf("JSON output missing suggestions; got %q", got)
	}
}

// TestPrintError_PlainErrorIsWrapped asserts that passing a non-UserFacingError
// (e.g., a plain errors.New error) still produces structured output —
// PrintError wraps with default ExitGeneralError.
func TestPrintError_PlainErrorIsWrapped(t *testing.T) {
	got := captureStderr(t, func() {
		PrintError(errors.New("something broke"), false)
	})
	if !strings.Contains(got, "Error: something broke") {
		t.Errorf("plain errors should still produce 'Error: ...' line; got %q", got)
	}
}

// captureStderr swaps os.Stderr with a pipe for the duration of fn,
// returns the captured output. Restores stderr on return regardless
// of test outcome.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = original })

	fn()
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close pipe: %v", closeErr)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buf.String()
}
