package cli

import (
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

// TestPrintDryRunActions_SingularResource locks in cycle 125's
// grammar fix: dry-run text output uses "1 resource" (singular)
// when only one skip is reported. Without pluralize, the prior
// output said "1 resources already at desired state" which reads
// as broken English in the common single-app case.
func TestPrintDryRunActions_SingularResource(t *testing.T) {
	// NOT Parallel: captureStdout swaps the global os.Stdout.
	actions := []provider.Action{
		{ID: "htop", Type: provider.ActionSkip},
	}
	out := captureStdout(t, func() {
		printDryRunActions("apt", "apt", actions)
	})
	if !strings.Contains(out, "1 resource already at desired state") {
		t.Errorf("expected singular `resource`, got:\n%s", out)
	}
	// Defense-in-depth: the broken phrasing must NOT appear.
	if strings.Contains(out, "1 resources already at desired state") {
		t.Errorf("should NOT use plural `resources` for count=1, got:\n%s", out)
	}
}

// TestPrintDryRunActions_PluralResources asserts the plural form
// kicks in correctly for count > 1.
func TestPrintDryRunActions_PluralResources(t *testing.T) {
	// NOT Parallel: captureStdout swaps the global os.Stdout.
	actions := []provider.Action{
		{ID: "htop", Type: provider.ActionSkip},
		{ID: "jq", Type: provider.ActionSkip},
	}
	out := captureStdout(t, func() {
		printDryRunActions("apt", "apt", actions)
	})
	if !strings.Contains(out, "2 resources already at desired state") {
		t.Errorf("expected plural `resources` for count=2, got:\n%s", out)
	}
}

// TestPrintDryRunActions_SingularUnchanged locks in the same
// grammar fix for the "(N resources unchanged)" line that prints
// alongside install/update/remove actions.
func TestPrintDryRunActions_SingularUnchanged(t *testing.T) {
	// NOT Parallel: captureStdout swaps the global os.Stdout.
	actions := []provider.Action{
		{ID: "htop", Type: provider.ActionInstall},
		{ID: "jq", Type: provider.ActionSkip},
	}
	out := captureStdout(t, func() {
		printDryRunActions("apt", "apt", actions)
	})
	if !strings.Contains(out, "(1 resource unchanged)") {
		t.Errorf("expected `(1 resource unchanged)`, got:\n%s", out)
	}
}
