package mas

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestBootstrap_MasPresentReturnsNil(t *testing.T) {
	p := New(NewRealCmdRunner())
	original := masBinaryLookup
	defer func() { masBinaryLookup = original }()

	masBinaryLookup = func(string) (string, error) { return "/usr/local/bin/mas", nil }

	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap with mas present should return nil, got %v", err)
	}
}

func TestBootstrap_MasMissingReturnsStructuredError(t *testing.T) {
	p := New(NewRealCmdRunner())
	original := masBinaryLookup
	defer func() { masBinaryLookup = original }()

	masBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected *BootstrapRequiredError, got %T", err)
	}
	if brerr.Binary != "mas" {
		t.Errorf("Binary = %q, want 'mas'", brerr.Binary)
	}
	if brerr.Script != masInstallScript {
		t.Errorf("Script = %q, want %q", brerr.Script, masInstallScript)
	}
	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Errorf("error should wrap ErrBootstrapRequired")
	}
}

func TestBootstrap_ScriptMatchesManifest(t *testing.T) {
	// DependsOn splits DAG ordering (Provider: brew, no Script) from
	// script host (Provider: bash, with Script). Only bash implements
	// BashScriptRunner, so the scripted entry's host must be bash.
	p := New(NewRealCmdRunner())
	original := masBinaryLookup
	defer func() { masBinaryLookup = original }()
	masBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

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
