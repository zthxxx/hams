package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GoBuilder runs `go build` with cross-compilation fixed to linux/<arch>.
type GoBuilder struct {
	Arch       string // "amd64" or "arm64"
	OutputPath string // e.g. "bin/hams-linux-arm64"
	Package    string // e.g. "./cmd/hams"
	RepoRoot   string // working directory; must be absolute
	ExtraEnv   []string

	clock Clock
}

// NewGoBuilder constructs a GoBuilder with a real clock.
func NewGoBuilder(arch, outputPath, pkg, repoRoot string) *GoBuilder {
	return &GoBuilder{
		Arch:       arch,
		OutputPath: outputPath,
		Package:    pkg,
		RepoRoot:   repoRoot,
		clock:      RealClock(),
	}
}

// Build invokes `go build` and returns the outcome.
//
// The short HEAD SHA is resolved before the build and injected via
// -ldflags so the resulting binary's `hams --version` output reflects the
// commit it was built from. GOCACHE-driven compilation is unaffected (only
// the link step sees the new ldflags string), so incremental rebuilds stay
// fast even as HEAD advances.
func (b *GoBuilder) Build(ctx context.Context) BuildResult {
	start := b.clock.Now()
	commit := readCommitSHA(ctx, b.RepoRoot)
	ldflags := fmt.Sprintf("-X github.com/zthxxx/hams/internal/version.commit=%s", commit)
	cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", ldflags, "-o", b.OutputPath, b.Package) //nolint:gosec // go build target args are fixed by the watcher config; commit comes from `git rev-parse`.
	cmd.Dir = b.RepoRoot
	cmd.Env = buildEnv(b.Arch, b.ExtraEnv)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	dur := b.clock.Now().Sub(start)
	return BuildResult{
		Err:       err,
		Stderr:    strings.TrimRight(stderr.String(), "\n"),
		Duration:  dur,
		CommitSHA: commit,
	}
}

// buildEnv returns the environment for a cross-compile `go build` invocation.
//
// The inherited environment is scrubbed of GOOS/GOARCH/CGO_ENABLED to avoid
// the caller's shell leaking in, then reset to the watcher's pinned values.
// GOCACHE is intentionally NOT cleared; the watcher relies on it for
// incremental compilation.
func buildEnv(arch string, extra []string) []string {
	base := os.Environ()
	filtered := base[:0]
	for _, kv := range base {
		switch {
		case strings.HasPrefix(kv, "GOOS="),
			strings.HasPrefix(kv, "GOARCH="),
			strings.HasPrefix(kv, "CGO_ENABLED="):
			// drop
		default:
			filtered = append(filtered, kv)
		}
	}
	filtered = append(filtered,
		"GOOS=linux",
		fmt.Sprintf("GOARCH=%s", arch),
		"CGO_ENABLED=0",
	)
	filtered = append(filtered, extra...)
	return filtered
}

// readCommitSHA returns the short HEAD SHA, or "unknown" on error.
// Errors are swallowed on purpose — a missing/fresh repo should not stop the
// watcher from reporting a build.
func readCommitSHA(ctx context.Context, repoRoot string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

var _ Builder = (*GoBuilder)(nil)

// FormatDuration renders a build duration for logs as "1.23s" or "456ms".
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
