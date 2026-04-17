package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

func TestNewApp_CreatesApp(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
	if app.Name != "hams" {
		t.Errorf("app.Name = %q, want 'hams'", app.Name)
	}
}

func TestNewApp_VersionFlag(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "--version"})
	if err != nil {
		t.Fatalf("--version error: %v", err)
	}
}

func TestNewApp_HelpFlag(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "--help"})
	if err != nil {
		t.Fatalf("--help error: %v", err)
	}
}

// TestNewApp_DebugFlagRaisesSlogLevel — cycle 243 guard. The root
// Before hook calls logging.SetupDebugOnly(true) when --debug is set,
// so every command (not just per-provider CLI dispatch from cycle 242)
// surfaces debug-level slog output. Asserts:
//
//  1. `hams --debug version` (a top-level non-provider command) raises
//     the global slog level so a subsequent slog.Debug call would emit.
//  2. `hams version` (no --debug) leaves the level unchanged so
//     slog.Debug calls stay suppressed.
//
// We can't easily intercept Before's internal slog.SetDefault from a
// black-box test, but we CAN observe the side effect: after Run
// returns, slog.Default() reflects whichever handler was installed
// last. Compare slog.Default()'s level via the Enabled API.
func TestNewApp_DebugFlagRaisesSlogLevel(t *testing.T) {
	// Save and restore the global default logger so this test doesn't
	// leak slog state into siblings.
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })

	// Install a known-low-level baseline handler (LevelInfo). The
	// Before hook should overwrite it with LevelDebug when --debug.
	baseline := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(baseline))

	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	// Sanity: baseline rejects Debug.
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("test setup: baseline handler should reject Debug")
	}

	// `hams --debug version` should raise the level so Debug is enabled.
	if err := app.Run(context.Background(), []string{"hams", "--debug", "version"}); err != nil {
		t.Fatalf("hams --debug version: %v", err)
	}
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("after hams --debug, slog.Default should accept Debug")
	}

	// Reset baseline. `hams version` (no --debug) should leave the
	// caller's handler alone — Before only fires when --debug is set,
	// so the baseline-installed Info handler stays as Default.
	slog.SetDefault(slog.New(baseline))
	if err := app.Run(context.Background(), []string{"hams", "version"}); err != nil {
		t.Fatalf("hams version: %v", err)
	}
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("after hams version (no --debug), Default should NOT accept Debug — but it does, suggesting Before fired unconditionally")
	}
}

// TestNewApp_VersionSubcommandAvailable asserts `hams version`
// routes to a dedicated subcommand (distinct from --version).
// Surfaces the detailed build info that --version omits.
func TestNewApp_VersionSubcommandAvailable(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	if err := app.Run(context.Background(), []string{"hams", "version"}); err != nil {
		t.Fatalf("`hams version` error: %v", err)
	}
}

// TestVersion_JSONOutputProducesParseableObject locks in cycle 181:
// `hams --json version` previously printed the same text as the
// non-JSON path, ignoring the global --json flag. CI scripts and
// bug-report templates that machine-extract the running version
// need a parseable shape — text form `hams 1.0.0 (abc123) built …`
// is awkward to regex-parse and brittle.
func TestVersion_JSONOutputProducesParseableObject(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	out := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"hams", "--json", "version"}); err != nil {
			t.Fatalf("hams --json version: %v", err)
		}
	})

	// Must be valid JSON.
	var data map[string]string
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %q", err, out)
	}

	// Required fields.
	for _, key := range []string{"version", "commit", "date", "goos", "goarch"} {
		if _, ok := data[key]; !ok {
			t.Errorf("JSON missing required key %q; got: %v", key, data)
		}
	}

	// Sanity-check goos / goarch reflect the running platform.
	if data["goos"] == "" || data["goarch"] == "" {
		t.Errorf("goos/goarch should be non-empty; got goos=%q goarch=%q", data["goos"], data["goarch"])
	}
}

