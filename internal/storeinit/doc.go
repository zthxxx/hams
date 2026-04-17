// Package storeinit creates a fresh hams store directory pre-initialized with
// a `.gitignore`, an empty `hams.config.yaml`, and a `git init`-ed working
// tree. The bundled template lives in `template/` and is embedded into the
// binary so the package works on a clean machine without any extra files.
//
// Bootstrap is idempotent: re-running it on an already-initialized store is
// a no-op apart from a debug log line. Tests therefore can run it multiple
// times against the same temp dir without juggling state.
package storeinit
