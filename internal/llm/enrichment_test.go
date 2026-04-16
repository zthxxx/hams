package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRecommend_NoLLMConfigured exercises the error path in
// Recommend that fires when cfg.CLI is empty. This is the primary
// reachable branch in unit tests because Recommend's success path
// spawns a real subprocess (out of scope for host-safe unit tests).
func TestRecommend_NoLLMConfigured(t *testing.T) {
	t.Parallel()
	_, err := Recommend(context.Background(), Config{CLI: ""}, "htop", "desc", nil)
	if err == nil {
		t.Fatal("expected error when CLI is empty, got nil")
	}
	if !strings.Contains(err.Error(), "LLM CLI not configured") {
		t.Errorf("error message missing the actionable hint: %v", err)
	}
}

// TestEnrichAsync_PropagatesRecommendError asserts that EnrichAsync
// surfaces a Recommend failure via EnrichResult.Error rather than
// crashing the goroutine. The "no CLI" path is the deterministic
// failure shape.
func TestEnrichAsync_PropagatesRecommendError(t *testing.T) {
	t.Parallel()
	ch := EnrichAsync(context.Background(), Config{CLI: ""}, "htop", "desc", nil)

	select {
	case res := <-ch:
		if res.Error == nil {
			t.Fatalf("expected non-nil Error in result; got %+v", res)
		}
		if res.PackageName != "htop" {
			t.Errorf("PackageName = %q, want htop", res.PackageName)
		}
	case <-context.Background().Done():
		t.Fatal("ctx unexpectedly canceled before result")
	}
}

// TestEnrichCollector_AddCollectAll asserts the collector's basic
// queue-and-drain semantics: each Add'd channel is consumed by
// CollectAll, and results accumulate across calls.
func TestEnrichCollector_AddCollectAll(t *testing.T) {
	t.Parallel()
	c := NewEnrichCollector()

	// Synthesize three pre-populated channels (immediate read).
	makeCh := func(name string, err error) <-chan EnrichResult {
		ch := make(chan EnrichResult, 1)
		ch <- EnrichResult{PackageName: name, Error: err}
		close(ch)
		return ch
	}
	c.Add(makeCh("vim", nil))
	c.Add(makeCh("nano", errors.New("simulated")))
	c.Add(makeCh("emacs", nil))

	results := c.CollectAll()
	if len(results) != 3 {
		t.Fatalf("CollectAll = %d results, want 3", len(results))
	}

	got := map[string]bool{}
	for _, r := range results {
		got[r.PackageName] = true
	}
	for _, want := range []string{"vim", "nano", "emacs"} {
		if !got[want] {
			t.Errorf("result missing package %q", want)
		}
	}

	// Second CollectAll on an empty pending should return same set
	// (results accumulate and are not cleared).
	more := c.CollectAll()
	if len(more) != 3 {
		t.Errorf("second CollectAll = %d results, want 3 (accumulator)", len(more))
	}
}

// TestEnrichCollector_AddAfterCollect asserts that channels added
// after a CollectAll are still drained on the next call. Important
// for overlapping apply phases where new packages enrich while
// earlier ones complete.
func TestEnrichCollector_AddAfterCollect(t *testing.T) {
	t.Parallel()
	c := NewEnrichCollector()

	first := make(chan EnrichResult, 1)
	first <- EnrichResult{PackageName: "first"}
	close(first)
	c.Add(first)
	_ = c.CollectAll()

	second := make(chan EnrichResult, 1)
	second <- EnrichResult{PackageName: "second"}
	close(second)
	c.Add(second)

	results := c.CollectAll()
	// Accumulator includes both first AND second.
	names := map[string]bool{}
	for _, r := range results {
		names[r.PackageName] = true
	}
	if !names["first"] || !names["second"] {
		t.Errorf("expected both first + second in accumulator, got %v", names)
	}
}

// TestReportErrors_Empty asserts no summary is produced when all
// enrichments succeeded.
func TestReportErrors_Empty(t *testing.T) {
	t.Parallel()
	results := []EnrichResult{
		{PackageName: "vim", Tags: []string{"editor"}},
		{PackageName: "nano", Tags: []string{"editor"}},
	}
	got := ReportErrors(results)
	if got != "" {
		t.Errorf("ReportErrors = %q, want empty (no failures)", got)
	}
}

// TestReportErrors_WithFailures asserts the summary message names
// the failing-package count and includes the actionable retry hint
// (`hams <provider> enrich <app>`).
func TestReportErrors_WithFailures(t *testing.T) {
	t.Parallel()
	results := []EnrichResult{
		{PackageName: "vim", Tags: []string{"editor"}},
		{PackageName: "nano", Error: errors.New("simulated")},
		{PackageName: "emacs", Error: errors.New("simulated")},
	}
	got := ReportErrors(results)
	if !strings.Contains(got, "2 packages installed without LLM enrichment") {
		t.Errorf("summary should mention failure count; got %q", got)
	}
	if !strings.Contains(got, "enrich") {
		t.Errorf("summary should include the retry hint; got %q", got)
	}
}