// TestProviderUsageDescription_NonPackageProvidersHaveSpecificNouns asserts
// each non-package provider maps to its correct verb/noun, so `hams --help`
// no longer advertises git-config, defaults, etc. as managing "packages".
func TestProviderUsageDescription_NonPackageProvidersHaveSpecificNouns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, displayName, wantSub string
	}{
		{"git-config", "git-config", "git config entries"},
		{"git-clone", "git-clone", "cloned git repositories"},
		{"defaults", "defaults", "macOS defaults"},
		{"duti", "duti", "default-app associations"},
		{"bash", "bash", "bash provisioning"},
		{"ansible", "ansible", "Ansible playbooks"},
		{"code-ext", "code-ext", "VS Code extensions"},
	}
	for _, tc := range cases {
		got := providerUsageDescription(tc.name, tc.displayName)
		if !strings.Contains(got, tc.wantSub) {
			t.Errorf("%s: got %q, want substring %q", tc.name, got, tc.wantSub)
		}
		if strings.Contains(got, "packages") {
			t.Errorf("%s: non-package provider should not say 'packages', got %q", tc.name, got)
		}
	}
}

// TestProviderUsageDescription_PackageProvidersUsePackageTemplate asserts
// the fallback for actual package-class providers still says "Manage X packages".
func TestProviderUsageDescription_PackageProvidersUsePackageTemplate(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, displayName string }{
		{"brew", "Homebrew"},
		{"apt", "apt"},
		{"pnpm", "pnpm"},
		{"npm", "npm"},
		{"uv", "uv"},
		{"goinstall", "goinstall"},
		{"cargo", "cargo"},
		{"mas", "mas"},
	}
	for _, tc := range cases {
		got := providerUsageDescription(tc.name, tc.displayName)
		wantSub := "Manage " + tc.displayName + " packages"
		if got != wantSub {
			t.Errorf("%s: got %q, want %q", tc.name, got, wantSub)
		}
	}
}

// TestProviderUsageDescription_UnknownProviderFallsBack asserts future
// external plugins get the package-class default rather than an empty string.
func TestProviderUsageDescription_UnknownProviderFallsBack(t *testing.T) {
	t.Parallel()
	got := providerUsageDescription("future-external", "future-external")
	if got == "" {
		t.Error("unknown provider must not return empty usage")
	}
	if !strings.Contains(got, "future-external") {
		t.Errorf("fallback should contain display name, got %q", got)
	}
}

// TestNewApp_UnknownCommandReturnsUsageError asserts that typing a
// non-existent subcommand (e.g., `hams bogus-command` or `hams aply`)
// returns a UserFacingError{Code: ExitUsageError} instead of
// silently printing the help text with exit 0. Scripts need to be
// able to detect typos; users need to be pointed at `--help`.
func TestNewApp_UnknownCommandReturnsUsageError(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "bogus-command"})
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error message should mention 'unknown command', got: %v", err)
	}
	if !strings.Contains(err.Error(), "bogus-command") {
		t.Errorf("error message should name the typo'd command, got: %v", err)
	}
}

// TestNewApp_UnknownCommandSuggestsClosestMatch asserts that
// `hams aply` (typo of `apply`) surfaces a "Did you mean 'hams
// apply'?" suggestion via urfave/cli's Jaro-Winkler suggester.
// Without this the user sees only the bare "unknown command"
// error and has to re-read the command list.
func TestNewApp_UnknownCommandSuggestsClosestMatch(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "aply"})
	if err == nil {
		t.Fatal("expected error for typo'd command, got nil")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("error should be *UserFacingError, got %T: %v", err, err)
	}
	joined := strings.Join(ufe.Suggestions, " | ")
	if !strings.Contains(joined, "apply") {
		t.Errorf("suggestions should mention 'apply' for typo 'aply', got: %v", ufe.Suggestions)
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("exit code = %d, want ExitUsageError (%d)", ufe.Code, hamserr.ExitUsageError)
	}
}

