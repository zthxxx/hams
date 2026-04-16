package cli

import (
	"context"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestStripGlobalFlags_DryRun(t *testing.T) {
	flags := &provider.GlobalFlags{}
	cleaned := stripGlobalFlags([]string{"install", "htop", "--dry-run"}, flags)
	if !flags.DryRun {
		t.Error("DryRun should be set")
	}
	if len(cleaned) != 2 || cleaned[0] != "install" || cleaned[1] != "htop" {
		t.Errorf("cleaned = %v, want [install htop]", cleaned)
	}
}

func TestStripGlobalFlags_ConfigWithValue(t *testing.T) {
	flags := &provider.GlobalFlags{}
	cleaned := stripGlobalFlags([]string{"install", "--config", "/tmp/cfg.yaml", "htop"}, flags)
	if flags.Config != "/tmp/cfg.yaml" {
		t.Errorf("Config = %q, want /tmp/cfg.yaml", flags.Config)
	}
	if len(cleaned) != 2 || cleaned[0] != "install" || cleaned[1] != "htop" {
		t.Errorf("cleaned = %v, want [install htop]", cleaned)
	}
}

func TestStripGlobalFlags_ConfigEquals(t *testing.T) {
	flags := &provider.GlobalFlags{}
	cleaned := stripGlobalFlags([]string{"install", "--config=/tmp/cfg.yaml", "htop"}, flags)
	if flags.Config != "/tmp/cfg.yaml" {
		t.Errorf("Config = %q, want /tmp/cfg.yaml", flags.Config)
	}
	if len(cleaned) != 2 {
		t.Errorf("cleaned = %v, want 2 items", cleaned)
	}
}

// Mock provider for testing.
type mockProvider struct {
	name          string
	displayName   string
	lastCtx       context.Context
	lastArgs      []string
	lastHamsFlags map[string]string
	lastFlags     *provider.GlobalFlags
}

func (m *mockProvider) Name() string        { return m.name }
func (m *mockProvider) DisplayName() string { return m.displayName }
func (m *mockProvider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	m.lastCtx = ctx
	m.lastArgs = args
	m.lastHamsFlags = hamsFlags
	m.lastFlags = flags
	return nil
}

func TestParseProviderArgs_PreservesSeparator(t *testing.T) {
	flags := &provider.GlobalFlags{}
	_, passthrough := parseProviderArgs([]string{"run", "--", "-v", "--flag"}, flags)

	// The "--" must be preserved so the underlying CLI receives it.
	expected := []string{"run", "--", "-v", "--flag"}
	if len(passthrough) != len(expected) {
		t.Fatalf("passthrough = %v, want %v", passthrough, expected)
	}
	for i, want := range expected {
		if passthrough[i] != want {
			t.Errorf("passthrough[%d] = %q, want %q", i, passthrough[i], want)
		}
	}
}

func TestParseProviderArgs_HamsFlagsBeforeSeparator(t *testing.T) {
	flags := &provider.GlobalFlags{}
	hamsFlags, passthrough := parseProviderArgs([]string{"run", "--hams-local", "--", "--hams-tag=foo"}, flags)

	// --hams-local before -- should be captured.
	if _, ok := hamsFlags["local"]; !ok {
		t.Error("hams[local] should exist")
	}
	// --hams-tag=foo after -- should be in passthrough, not captured.
	if _, ok := hamsFlags["tag"]; ok {
		t.Error("hams[tag] should not exist (after --)")
	}
	// passthrough should be: run, --, --hams-tag=foo
	if len(passthrough) != 3 || passthrough[0] != "run" || passthrough[1] != "--" || passthrough[2] != "--hams-tag=foo" {
		t.Errorf("passthrough = %v, want [run -- --hams-tag=foo]", passthrough)
	}
}

func TestRouteToProvider_HelpIntercept(t *testing.T) {
	mock := &mockProvider{name: "brew", displayName: "Homebrew"}

	err := routeToProvider(context.Background(), mock, []string{"install", "--help"}, &provider.GlobalFlags{})
	if err != nil {
		t.Fatalf("routeToProvider --help error: %v", err)
	}

	if mock.lastArgs != nil {
		t.Error("provider should not receive args when --help is present")
	}
}

// TestRouteToProvider_ContextForwarded asserts the caller's context.Context
// (carrying signal cancellation from urfave/cli) reaches the provider
// handler. Previously routeToProvider dropped it in favor of context.TODO(),
// breaking Ctrl+C propagation to long-running provider commands.
func TestRouteToProvider_ContextForwarded(t *testing.T) {
	mock := &mockProvider{name: "brew", displayName: "Homebrew"}
	type ctxKey string
	const sentinelKey ctxKey = "sentinel"
	ctx := context.WithValue(context.Background(), sentinelKey, "marker")

	if err := routeToProvider(ctx, mock, []string{"install", "htop"}, &provider.GlobalFlags{}); err != nil {
		t.Fatalf("routeToProvider: %v", err)
	}
	if mock.lastCtx == nil {
		t.Fatal("provider did not receive a context")
	}
	got, ok := mock.lastCtx.Value(sentinelKey).(string)
	if !ok || got != "marker" {
		t.Errorf("context not forwarded; got value %q (ok=%v), want %q", got, ok, "marker")
	}
}

// TestParseProviderArgs_BoolFlagEqualsForm locks in cycle 95: the
// five BoolFlag forms urfave/cli accepts (bare, =true, =1, =false,
// =0) must all be recognized by parseProviderArgs so they're stripped
// before passthrough to the wrapped CLI. Previously only the bare
// form matched, so `hams apt --json=true install foo` leaked
// `--json=true` to apt-get which rejected it with "option --json=true
// is not understood".
func TestParseProviderArgs_BoolFlagEqualsForm(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		input      []string
		wantJSON   bool
		wantDebug  bool
		wantDryRun bool
		wantPass   []string
	}{
		{
			name:     "bare --json",
			input:    []string{"--json", "install", "foo"},
			wantJSON: true,
			wantPass: []string{"install", "foo"},
		},
		{
			name:     "--json=true",
			input:    []string{"--json=true", "install", "foo"},
			wantJSON: true,
			wantPass: []string{"install", "foo"},
		},
		{
			name:     "--json=1",
			input:    []string{"--json=1", "install", "foo"},
			wantJSON: true,
			wantPass: []string{"install", "foo"},
		},
		{
			name:     "--json=false is consumed, jsonMode stays false",
			input:    []string{"--json=false", "install", "foo"},
			wantJSON: false,
			wantPass: []string{"install", "foo"},
		},
		{
			name:      "--debug=true",
			input:     []string{"--debug=true", "list"},
			wantDebug: true,
			wantPass:  []string{"list"},
		},
		{
			name:       "--dry-run=true with other flags",
			input:      []string{"--dry-run=true", "--json", "install", "foo"},
			wantDryRun: true,
			wantJSON:   true,
			wantPass:   []string{"install", "foo"},
		},
		{
			name:     "unknown --flag=value stays in passthrough",
			input:    []string{"--custom=x", "install", "foo"},
			wantPass: []string{"--custom=x", "install", "foo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			flags := &provider.GlobalFlags{}
			_, pass := parseProviderArgs(tc.input, flags)
			if flags.JSON != tc.wantJSON {
				t.Errorf("JSON = %v, want %v", flags.JSON, tc.wantJSON)
			}
			if flags.Debug != tc.wantDebug {
				t.Errorf("Debug = %v, want %v", flags.Debug, tc.wantDebug)
			}
			if flags.DryRun != tc.wantDryRun {
				t.Errorf("DryRun = %v, want %v", flags.DryRun, tc.wantDryRun)
			}
			if len(pass) != len(tc.wantPass) {
				t.Errorf("passthrough = %v (len %d), want %v", pass, len(pass), tc.wantPass)
				return
			}
			for i, want := range tc.wantPass {
				if pass[i] != want {
					t.Errorf("passthrough[%d] = %q, want %q", i, pass[i], want)
				}
			}
		})
	}
}
