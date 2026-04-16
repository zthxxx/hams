package pnpm

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestBootstrap_PnpmPresentReturnsNil(t *testing.T) {
	p := New(nil, NewRealCmdRunner())
	original := pnpmBinaryLookup
	defer func() { pnpmBinaryLookup = original }()

	pnpmBinaryLookup = func(string) (string, error) { return "/usr/local/bin/pnpm", nil }

	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap with pnpm present should return nil, got %v", err)
	}
}

func TestBootstrap_PnpmMissingReturnsStructuredError(t *testing.T) {
	p := New(nil, NewRealCmdRunner())
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
	// The DependsOn entry with a non-empty Script MUST match the
	// BootstrapRequiredError.Script so RunBootstrap delegates the same
	// command the user saw in the prompt/error. Mismatch would let
	// --bootstrap execute a different script than the one the user
	// audited. The manifest splits DAG ordering (Provider: npm, no
	// Script) from script host (Provider: bash, with Script) into two
	// DependsOn entries; this test finds the scripted one.
	p := New(nil, NewRealCmdRunner())
	original := pnpmBinaryLookup
	defer func() { pnpmBinaryLookup = original }()
	pnpmBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected BootstrapRequiredError, got %T", err)
	}
	var manifestScript string
	var scriptHost string
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
		t.Errorf("script host must be 'bash' (only provider implementing BashScriptRunner), got %q", scriptHost)
	}
}
