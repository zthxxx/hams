// Command watch is the hams dev-sandbox hot-reload file watcher.
//
// It watches the hams source tree recursively via fsnotify, debounces bursts
// of file-change events, coalesces concurrent saves into at most one pending
// rebuild, and invokes `go build` with a fixed cross-compilation target so
// the resulting binary can be executed inside a Linux sandbox container on
// a macOS or Linux host.
//
// Run it from the project root:
//
//	go run ./internal/devtools/watch --arch arm64
//
// It is not part of the hams runtime.
package main
