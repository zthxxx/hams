package otel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSession(t *testing.T) {
	s := NewSession(Config{DataHome: t.TempDir(), Enabled: true})
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
}

func TestStartEndSpan(t *testing.T) {
	s := NewSession(Config{DataHome: t.TempDir(), Enabled: true})

	span := s.StartSpan("test-op", "", map[string]string{"provider": "brew"})
	if span.Name != "test-op" {
		t.Errorf("Name = %q, want 'test-op'", span.Name)
	}

	time.Sleep(time.Millisecond)
	s.EndSpan(span, "ok")

	if len(s.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(s.spans))
	}
	if s.spans[0].Status != "ok" {
		t.Errorf("Status = %q, want 'ok'", s.spans[0].Status)
	}
}

func TestRecordMetric(t *testing.T) {
	s := NewSession(Config{DataHome: t.TempDir(), Enabled: true})
	s.RecordMetric("hams.apply.duration", 1500.0, "ms", map[string]string{"profile": "macOS"})

	if len(s.metrics) != 1 {
		t.Fatalf("metrics = %d, want 1", len(s.metrics))
	}
	if s.metrics[0].Name != "hams.apply.duration" {
		t.Errorf("Name = %q, want 'hams.apply.duration'", s.metrics[0].Name)
	}
}

func TestFlush_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewSession(Config{DataHome: dir, Enabled: true})

	s.StartSpan("root", "", nil)
	s.EndSpan(s.StartSpan("child", "", nil), "ok")
	s.RecordMetric("duration", 100.0, "ms", nil)

	if err := s.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	// Check trace files exist.
	traceDir := filepath.Join(dir, "otel", "traces")
	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("ReadDir traces: %v", err)
	}
	if len(entries) == 0 {
		t.Error("no trace files written")
	}

	// Check metric files exist.
	metricDir := filepath.Join(dir, "otel", "metrics")
	entries, err = os.ReadDir(metricDir)
	if err != nil {
		t.Fatalf("ReadDir metrics: %v", err)
	}
	if len(entries) == 0 {
		t.Error("no metric files written")
	}
}

func TestFlush_DisabledNoOp(t *testing.T) {
	s := NewSession(Config{Enabled: false})
	s.RecordMetric("test", 1.0, "ms", nil)

	// Should not error even when disabled.
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush should be no-op when disabled: %v", err)
	}
}

func TestShutdown_WithTimeout(t *testing.T) {
	dir := t.TempDir()
	s := NewSession(Config{DataHome: dir, Enabled: true})
	s.RecordMetric("test", 1.0, "ms", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}
}

func TestEndSpan_Nil(_ *testing.T) {
	s := NewSession(Config{Enabled: true})
	s.EndSpan(nil, "ok") // Should not panic.
}
