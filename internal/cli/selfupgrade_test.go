package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

// TestRunHomebrewUpgrade_DryRun locks in the dry-run preview UX:
// `hams self-upgrade --dry-run` on a Homebrew install MUST print
// "[dry-run] Would run: brew upgrade ..." and exit zero without
// invoking brew. Without this gate, a future refactor that drops
// the dry-run branch would silently run `brew upgrade` on a user
// who only asked for a preview — exactly the "dry-run has side
// effects" anti-pattern that cycles 39, 41, 84, 86 fixed in
// apply.
func TestRunHomebrewUpgrade_DryRun(t *testing.T) {
	flags := &provider.GlobalFlags{DryRun: true}

	// The dry-run branch returns immediately without exec-ing
	// brew. Test just captures stdout and asserts the preview
	// text appears.
	got := captureStdout(t, func() {
		if err := runHomebrewUpgrade(context.Background(), flags); err != nil {
			t.Fatalf("dry-run: %v", err)
		}
	})
	want := "[dry-run] Would run: brew upgrade"
	if !strings.Contains(got, want) {
		t.Errorf("dry-run output missing %q; got:\n%s", want, got)
	}
	// Must NOT contain the live-run confirmation text — that
	// string only prints in the real path.
	if strings.Contains(got, "Detected Homebrew install") {
		t.Errorf("dry-run output should NOT contain live-run text; got:\n%s", got)
	}
}
