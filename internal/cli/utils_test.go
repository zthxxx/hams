package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zthxxx/hams/internal/config"
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

// TestValidateProviderNames_UnknownListIsAlphabetical locks in
// cycle 152: when multiple typo'd providers are in the requested
// set, the resulting error message lists them in stable,
// alphabetical order across runs. Previously the unknown slice was
// populated by iterating the requested map (Go map iteration is
// non-deterministic), so a user typing `--only=foo,bar,baz` saw
// the unknown providers in a different order on every run, breaking
// any script grepping the error text. Symmetric with cycles 148-151.
func TestValidateProviderNames_UnknownListIsAlphabetical(t *testing.T) {
	t.Parallel()
	requested := map[string]bool{"zfoo": true, "abar": true, "mbaz": true}
	known := map[string]bool{"brew": true, "apt": true}
	knownNames := []string{"brew", "apt"}

	first := validateProviderNames(requested, known, knownNames)
	if first == nil {
		t.Fatal("expected error for 3 unknown providers")
	}
	var firstUFE *hamserr.UserFacingError
	if !errors.As(first, &firstUFE) {
		t.Fatalf("expected *UserFacingError, got %T", first)
	}

	// 20 reps must produce byte-identical Message + Suggestions.
	for range 20 {
		again := validateProviderNames(requested, known, knownNames)
		var ufe *hamserr.UserFacingError
		if !errors.As(again, &ufe) {
			t.Fatalf("expected *UserFacingError, got %T", again)
		}
		if ufe.Message != firstUFE.Message {
			t.Errorf("Message differs across runs:\nfirst:  %q\nlater:  %q", firstUFE.Message, ufe.Message)
			break
		}
	}

	// Assert alphabetical positioning of the three names.
	idxA := strings.Index(firstUFE.Message, "abar")
	idxM := strings.Index(firstUFE.Message, "mbaz")
	idxZ := strings.Index(firstUFE.Message, "zfoo")
	if idxA < 0 || idxM < 0 || idxZ < 0 {
		t.Fatalf("Message missing one of the unknown names; got %q", firstUFE.Message)
	}
	if idxA >= idxM || idxM >= idxZ {
		t.Errorf("unknown names not alphabetical (abar < mbaz < zfoo); got %q", firstUFE.Message)
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

// TestPrintError_JSONMode_PlainErrorIncludesErrorCode locks in cycle 245:
// PrintError's fallback path (non-UserFacingError) must still emit a
// populated error_code field in JSON mode. Previously the fallback
// constructed a bare &UserFacingError{Code, Message} leaving ErrorCode
// at its zero value; json:"error_code,omitempty" then stripped the
// field from output. CI scripts parsing error_code per the
// cli-architecture spec §"Error in JSON mode" saw no field on plain
// errors while UserFacingError errors carried "GENERAL_ERROR" — a
// silent shape divergence between call sites. Now every JSON error
// carries error_code.
func TestPrintError_JSONMode_PlainErrorIncludesErrorCode(t *testing.T) {
	got := captureStderr(t, func() {
		PrintError(errors.New("network blew up"), true)
	})

	var data map[string]any
	if err := json.Unmarshal([]byte(got), &data); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %q", err, got)
	}

	if data["error_code"] != string(hamserr.CodeGeneralError) {
		t.Errorf("error_code = %v, want %q (fallback plain-error path should map ExitGeneralError → GENERAL_ERROR)",
			data["error_code"], hamserr.CodeGeneralError)
	}
	if code, ok := data["code"].(float64); !ok || int(code) != hamserr.ExitGeneralError {
		t.Errorf("code = %v (ok=%v), want %d",
			data["code"], ok, hamserr.ExitGeneralError)
	}
	if !strings.Contains(got, "network blew up") {
		t.Errorf("message should surface the plain error text; got %q", got)
	}
}

// TestShortName_ExtractsURNSuffix asserts cycle-71: shortName
// strips the `urn:hams:<provider>:` prefix to yield just the
// resource name used by `hams list --json`'s `name` field per
// the cli-architecture spec.
func TestShortName_ExtractsURNSuffix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"urn:hams:apt:htop", "htop"},
		{"urn:hams:brew:git", "git"},
		{"urn:hams:defaults:com.apple.dock.autohide", "com.apple.dock.autohide"},
		{"htop", "htop"},                 // bare name passthrough
		{"", ""},                         // empty passthrough
		{"urn:hams:", "urn:hams:"},       // malformed URN (no provider) passthrough
		{"urn:hams:apt", "urn:hams:apt"}, // missing colon after provider → passthrough
	}
	for _, tc := range cases {
		if got := shortName(tc.in); got != tc.want {
			t.Errorf("shortName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPrintConfigKey_TypedFields asserts each whitelisted typed
// field prints the expected Config field; previously 0% coverage.
func TestPrintConfigKey_TypedFields(t *testing.T) {
	cfg := &config.Config{
		ProfileTag: "macOS",
		MachineID:  "MyMac",
		StoreRepo:  "user/repo",
		LLMCLI:     "claude",
	}
	paths := config.Paths{ConfigHome: "/x/cfg", DataHome: "/x/data"}

	cases := []struct {
		key      string
		contains string
	}{
		{"profile_tag", "macOS"},
		{"machine_id", "MyMac"},
		{"store_repo", "user/repo"},
		{"llm_cli", "claude"},
		{"config_home", "/x/cfg"},
		{"data_home", "/x/data"},
	}
	for _, tc := range cases {
		got := captureStdout(t, func() {
			if err := printConfigKey(cfg, paths, "", tc.key); err != nil {
				t.Fatalf("printConfigKey(%q): %v", tc.key, err)
			}
		})
		if !strings.Contains(got, tc.contains) {
			t.Errorf("%s: got %q, want to contain %q", tc.key, got, tc.contains)
		}
	}
}

// TestPrintConfigKey_UnknownKeyReturnsUserError asserts typos are
// rejected with both the whitelist and the sensitive-pattern hint.
func TestPrintConfigKey_UnknownKeyReturnsUserError(t *testing.T) {
	t.Parallel()
	err := printConfigKey(&config.Config{}, config.Paths{}, "", "profile_tg")
	if err == nil {
		t.Fatal("expected error for typo key")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("expected *UserFacingError, got %T", err)
	}
	if len(ufe.Suggestions) != 2 {
		t.Errorf("want 2 suggestions (whitelist + pattern hint), got %d", len(ufe.Suggestions))
	}
}

// TestPrintConfigKey_SensitiveKey_NoFile asserts a sensitive key
// with no .local.yaml on disk prints nothing (scripting-friendly)
// and returns nil error.
func TestPrintConfigKey_SensitiveKey_NoFile(t *testing.T) {
	t.Parallel()
	paths := config.Paths{ConfigHome: t.TempDir(), DataHome: t.TempDir()}
	got := captureStdout(t, func() {
		if err := printConfigKey(&config.Config{}, paths, "", "notification.bark_token"); err != nil {
			t.Errorf("unset sensitive key should be silent, got %v", err)
		}
	})
	if got != "" {
		t.Errorf("output should be empty for unset sensitive key, got %q", got)
	}
}

// TestPrintConfigKeyMode_JSONShape — cycle 236. `hams --json config
// get <key>` previously fell through to plain text (config get
// didn't honor --json), so CI consumers had to special-case it. The
// new printConfigKeyMode emits a stable structured object with key,
// value, and a `set` boolean that distinguishes "set to empty
// string" from "unset". Asserts the four representative cases.
func TestPrintConfigKeyMode_JSONShape(t *testing.T) {
	t.Parallel()
	paths := config.Paths{ConfigHome: t.TempDir(), DataHome: t.TempDir()}

	cases := []struct {
		name    string
		cfg     *config.Config
		key     string
		wantSet bool
		wantVal string
	}{
		{"unset profile_tag", &config.Config{}, "profile_tag", false, ""},
		{"set profile_tag", &config.Config{ProfileTag: "macOS"}, "profile_tag", true, "macOS"},
		{"unset machine_id", &config.Config{}, "machine_id", false, ""},
		{"set machine_id", &config.Config{MachineID: "laptop-01"}, "machine_id", true, "laptop-01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := captureStdout(t, func() {
				if err := printConfigKeyMode(tc.cfg, paths, "", tc.key, true); err != nil {
					t.Fatalf("printConfigKeyMode: %v", err)
				}
			})
			var data map[string]any
			if err := json.Unmarshal([]byte(got), &data); err != nil {
				t.Fatalf("output not valid JSON: %v\nraw: %q", err, got)
			}
			if data["key"] != tc.key {
				t.Errorf("key = %v, want %q", data["key"], tc.key)
			}
			if data["value"] != tc.wantVal {
				t.Errorf("value = %v, want %q", data["value"], tc.wantVal)
			}
			if data["set"] != tc.wantSet {
				t.Errorf("set = %v, want %v", data["set"], tc.wantSet)
			}
		})
	}
}

