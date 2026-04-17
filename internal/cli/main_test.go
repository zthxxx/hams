package cli

import (
	"os"
	"testing"
)

// TestMain wraps every test in the package with a hard-default of
// HAMS_NO_AUTO_INIT=1 so existing tests that exercise routeToProvider /
// runApply / runRefresh do NOT mutate the developer's $HOME on first
// run. Tests that explicitly cover auto-init behavior re-enable it via
// `t.Setenv("HAMS_NO_AUTO_INIT", "")` inside the test body, alongside
// `t.Setenv("HAMS_CONFIG_HOME", t.TempDir())` and
// `t.Setenv("HAMS_DATA_HOME", t.TempDir())` so the auto-init writes
// land in a temp dir.
//
// This pattern keeps the "no host mutation" invariant intact for the
// dozens of pre-existing tests that pre-date the auto-init feature.
func TestMain(m *testing.M) {
	if v := os.Getenv("HAMS_NO_AUTO_INIT"); v == "" {
		if err := os.Setenv("HAMS_NO_AUTO_INIT", "1"); err != nil {
			panic("test setup: " + err.Error())
		}
		defer func() {
			if err := os.Unsetenv("HAMS_NO_AUTO_INIT"); err != nil {
				panic("test teardown: " + err.Error())
			}
		}()
	}
	os.Exit(m.Run())
}
