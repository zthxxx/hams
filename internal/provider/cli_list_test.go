package provider

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

// fakeListProvider is a minimal Provider fixture that records the
// arguments HandleListCmd passes into List, and returns a canned
// string that the test inspects after stdout capture.
type fakeListProvider struct {
	name       string
	prefix     string
	listOutput string
	listErr    error

	gotHf *hamsfile.File
	gotSf *state.File
}

func (p *fakeListProvider) Manifest() Manifest {
	return Manifest{Name: p.name, FilePrefix: p.prefix}
}

func (p *fakeListProvider) Bootstrap(_ context.Context) error { return nil }

func (p *fakeListProvider) Probe(_ context.Context, _ *state.File) ([]ProbeResult, error) {
	return nil, nil
}

func (p *fakeListProvider) Plan(_ context.Context, _ *hamsfile.File, _ *state.File) ([]Action, error) {
	return nil, nil
}

func (p *fakeListProvider) Apply(_ context.Context, _ Action) error { return nil }

func (p *fakeListProvider) Remove(_ context.Context, _ string) error { return nil }

func (p *fakeListProvider) List(_ context.Context, hf *hamsfile.File, sf *state.File) (string, error) {
	p.gotHf = hf
	p.gotSf = sf
	return p.listOutput, p.listErr
}

// captureHandleListCmd redirects stdout around a HandleListCmd call so
// the test can assert the printed diff. Returns (captured stdout, err).
func captureHandleListCmd(t *testing.T, p *fakeListProvider, cfg *config.Config) (string, error) {
	t.Helper()
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("pipe: %v", pipeErr)
	}
	orig := os.Stdout
	os.Stdout = w

	var ioErr error
	done := make(chan struct{})
	var captured []byte
	go func() {
		captured, ioErr = io.ReadAll(r)
		close(done)
	}()

	err := HandleListCmd(context.Background(), p, cfg)
	if closeErr := w.Close(); closeErr != nil {
		t.Logf("close pipe: %v", closeErr)
	}
	os.Stdout = orig
	<-done
	if ioErr != nil {
		t.Fatalf("read stdout: %v", ioErr)
	}
	return string(captured), err
}

// makeListCfg seeds a temp store/profile/state layout so HandleListCmd
// can resolve paths without touching the host. Prefix-agnostic (the
// helper resolves the filename from Manifest().FilePrefix at call
// time), but every test in this file happens to use "cargo" as the
// representative builtin name.
func makeListCfg(t *testing.T) *config.Config {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	profileDir := filepath.Join(storeDir, "test")
	stateDir := filepath.Join(storeDir, ".state", "m1")
	for _, d := range []string{profileDir, stateDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return &config.Config{StorePath: storeDir, ProfileTag: "test", MachineID: "m1"}
}

// TestHandleListCmd_PrintsListOutput locks in the happy path: hamsfile
// and state both absent → List gets empty-but-valid doubles → output
// is printed verbatim.
func TestHandleListCmd_PrintsListOutput(t *testing.T) {
	p := &fakeListProvider{name: "cargo", prefix: "cargo", listOutput: "[diff goes here]\n"}
	cfg := makeListCfg(t)

	out, err := captureHandleListCmd(t, p, cfg)
	if err != nil {
		t.Fatalf("HandleListCmd: %v", err)
	}
	if out != p.listOutput {
		t.Errorf("stdout = %q, want %q", out, p.listOutput)
	}
}

// TestHandleListCmd_NoStoreReturnsUsageError: empty cfg should short-
// circuit before touching any file. Asserts ExitUsageError so scripts
// get a stable exit code.
func TestHandleListCmd_NoStoreReturnsUsageError(t *testing.T) {
	p := &fakeListProvider{name: "cargo", prefix: "cargo"}
	_, err := captureHandleListCmd(t, p, &config.Config{})
	if err == nil {
		t.Fatal("expected usage error for empty store_path")
	}
	var ufe *hamserr.UserFacingError
	if !errors.As(err, &ufe) || ufe.Code != hamserr.ExitUsageError {
		t.Errorf("want ExitUsageError, got %v (%T)", err, err)
	}
}

// TestHandleListCmd_SeedsEmptyStateWhenAbsent asserts that the helper
// tolerates a missing state file by constructing a fresh in-memory
// state.File rather than erroring. This matches the "list works on a
// fresh store" invariant that cycle 210's FormatDiff hint relies on.
func TestHandleListCmd_SeedsEmptyStateWhenAbsent(t *testing.T) {
	p := &fakeListProvider{name: "cargo", prefix: "cargo", listOutput: "ok"}
	cfg := makeListCfg(t)
	if _, err := captureHandleListCmd(t, p, cfg); err != nil {
		t.Fatalf("HandleListCmd: %v", err)
	}
	if p.gotSf == nil {
		t.Fatal("List was not called")
	}
	if p.gotSf.Provider != "cargo" {
		t.Errorf("gotSf.Provider = %q, want %q", p.gotSf.Provider, "cargo")
	}
	if p.gotSf.MachineID != "m1" {
		t.Errorf("gotSf.MachineID = %q, want %q", p.gotSf.MachineID, "m1")
	}
}

// TestHandleListCmd_ReadsExistingHamsfile: when the hamsfile exists
// with a tracked app, HandleListCmd loads it and passes it into List.
func TestHandleListCmd_ReadsExistingHamsfile(t *testing.T) {
	p := &fakeListProvider{name: "cargo", prefix: "cargo", listOutput: "ok"}
	cfg := makeListCfg(t)
	hfPath := filepath.Join(cfg.ProfileDir(), "cargo.hams.yaml")
	if err := os.WriteFile(hfPath, []byte("cli:\n  - app: ripgrep\n"), 0o600); err != nil {
		t.Fatalf("seed hamsfile: %v", err)
	}
	if _, err := captureHandleListCmd(t, p, cfg); err != nil {
		t.Fatalf("HandleListCmd: %v", err)
	}
	if p.gotHf == nil {
		t.Fatal("List was not called with a hamsfile")
	}
	apps := p.gotHf.ListApps()
	if len(apps) != 1 || apps[0] != "ripgrep" {
		t.Errorf("hamsfile apps = %v, want [ripgrep]", apps)
	}
}

// TestHandleListCmd_PropagatesListError confirms that a provider-level
// List error is returned to the caller rather than swallowed.
func TestHandleListCmd_PropagatesListError(t *testing.T) {
	want := errors.New("list: boom")
	p := &fakeListProvider{name: "cargo", prefix: "cargo", listErr: want}
	cfg := makeListCfg(t)
	_, err := captureHandleListCmd(t, p, cfg)
	if !errors.Is(err, want) && !strings.Contains(fmtErr(err), "boom") {
		t.Errorf("err = %v, want to propagate %v", err, want)
	}
}

// fmtErr formats an error to a string without depending on fmt.Sprintf
// so the assertion stays readable under -race.
func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
