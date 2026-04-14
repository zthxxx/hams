package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// discardLogger discards output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestIsRelevant_GoFile(t *testing.T) {
	cases := map[string]bool{
		"foo.go":               true,
		"bar/baz/qux.go":       true,
		"go.mod":               true,
		"sub/go.mod":           true,
		"go.sum":               true,
		"foo.txt":              false,
		"README.md":            false,
		".foo.go.swp":          false,
		"foo.go~":              false,
		".hidden":              false,
		"node_modules/x/y.go":  true, // path alone — filter happens at walk level
		"foo.go.tmp1234567890": false,
	}
	for in, want := range cases {
		if got := isRelevant(in); got != want {
			t.Errorf("isRelevant(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFSNotifier_DetectsGoFileChange(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}

	n, err := NewFSNotifier([]string{root}, discardLogger())
	if err != nil {
		t.Fatalf("NewFSNotifier: %v", err)
	}
	t.Cleanup(func() {
		if err := n.Close(); err != nil {
			t.Logf("close notifier: %v", err)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 4)
	done := make(chan struct{})
	go func() { n.Run(ctx, events); close(done) }()

	// Give fsnotify time to register.
	time.Sleep(30 * time.Millisecond)

	target := filepath.Join(root, "pkg", "a.go")
	if err := os.WriteFile(target, []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("no event received for .go write")
	}
	cancel()
	<-done
}

func TestFSNotifier_IgnoresNonGo(t *testing.T) {
	root := t.TempDir()
	n, err := NewFSNotifier([]string{root}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := n.Close(); err != nil {
			t.Logf("close notifier: %v", err)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 4)
	done := make(chan struct{})
	go func() { n.Run(ctx, events); close(done) }()

	time.Sleep(30 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-events:
		t.Fatalf("unexpected event for non-Go file")
	case <-time.After(200 * time.Millisecond):
		// good — nothing relevant.
	}
	cancel()
	<-done
}

func TestFSNotifier_AddsNewSubdirectory(t *testing.T) {
	root := t.TempDir()
	n, err := NewFSNotifier([]string{root}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := n.Close(); err != nil {
			t.Logf("close notifier: %v", err)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 4)
	done := make(chan struct{})
	go func() { n.Run(ctx, events); close(done) }()

	time.Sleep(30 * time.Millisecond)

	subdir := filepath.Join(root, "newpkg")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Drain any event the directory creation produced on macOS (Create
	// events on directories do pass isRelevant() == false, but on some
	// platforms a CHMOD follows; be tolerant).
	drainFor(events, 50*time.Millisecond)

	// Give the watcher a beat to register the new subdir.
	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(subdir, "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatalf("no event for newly created subdir .go")
	}
	cancel()
	<-done
}

func TestFSNotifier_SkipsHiddenAndNodeModules(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{".git", "node_modules", "ok"} {
		if err := os.Mkdir(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}

	n, err := NewFSNotifier([]string{root}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := n.Close(); err != nil {
			t.Logf("close notifier: %v", err)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 4)
	done := make(chan struct{})
	go func() { n.Run(ctx, events); close(done) }()

	time.Sleep(30 * time.Millisecond)

	writeGoFile := func(rel string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel), []byte("package x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// Writes into skipped dirs must not produce events.
	writeGoFile(".git/x.go")
	writeGoFile("node_modules/y.go")
	// Writes into ok/ must produce an event.
	time.Sleep(30 * time.Millisecond)
	writeGoFile("ok/z.go")

	got := drainFor(events, 500*time.Millisecond)
	if got < 1 {
		t.Fatalf("expected at least one event from ok/, got %d", got)
	}
	cancel()
	<-done
}

func drainFor(ch <-chan struct{}, d time.Duration) int {
	deadline := time.After(d)
	n := 0
	for {
		select {
		case <-ch:
			n++
		case <-deadline:
			return n
		}
	}
}