// captureStdout is the stdout twin of captureStderr.
// captureStdoutMu serializes os.Stdout swaps across tests that run
// in parallel. Without this, two Parallel tests both calling
// captureStdout would race on the global os.Stdout variable — Go's
// race detector flags this and the test suite fails.
var captureStdoutMu sync.Mutex

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	captureStdoutMu.Lock()
	defer captureStdoutMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()
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

// TestEnsureStoreIsGitRepo covers the three gates added in cycle 27:
// a real .git directory passes, a bare-repo HEAD file passes, and
// anything else returns a UserFacingError with both suggestions.
func TestEnsureStoreIsGitRepo(t *testing.T) {
	t.Run("non-bare repo passes", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
			t.Fatalf("mkdir .git: %v", err)
		}
		if err := ensureStoreIsGitRepo(dir); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
	t.Run("bare repo passes", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
			t.Fatalf("write HEAD: %v", err)
		}
		if err := ensureStoreIsGitRepo(dir); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
	t.Run("plain directory returns UserFacingError", func(t *testing.T) {
		dir := t.TempDir()
		err := ensureStoreIsGitRepo(dir)
		if err == nil {
			t.Fatal("expected error for non-git dir")
		}
		var ufe *hamserr.UserFacingError
		if !errors.As(err, &ufe) {
			t.Fatalf("expected *UserFacingError, got %T", err)
		}
		if ufe.Code != hamserr.ExitUsageError {
			t.Errorf("Code = %d, want ExitUsageError", ufe.Code)
		}
		if len(ufe.Suggestions) != 2 {
			t.Errorf("want 2 suggestions (git init, --from-repo), got %d", len(ufe.Suggestions))
		}
	})
}

// TestLocalConfigPath covers the routing helper that cycle 18 added:
// when storePath is empty, the local config sits under ConfigHome;
// otherwise it sits in the store.
func TestLocalConfigPath(t *testing.T) {
	paths := config.Paths{ConfigHome: "/home/u/.config/hams"}

	if got := localConfigPath(paths, ""); got != "/home/u/.config/hams/hams.config.local.yaml" {
		t.Errorf("no-store fallback = %q, want the global path", got)
	}
	if got := localConfigPath(paths, "/store"); got != "/store/hams.config.local.yaml" {
		t.Errorf("store-scoped = %q, want '/store/hams.config.local.yaml'", got)
	}
}

// captureStderrMu serializes os.Stderr swaps, same rationale as
// captureStdoutMu: concurrent -race runs flagged the global
// variable mutation as a race.
var captureStderrMu sync.Mutex

// captureStderr swaps os.Stderr with a pipe for the duration of fn,
// returns the captured output. Restores stderr on return regardless
// of test outcome.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	captureStderrMu.Lock()
	defer captureStderrMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = original }()

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
