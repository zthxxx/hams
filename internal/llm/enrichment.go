// Package llm provides LLM subprocess integration for tag/intro enrichment.
package llm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// EnrichResult holds the enrichment outcome for one package.
type EnrichResult struct {
	PackageName string
	Tags        []string
	Intro       string
	Error       error
}

// EnrichAsync runs LLM enrichment for a package in a background goroutine.
// Returns a channel that will receive the result when done.
func EnrichAsync(ctx context.Context, cfg Config, packageName, description string, existingTags []string) <-chan EnrichResult {
	ch := make(chan EnrichResult, 1)
	go func() {
		defer close(ch)
		rec, err := Recommend(ctx, cfg, packageName, description, existingTags)
		if err != nil {
			ch <- EnrichResult{PackageName: packageName, Error: err}
			return
		}
		ch <- EnrichResult{
			PackageName: packageName,
			Tags:        rec.Tags,
			Intro:       rec.Intro,
		}
	}()
	return ch
}

// EnrichCollector collects async enrichment results during an apply session.
type EnrichCollector struct {
	mu      sync.Mutex
	results []EnrichResult
	pending []<-chan EnrichResult
}

// NewEnrichCollector creates a new collector.
func NewEnrichCollector() *EnrichCollector {
	return &EnrichCollector{}
}

// Add registers an async enrichment channel for collection.
func (c *EnrichCollector) Add(ch <-chan EnrichResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = append(c.pending, ch)
}

// CollectAll waits for all pending enrichments and returns the results.
// Failed enrichments are included with their errors for reporting.
func (c *EnrichCollector) CollectAll() []EnrichResult {
	c.mu.Lock()
	pending := c.pending
	c.pending = nil
	c.mu.Unlock()

	for _, ch := range pending {
		result := <-ch
		c.mu.Lock()
		c.results = append(c.results, result)
		c.mu.Unlock()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.results
}

// ReportErrors logs and returns a summary of enrichment failures.
func ReportErrors(results []EnrichResult) string {
	var failures int
	for _, r := range results {
		if r.Error != nil {
			slog.Warn("LLM enrichment failed", "package", r.PackageName, "error", r.Error)
			failures++
		}
	}
	if failures > 0 {
		return fmt.Sprintf("%d packages installed without LLM enrichment; run 'hams <provider> enrich <app>' to retry", failures)
	}
	return ""
}
