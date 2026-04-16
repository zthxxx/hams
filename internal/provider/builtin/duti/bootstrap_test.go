package duti

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestBootstrap_DutiPresentReturnsNil(t *testing.T) {
	p := New()
	original := dutiBinaryLookup
	defer func() { dutiBinaryLookup = original }()

	dutiBinaryLookup = func(string) (string, error) { return "/usr/local/bin/duti", nil }

	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap with duti present should return nil, got %v", err)
	}
}

func TestBootstrap_DutiMissingReturnsStructuredError(t *testing.T) {
	p := New()
	original := dutiBinaryLookup
	defer func() { dutiBinaryLookup = original }()

	dutiBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected *BootstrapRequiredError, got %T", err)
	}
	if brerr.Binary != "duti" {
		t.Errorf("Binary = %q, want 'duti'", brerr.Binary)
	}
	if brerr.Script != dutiInstallScript {
		t.Errorf("Script = %q, want %q", brerr.Script, dutiInstallScript)
	}
	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Errorf("error should wrap ErrBootstrapRequired")
	}
}

func TestBootstrap_ScriptMatchesManifest(t *testing.T) {
	p := New()
	original := dutiBinaryLookup
	defer func() { dutiBinaryLookup = original }()
	dutiBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected BootstrapRequiredError, got %T", err)
	}
	// DependsOn is platform-gated to darwin; on a non-darwin runner the
	// manifest still lists the entry (matchesPlatform is consulted at
	// RunBootstrap time, not Manifest time). Assert the Script field
	// matches regardless of the running OS.
	if brerr.Script != p.Manifest().DependsOn[0].Script {
		t.Errorf("Script mismatch: error=%q manifest=%q",
			brerr.Script, p.Manifest().DependsOn[0].Script)
	}
}
