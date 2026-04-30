package defaults

import (
	"context"
	"os"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

// TestMain installs a no-op for provider.PassthroughExec for the
// entire defaults test binary. The seam is mutated exactly once,
// before any test runs, so parallel tests can read it freely without
// racing.
//
// Why this exists: HandleCommand's unrecognized-verb branch (e.g.
// `defaults read …`) routes through provider.Passthrough, which calls
// PassthroughExec. Without this stub, every test that hits that path
// would exec the host's real `defaults` binary — and on a developer's
// macOS workstation that mutates the real user-defaults database,
// violating the project's "Isolated Verification" first principle.
// Tests that want to assert on the call swap PassthroughExec inside
// the test body and restore via t.Cleanup.
func TestMain(m *testing.M) {
	provider.PassthroughExec = func(_ context.Context, _ string, _ []string) error {
		return nil
	}
	os.Exit(m.Run())
}
