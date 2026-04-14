// Package otel provides OpenTelemetry trace and metrics instrumentation with local file export.
package otel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Exporter writes collected trace spans and metrics to a backend.
type Exporter interface {
	// ExportSpans writes spans to the backend.
	ExportSpans(spans []Span) error
	// ExportMetrics writes metrics to the backend.
	ExportMetrics(metrics []Metric) error
}

// Config holds OTel configuration.
type Config struct {
	DataHome string
	Enabled  bool
	Exporter Exporter // Optional custom exporter; defaults to local file exporter.
}

// Session holds the active OTel session for trace/metrics collection.
type Session struct {
	config    Config
	startTime time.Time
	traceID   string
	spans     []Span
	metrics   []Metric
}

// Span represents a trace span.
type Span struct {
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id"`
	ParentID  string            `json:"parent_id,omitempty"`
	Name      string            `json:"name"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time"`
	Status    string            `json:"status"`
	Attrs     map[string]string `json:"attributes,omitempty"`
}

// Metric represents a recorded metric.
type Metric struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Unit      string            `json:"unit"`
	Timestamp time.Time         `json:"timestamp"`
	Attrs     map[string]string `json:"attributes,omitempty"`
}

// NewSession creates a new OTel session.
func NewSession(cfg Config) *Session {
	return &Session{
		config:    cfg,
		startTime: time.Now(),
		traceID:   randomHex(16), // 32 hex chars, W3C trace-id format.
	}
}

// StartSpan begins a new span and returns it.
func (s *Session) StartSpan(name, parentID string, attrs map[string]string) *Span {
	span := Span{
		TraceID:   s.traceID,
		SpanID:    randomHex(8), // 16 hex chars, unique per span.
		ParentID:  parentID,
		Name:      name,
		StartTime: time.Now(),
		Attrs:     attrs,
	}
	return &span
}

// EndSpan finishes a span and records it.
func (s *Session) EndSpan(span *Span, status string) {
	if span == nil {
		return
	}
	span.EndTime = time.Now()
	span.Status = status
	s.spans = append(s.spans, *span)
}

// RecordMetric adds a metric value.
func (s *Session) RecordMetric(name string, value float64, unit string, attrs map[string]string) {
	s.metrics = append(s.metrics, Metric{
		Name:      name,
		Value:     value,
		Unit:      unit,
		Timestamp: time.Now(),
		Attrs:     attrs,
	})
}

// Flush writes all collected spans and metrics via the configured exporter.
func (s *Session) Flush() error {
	if !s.config.Enabled {
		return nil
	}

	exp := s.config.Exporter
	if exp == nil {
		if s.config.DataHome == "" {
			return nil
		}
		exp = &LocalFileExporter{DataHome: s.config.DataHome}
	}

	if len(s.spans) > 0 {
		if err := exp.ExportSpans(s.spans); err != nil {
			return fmt.Errorf("exporting traces: %w", err)
		}
	}

	if len(s.metrics) > 0 {
		if err := exp.ExportMetrics(s.metrics); err != nil {
			return fmt.Errorf("exporting metrics: %w", err)
		}
	}

	return nil
}

// LocalFileExporter writes spans/metrics as JSON to local files.
type LocalFileExporter struct {
	DataHome string
}

// ExportSpans writes spans to local JSON files.
func (e *LocalFileExporter) ExportSpans(spans []Span) error {
	return writeJSON(filepath.Join(e.DataHome, "otel", "traces"), spans)
}

// ExportMetrics writes metrics to local JSON files.
func (e *LocalFileExporter) ExportMetrics(metrics []Metric) error {
	return writeJSON(filepath.Join(e.DataHome, "otel", "metrics"), metrics)
}

// Shutdown flushes and cleans up the OTel session.
func (s *Session) Shutdown(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		done <- s.Flush()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		slog.Warn("OTel shutdown timed out, traces may be lost")
		return ctx.Err()
	}
}

// randomHex generates a random hex string of n bytes (2n hex characters).
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error on supported platforms.
	return hex.EncodeToString(b)
}

func writeJSON(dir string, data any) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating otel directory %s: %w", dir, err)
	}

	filename := fmt.Sprintf("%s.json", time.Now().Format("20060102T150405"))
	path := filepath.Join(dir, filename)

	f, err := os.Create(path) //nolint:gosec // otel path derived from XDG data directory
	if err != nil {
		return fmt.Errorf("creating otel file: %w", err)
	}
	defer f.Close() //nolint:errcheck // best-effort flush

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