// TestNewApp_ApplyRejectsPositionalArgs asserts that `hams apply
// bogus-arg` returns a UserFacingError instead of silently
// ignoring the stray arg. Common typo: `hams apply apt` meaning
// `hams apply --only=apt`. Without this guard the apt filter is
// silently dropped and apply runs for everything, which the user
// could miss until drift appears.
func TestNewApp_ApplyRejectsPositionalArgs(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "apply", "bogus"})
	if err == nil {
		t.Fatal("expected error for positional arg")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) {
		t.Fatalf("error should be *UserFacingError, got %T: %v", err, err)
	}
	if !strings.Contains(ufe.Error(), "positional") {
		t.Errorf("error should mention 'positional', got: %v", ufe.Error())
	}
	if !strings.Contains(ufe.Error(), "bogus") {
		t.Errorf("error should name the bad arg, got: %v", ufe.Error())
	}
	if ufe.Code != hamserr.ExitUsageError {
		t.Errorf("exit code = %d, want ExitUsageError", ufe.Code)
	}
}

// TestNewApp_RefreshRejectsPositionalArgs mirrors the apply check
// for refresh — same class of typo.
func TestNewApp_RefreshRejectsPositionalArgs(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "refresh", "foo"})
	if err == nil {
		t.Fatal("expected error for positional arg")
	}
	if !strings.Contains(err.Error(), "positional") {
		t.Errorf("error should mention 'positional', got: %v", err)
	}
}

// TestNewApp_ListRejectsPositionalArgs mirrors the apply check
// for list — same class of typo.
func TestNewApp_ListRejectsPositionalArgs(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "list", "bar"})
	if err == nil {
		t.Fatal("expected error for positional arg")
	}
	if !strings.Contains(err.Error(), "positional") {
		t.Errorf("error should mention 'positional', got: %v", err)
	}
}

// TestNewApp_NoArgsShowsHelpNotError asserts that bare `hams` (no
// subcommand) still prints the help text and exits 0, preserving
// the prior behavior for the empty-args path — the usage-error
// fix is SCOPED to "args given but no subcommand matched".
func TestNewApp_NoArgsShowsHelpNotError(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	if err := app.Run(context.Background(), []string{"hams"}); err != nil {
		t.Errorf("bare 'hams' should not error, got: %v", err)
	}
}

// TestNewApp_ProviderCommandsAreSorted asserts that provider subcommands
// appear in alphabetical order regardless of Go map iteration randomness —
// so `hams --help` produces reproducible output across runs.
func TestNewApp_ProviderCommandsAreSorted(t *testing.T) {
	// Save and restore registry to avoid cross-test contamination.
	orig := providerRegistry
	t.Cleanup(func() { providerRegistry = orig })

	providerRegistry = map[string]ProviderHandler{
		"zeta":  &mockProvider{name: "zeta", displayName: "Zeta"},
		"alpha": &mockProvider{name: "alpha", displayName: "Alpha"},
		"mango": &mockProvider{name: "mango", displayName: "Mango"},
		"beta":  &mockProvider{name: "beta", displayName: "Beta"},
	}

	// Run NewApp many times; provider ordering MUST stay identical.
	var firstOrder []string
	for i := range 20 {
		app := NewApp(provider.NewRegistry(), sudo.NoopAcquirer{})
		var order []string
		for _, c := range app.Commands {
			switch c.Name {
			case "zeta", "alpha", "mango", "beta":
				order = append(order, c.Name)
			}
		}
		if i == 0 {
			firstOrder = order
			want := []string{"alpha", "beta", "mango", "zeta"}
			for j, w := range want {
				if j >= len(order) || order[j] != w {
					t.Fatalf("expected sorted providers %v, got %v", want, order)
				}
			}
		} else {
			for j, name := range order {
				if j >= len(firstOrder) || firstOrder[j] != name {
					t.Fatalf("iteration %d: order changed; was %v, now %v", i, firstOrder, order)
				}
			}
		}
	}
}

