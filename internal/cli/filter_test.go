package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// namedProvider is a minimal provider.Provider implementation used by
// filterProviders tests. The filter only consults p.Manifest().Name;
// the other methods are no-op stubs to satisfy the interface.
type namedProvider struct{ name string }

func (p *namedProvider) Manifest() provider.Manifest {
	return provider.Manifest{Name: p.name, DisplayName: p.name}
}
func (p *namedProvider) Bootstrap(_ context.Context) error { return nil }
func (p *namedProvider) Probe(_ context.Context, _ *state.File) ([]provider.ProbeResult, error) {
	return nil, nil
}
func (p *namedProvider) Plan(_ context.Context, _ *hamsfile.File, _ *state.File) ([]provider.Action, error) {
	return nil, nil
}
func (p *namedProvider) Apply(_ context.Context, _ provider.Action) error { return nil }
func (p *namedProvider) Remove(_ context.Context, _ string) error         { return nil }
func (p *namedProvider) List(_ context.Context, _ *hamsfile.File, _ *state.File) (string, error) {
	return "", nil
}

// namesOf extracts the Manifest().Name of each provider for assertion
// output comparisons.
func namesOf(providers []provider.Provider) []string {
	out := make([]string, len(providers))
	for i, p := range providers {
		out[i] = p.Manifest().Name
	}
	return out
}

// TestFilterProviders_NeitherFlagReturnsAll asserts the no-filter
// short-circuit: both args empty → return the input unmodified.
func TestFilterProviders_NeitherFlagReturnsAll(t *testing.T) {
	t.Parallel()
	input := []provider.Provider{
		&namedProvider{name: "apt"},
		&namedProvider{name: "brew"},
	}
	got, err := filterProviders(input, "", "", []string{"apt", "brew"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 providers, got %d", len(got))
	}
}

// TestFilterProviders_MutualExclusion asserts setting both --only and
// --except surfaces a usage error.
func TestFilterProviders_MutualExclusion(t *testing.T) {
	t.Parallel()
	input := []provider.Provider{&namedProvider{name: "apt"}}
	_, err := filterProviders(input, "apt", "brew", []string{"apt", "brew"})
	if err == nil {
		t.Fatalf("expected mutual-exclusion error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention 'mutually exclusive', got: %v", err)
	}
}

// TestFilterProviders_OnlyFiltersDown asserts --only keeps only the
// named providers.
func TestFilterProviders_OnlyFiltersDown(t *testing.T) {
	t.Parallel()
	input := []provider.Provider{
		&namedProvider{name: "apt"},
		&namedProvider{name: "brew"},
		&namedProvider{name: "npm"},
	}
	got, err := filterProviders(input, "brew,npm", "", []string{"apt", "brew", "npm"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := namesOf(got)
	want := map[string]bool{"brew": true, "npm": true}
	if len(names) != 2 {
		t.Fatalf("want 2 providers, got %d (%v)", len(names), names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected provider %q in filtered list", n)
		}
	}
}

// TestFilterProviders_ExceptFiltersOut asserts --except drops the
// named providers.
func TestFilterProviders_ExceptFiltersOut(t *testing.T) {
	t.Parallel()
	input := []provider.Provider{
		&namedProvider{name: "apt"},
		&namedProvider{name: "brew"},
		&namedProvider{name: "npm"},
	}
	got, err := filterProviders(input, "", "brew", []string{"apt", "brew", "npm"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := namesOf(got)
	if len(names) != 2 {
		t.Fatalf("want 2 providers, got %d (%v)", len(names), names)
	}
	for _, n := range names {
		if n == "brew" {
			t.Errorf("brew should have been filtered out, got %v", names)
		}
	}
}

// TestFilterProviders_CaseInsensitive asserts provider name matching
// is case-insensitive. `--only=BREW` should match provider "brew".
func TestFilterProviders_CaseInsensitive(t *testing.T) {
	t.Parallel()
	input := []provider.Provider{
		&namedProvider{name: "brew"},
		&namedProvider{name: "apt"},
	}
	got, err := filterProviders(input, "BREW", "", []string{"apt", "brew"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Manifest().Name != "brew" {
		t.Errorf("case-insensitive match failed: got %v", namesOf(got))
	}
}

// TestFilterProviders_EmptyPartsTrimmed asserts whitespace and empty
// CSV parts are silently dropped.
func TestFilterProviders_EmptyPartsTrimmed(t *testing.T) {
	t.Parallel()
	input := []provider.Provider{&namedProvider{name: "apt"}}
	// Input "  ,  apt,  ,  " should be equivalent to "apt".
	got, err := filterProviders(input, "  ,  apt,  ,  ", "", []string{"apt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 provider, got %d (%v)", len(got), namesOf(got))
	}
}

// TestFilterProviders_UnknownOnlyNameErrors asserts --only with an
// unknown provider name surfaces a helpful error naming the unknown
// value AND carrying available-providers in its suggestions list (the
// UserFacingError surface that PrintError renders, not the bare
// Error() message).
func TestFilterProviders_UnknownOnlyNameErrors(t *testing.T) {
	t.Parallel()
	input := []provider.Provider{&namedProvider{name: "apt"}}
	_, err := filterProviders(input, "bogus", "", []string{"apt", "brew"})
	if err == nil {
		t.Fatalf("expected unknown-provider error, got nil")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should name the unknown provider, got: %v", err)
	}
	var ufErr *hamserr.UserFacingError
	if !errors.As(err, &ufErr) {
		t.Fatalf("error should be a *UserFacingError, got %T: %v", err, err)
	}
	joined := strings.Join(ufErr.Suggestions, " | ")
	if !strings.Contains(joined, "apt") {
		t.Errorf("suggestions should list available providers, got: %q", joined)
	}
}

// TestFilterProviders_UnknownExceptNameErrors asserts --except with an
// unknown name also errors (not silently accepted).
func TestFilterProviders_UnknownExceptNameErrors(t *testing.T) {
	t.Parallel()
	input := []provider.Provider{&namedProvider{name: "apt"}}
	_, err := filterProviders(input, "", "bogus", []string{"apt"})
	if err == nil {
		t.Fatalf("expected unknown-provider error, got nil")
	}
}

// TestParseCSV_DropsEmptyAndTrimsWhitespace pins the CSV parser's
// contract. Used to detect drift if someone "optimizes" the parser
// and accidentally keeps empty-after-trim entries as map keys.
func TestParseCSV_DropsEmptyAndTrimsWhitespace(t *testing.T) {
	t.Parallel()
	got := parseCSV("  apt , brew ,  , npm,")
	want := map[string]bool{"apt": true, "brew": true, "npm": true}
	if len(got) != len(want) {
		t.Fatalf("parseCSV count = %d, want %d (%v)", len(got), len(want), got)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("parseCSV missing key %q", k)
		}
	}
}
