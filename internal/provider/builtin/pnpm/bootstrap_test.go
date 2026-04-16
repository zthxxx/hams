package pnpm

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestBootstrap_PnpmPresentReturnsNil(t *testing.T) {
	p := New()
	original := pnpmBinaryLookup
	defer func() { pnpmBinaryLookup = original }()

	pnpmBinaryLookup = func(string) (string, error) { return "/usr/local/bin/pnpm", nil }

	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap with pnpm present should return nil, got %v", err)
	}
}

func TestBootstrap_PnpmMissingReturnsStructuredError(t *testing.T) {
	p := New()
	original := pnpmBinaryLookup
	defer func() { pnpmBinaryLookup = original }()

	pnpmBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected *BootstrapRequiredError, got %T", err)
	}
	if brerr.Binary != "pnpm" {
		t.Errorf("Binary = %q, want 'pnpm'", brerr.Binary)
	}
	if brerr.Script != pnpmInstallScript {
		t.Errorf("Script = %q, want %q", brerr.Script, pnpmInstallScript)
	}
	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Errorf("error should wrap ErrBootstrapRequired")
	}
}

func TestBootstrap_ScriptMatchesManifest(t *testing.T) {
	// The DependsOn[0].Script MUST match the BootstrapRequiredError.Script
	// so that RunBootstrap delegates the same command the user saw in
	// the prompt/error. Mismatch would let --bootstrap execute a
	// different script than the one the user audited.
	p := New()
	original := pnpmBinaryLookup
	defer func() { pnpmBinaryLookup = original }()
	pnpmBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected BootstrapRequiredError, got %T", err)
	}
	if brerr.Script != p.Manifest().DependsOn[0].Script {
		t.Errorf("Script mismatch: error=%q manifest=%q",
			brerr.Script, p.Manifest().DependsOn[0].Script)
	}
}