// TestResolvePaths_TildeExpansionForConfig locks in cycle 89: when
// the user types `hams --config=~/my.yaml`, shell leaves `~/` as a
// literal (bash only tilde-expands `~/...` at the start of a
// separate argument). hams MUST expand it itself — otherwise
// `paths.ConfigFilePath` stores `~/my.yaml`, which never matches
// the real file on disk and every config read silently falls back
// to defaults.
func TestResolvePaths_TildeExpansionForConfig(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	flags := &provider.GlobalFlags{Config: "~/my-config.yaml"}
	paths := resolvePaths(flags)

	wantConfigFile := filepath.Join(fakeHome, "my-config.yaml")
	if paths.ConfigFilePath != wantConfigFile {
		t.Errorf("ConfigFilePath = %q, want %q", paths.ConfigFilePath, wantConfigFile)
	}
	if flags.Config != wantConfigFile {
		t.Errorf("flags.Config after resolvePaths = %q, want %q (callers reading flags.Config elsewhere need the expanded value)", flags.Config, wantConfigFile)
	}
	if paths.ConfigHome != fakeHome {
		t.Errorf("ConfigHome = %q, want %q", paths.ConfigHome, fakeHome)
	}
}

// TestResolvePaths_TildeExpansionForStore locks the same invariant
// for --store. Without it, `hams --store=~/my-store apply` would
// miss the actual store on disk and silently fall through to "no
// store directory configured".
func TestResolvePaths_TildeExpansionForStore(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	flags := &provider.GlobalFlags{Store: "~/my-store"}
	_ = resolvePaths(flags)

	wantStore := filepath.Join(fakeHome, "my-store")
	if flags.Store != wantStore {
		t.Errorf("flags.Store after resolvePaths = %q, want %q", flags.Store, wantStore)
	}
}

// TestHasJSONFlag_AllForms locks in cycle 94: urfave/cli accepts
// all of `--json`, `--json=true`, `--json=false`, `--json=1`, and
// `--json=0` for BoolFlag — but the top-level Execute error path
// scans os.Args directly (urfave's parsed value isn't reachable
// there), so the scan must handle each form. Previously only bare
// `--json` matched; `hams --json=true apply` silently produced text
// output instead of JSON even though the user explicitly opted in.
func TestHasJSONFlag_AllForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no flag", []string{"hams", "apply"}, false},
		{"bare --json", []string{"hams", "--json", "apply"}, true},
		{"--json=true", []string{"hams", "--json=true", "apply"}, true},
		{"--json=1", []string{"hams", "--json=1", "apply"}, true},
		{"--json=false", []string{"hams", "--json=false", "apply"}, false},
		{"--json=0", []string{"hams", "--json=0", "apply"}, false},
		{"--json then --json=false (last wins)", []string{"hams", "--json", "--json=false", "apply"}, false},
		{"--json=false then --json (last wins)", []string{"hams", "--json=false", "--json", "apply"}, true},
		{"embedded in other args does NOT match", []string{"hams", "apply", "--jsonx", "something"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hasJSONFlag(tc.args); got != tc.want {
				t.Errorf("hasJSONFlag(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// TestResolvePaths_AbsolutePathsUnchanged asserts the expansion is
// a no-op for paths that don't start with `~/` (absolute or relative
// without tilde prefix).
func TestResolvePaths_AbsolutePathsUnchanged(t *testing.T) {
	flags := &provider.GlobalFlags{
		Config: "/abs/path/config.yaml",
		Store:  "/abs/path/store",
	}
	paths := resolvePaths(flags)

	if flags.Config != "/abs/path/config.yaml" {
		t.Errorf("Config changed unexpectedly: %q", flags.Config)
	}
	if flags.Store != "/abs/path/store" {
		t.Errorf("Store changed unexpectedly: %q", flags.Store)
	}
	if paths.ConfigFilePath != "/abs/path/config.yaml" {
		t.Errorf("ConfigFilePath = %q", paths.ConfigFilePath)
	}
}
