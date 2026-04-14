package hamsfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path atomically using temp-file + fsync + rename.
// Creates parent directories if needed.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".hams-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Clean up temp file on any error path.
	success := false
	defer func() {
		if !success {
			tmp.Close()        //nolint:errcheck,gosec // best-effort cleanup on error path
			os.Remove(tmpName) //nolint:errcheck,gosec // best-effort cleanup on error path
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpName, path, err)
	}

	success = true
	return nil
}
