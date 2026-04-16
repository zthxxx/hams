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
	// DependsOn is platform-gated to darwin but still listed in the
	// manifest on all runners (matchesPlatform is consulted at
	// RunBootstrap time). The DAG entry (Provider: brew, no Script)
	// and the scripted entry (Provider: bash, with Script) are split;
	// this test finds the scripted one.
	p := New()
	original := dutiBinaryLookup
	defer func() { dutiBinaryLookup = original }()
	dutiBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected BootstrapRequiredError, got %T", err)
	}
	var manifestScript, scriptHost string
	for _, dep := range p.Manifest().DependsOn {
		if dep.Script != "" {
			manifestScript = dep.Script
			scriptHost = dep.Provider
			break
		}
	}
	if brerr.Script != manifestScript {
		t.Errorf("Script mismatch: error=%q manifest=%q", brerr.Script, manifestScript)
	}
	if scriptHost != "bash" {
		t.Errorf("script host must be 'bash', got %q", scriptHost)
	}
}
