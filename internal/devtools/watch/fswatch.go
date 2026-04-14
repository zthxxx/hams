package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// FSNotifier wraps fsnotify.Watcher with recursive directory tracking and
// filtering down to Go source changes.
//
// Filter rules:
//   - Only directory paths are registered with fsnotify (fsnotify itself
//     delivers events on files inside watched directories).
//   - New directories created during the session are added automatically on
//     Create events so contributors adding a new package do not have to
//     restart the watcher.
//   - Events on files whose base name starts with "." (editor swap files,
//     vim .swp, etc.) are dropped.
//   - Only events on files with extensions in a small allowlist (.go,
//     go.mod, go.sum) produce rebuilds. Directory events do not.
type FSNotifier struct {
	watcher *fsnotify.Watcher
	roots   []string
	logger  *slog.Logger
}

// NewFSNotifier constructs an FSNotifier watching the given roots. Each root
// is walked recursively on start; fsnotify watchers are added for every
// directory encountered.
func NewFSNotifier(roots []string, logger *slog.Logger) (*FSNotifier, error) {
	if len(roots) == 0 {
		return nil, errors.New("fswatch: no roots provided")
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fswatch: new watcher: %w", err)
	}
	fn := &FSNotifier{watcher: w, roots: roots, logger: logger}
	for _, root := range roots {
		if err := fn.addTree(root); err != nil {
			if closeErr := w.Close(); closeErr != nil {
				logger.Warn("fswatch: close watcher after error", "err", closeErr)
			}
			return nil, err
		}
	}
	return fn, nil
}

// Close releases the underlying fsnotify watcher.
func (f *FSNotifier) Close() error { return f.watcher.Close() }

// Run translates raw fsnotify events into debounce ticks sent on out.
// It returns when ctx is canceled or the fsnotify channel closes.
func (f *FSNotifier) Run(ctx context.Context, out chan<- struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-f.watcher.Errors:
			if !ok {
				return
			}
			f.logger.Warn("fsnotify error", "err", err)
		case ev, ok := <-f.watcher.Events:
			if !ok {
				return
			}
			f.handleEvent(ev, out)
		}
	}
}

func (f *FSNotifier) handleEvent(ev fsnotify.Event, out chan<- struct{}) {
	// If a directory was created, start watching it so files inside it
	// produce events.
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if err := f.addTree(ev.Name); err != nil {
				f.logger.Warn("fswatch: add new dir", "path", ev.Name, "err", err)
			}
		}
	}
	if !isRelevant(ev.Name) {
		return
	}
	// Drop pure CHMOD events — editors often touch metadata.
	if ev.Op == fsnotify.Chmod {
		return
	}
	// Non-blocking send: if the engine hasn't yet consumed the previous
	// tick, one more doesn't add information.
	select {
	case out <- struct{}{}:
	default:
	}
}

// addTree walks root and adds every directory to the underlying watcher.
// Files are not added individually; fsnotify notifies file events through
// their parent directory's watcher.
func (f *FSNotifier) addTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// On ENOTDIR / missing subtree we tolerate and continue —
			// a race with an in-flight delete is fine.
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		// Skip hidden directories (.git, .idea, node_modules) to reduce
		// inotify churn on large repos.
		name := d.Name()
		if strings.HasPrefix(name, ".") && path != root {
			return filepath.SkipDir
		}
		if name == "node_modules" || name == "bin" {
			return filepath.SkipDir
		}
		if err := f.watcher.Add(path); err != nil {
			return fmt.Errorf("fswatch: add %s: %w", path, err)
		}
		return nil
	})
}

// isRelevant returns true for events that should trigger a rebuild.
// Only Go source files and the module metadata files qualify.
func isRelevant(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return false
	}
	// Editor backup files that don't start with a dot.
	if strings.HasSuffix(base, "~") {
		return false
	}
	switch base {
	case "go.mod", "go.sum":
		return true
	}
	return filepath.Ext(base) == ".go"
}
