package homebrew

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/hamsfile"
)

// TestParseBrewDescJSON_Formula asserts the parser extracts the
// `desc` field from the shape `brew info --json=v2 <formula>` returns.
// This is the dominant case — `hams brew install midnight-commander`
// hits exactly this path.
func TestParseBrewDescJSON_Formula(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"formulae": [{"name": "midnight-commander", "desc": "Terminal-based visual file manager"}],
		"casks": []
	}`)
	got, err := parseBrewDescJSON(raw)
	if err != nil {
		t.Fatalf("parseBrewDescJSON: %v", err)
	}
	if got != "Terminal-based visual file manager" {
		t.Errorf("desc = %q, want the formula's desc", got)
	}
}

// TestParseBrewDescJSON_Cask asserts the parser falls through to the
// casks array when formulae is empty — which is what `brew info
// --json=v2 --cask <name>` returns.
func TestParseBrewDescJSON_Cask(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"formulae": [],
		"casks": [{"token": "iterm2", "desc": "Terminal emulator as alternative to Apple's Terminal app"}]
	}`)
	got, err := parseBrewDescJSON(raw)
	if err != nil {
		t.Fatalf("parseBrewDescJSON: %v", err)
	}
	if !strings.HasPrefix(got, "Terminal emulator") {
		t.Errorf("desc = %q, want the cask's desc", got)
	}
}

// TestParseBrewDescJSON_EmptyArraysReturnsEmpty asserts the "not found
// in either set" case returns ("", nil) — the caller interprets that
// as "no intro available" and falls back to an empty intro, which the
// install flow must accept without erroring (intro is optional per
// openspec/specs/schema-design/spec.md:210).
func TestParseBrewDescJSON_EmptyArraysReturnsEmpty(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"formulae": [], "casks": []}`)
	got, err := parseBrewDescJSON(raw)
	if err != nil {
		t.Fatalf("parseBrewDescJSON: %v", err)
	}
	if got != "" {
		t.Errorf("desc = %q, want empty", got)
	}
}

// TestParseBrewDescJSON_EmptyDescFieldFallsThrough asserts: if the
// formula entry exists but its `desc` is empty (Homebrew has a few
// stub formulae like that), the parser falls through to the cask
// array rather than returning the empty string from the first hit.
// Prevents a confusing case where a legitimate cask desc is shadowed
// by a stub formula response.
func TestParseBrewDescJSON_EmptyDescFieldFallsThrough(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"formulae": [{"name": "stub", "desc": ""}],
		"casks": [{"token": "stub", "desc": "Real desc from the cask"}]
	}`)
	got, err := parseBrewDescJSON(raw)
	if err != nil {
		t.Fatalf("parseBrewDescJSON: %v", err)
	}
	if got != "Real desc from the cask" {
		t.Errorf("desc = %q, want the cask desc (empty formula.desc should not shadow)", got)
	}
}

// TestParseBrewDescJSON_MalformedBytesErrors asserts a truly broken
// JSON response produces an error — unlike the empty/missing-field
// cases, we can't fall back to "" here because the response shape
// is unknown. The install flow's IntroFn wrapper swallows it.
func TestParseBrewDescJSON_MalformedBytesErrors(t *testing.T) {
	t.Parallel()
	_, err := parseBrewDescJSON([]byte("this is not json"))
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

// TestHandleInstall_RecordsIntroFromDescribe is the end-to-end proof
// that `hams brew install <pkg>` writes the `intro:` field — the
// user-reported bug where `~/.local/share/hams/store/<tag>/Homebrew.hams.yaml`
// only contained `- app: midnight-commander` and no description.
// After the fix, the same flow seeds intro from the runner's Describe
// (which production wires to `brew info --json=v2`).
func TestHandleInstall_RecordsIntroFromDescribe(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)
	h.runner.SeedDescription("midnight-commander", "Terminal-based visual file manager")

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "midnight-commander"}, nil, h.flags); err != nil {
		t.Fatalf("handle install: %v", err)
	}

	if h.runner.CallCount(fakeOpDescribe, "midnight-commander") != 1 {
		t.Errorf("runner.Describe(midnight-commander) calls = %d, want 1",
			h.runner.CallCount(fakeOpDescribe, "midnight-commander"))
	}

	raw, err := os.ReadFile(h.hamsfilePath)
	if err != nil {
		t.Fatalf("read hamsfile: %v", err)
	}
	if !strings.Contains(string(raw), "intro: Terminal-based visual file manager") {
		t.Errorf("hamsfile missing intro line; contents:\n%s", raw)
	}

	// Sanity: the app is still recorded and the intro isn't being
	// written as a separate top-level entry.
	hf, err := hamsfile.Read(h.hamsfilePath)
	if err != nil {
		t.Fatalf("hamsfile.Read: %v", err)
	}
	apps := hf.ListApps()
	if len(apps) != 1 || apps[0] != "midnight-commander" {
		t.Errorf("ListApps() = %v, want [midnight-commander]", apps)
	}
}

// TestHandleInstall_DescribeFailureFallsBackToEmptyIntro asserts the
// install still succeeds and records the app even when the runner's
// Describe returns an error (brew offline, tap data stale, etc.).
// The `intro:` line is simply omitted — users shouldn't lose their
// install because the metadata lookup is flaky.
func TestHandleInstall_DescribeFailureFallsBackToEmptyIntro(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)
	h.runner.WithDescribeError(errors.New("brew: info network glitch"))

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "htop"}, nil, h.flags); err != nil {
		t.Fatalf("handle install (expected to succeed despite describe error): %v", err)
	}

	raw, err := os.ReadFile(h.hamsfilePath)
	if err != nil {
		t.Fatalf("read hamsfile: %v", err)
	}
	if strings.Contains(string(raw), "intro:") {
		t.Errorf("hamsfile unexpectedly contains intro: field; contents:\n%s", raw)
	}
	hf, err := hamsfile.Read(h.hamsfilePath)
	if err != nil {
		t.Fatalf("hamsfile.Read: %v", err)
	}
	if apps := hf.ListApps(); len(apps) != 1 || apps[0] != "htop" {
		t.Errorf("ListApps() = %v, want [htop]", apps)
	}
}

// TestHandleInstall_CaskRoutesDescribeWithCaskFlag asserts the
// isCask flag is propagated to Describe — necessary so brew info
// queries the cask manifest (distinct from a same-named formula).
// Without this, a `hams brew install --cask <name>` could read a
// formula's desc where a cask-specific one exists.
func TestHandleInstall_CaskRoutesDescribeWithCaskFlag(t *testing.T) {
	t.Parallel()
	h := newBrewHarness(t)
	h.runner.SeedDescription("visual-studio-code", "Open-source code editor")

	if err := h.provider.HandleCommand(context.Background(),
		[]string{"install", "--cask", "visual-studio-code"}, nil, h.flags); err != nil {
		t.Fatalf("handle install --cask: %v", err)
	}

	// Describe must have been called with isCask=true so the
	// production real runner hits `brew info --json=v2 --cask <name>`.
	// FakeCmdRunner records the isCask bit on every Describe call;
	// inspect the last one.
	var sawCaskDescribe bool
	h.runner.mu.Lock()
	for _, c := range h.runner.calls {
		if c.op == fakeOpDescribe && c.name == "visual-studio-code" && c.isCask {
			sawCaskDescribe = true
			break
		}
	}
	h.runner.mu.Unlock()
	if !sawCaskDescribe {
		t.Errorf("expected Describe(visual-studio-code, isCask=true), got calls: %+v", h.runner.calls)
	}
}
