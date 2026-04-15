package homebrew

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
)

func TestBootstrap_BrewPresentReturnsNil(t *testing.T) {
	p := New(&config.Config{})
	original := brewBinaryLookup
	defer func() { brewBinaryLookup = original }()

	brewBinaryLookup = func(string) (string, error) { return "/opt/homebrew/bin/brew", nil }

	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap with brew present should return nil, got %v", err)
	}
}

func TestBootstrap_BrewMissingReturnsStructuredError(t *testing.T) {
	p := New(&config.Config{})
	original := brewBinaryLookup
	defer func() { brewBinaryLookup = original }()

	brewBinaryLookup = func(string) (string, error) {
		return "", &exec.Error{Name: "brew", Err: exec.ErrNotFound}
	}

	err := p.Bootstrap(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Fatalf("error should wrap provider.ErrBootstrapRequired, got %v", err)
	}

	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("error should be unwrappable to *BootstrapRequiredError, got %T", err)
	}
	if brerr.Binary != "brew" {
		t.Errorf("Binary = %q, want 'brew'", brerr.Binary)
	}
	if brerr.Provider != "brew" {
		t.Errorf("Provider = %q, want 'brew'", brerr.Provider)
	}
	if !strings.Contains(brerr.Script, "raw.githubusercontent.com/Homebrew/install") {
		t.Errorf("Script %q should be the manifest-declared install.sh text", brerr.Script)
	}
}

func TestBootstrap_ScriptMatchesManifest(t *testing.T) {
	// The script surfaced in the BootstrapRequiredError MUST be exactly
	// what the manifest declares — otherwise users auditing the error
	// message would see one script but --bootstrap would run another.
	p := New(&config.Config{})
	original := brewBinaryLookup
	defer func() { brewBinaryLookup = original }()

	brewBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected BootstrapRequiredError, got %T", err)
	}

	manifestScript := p.Manifest().DependsOn[0].Script
	if brerr.Script != manifestScript {
		t.Errorf("BootstrapRequiredError.Script does not match manifest: %q vs %q",
			brerr.Script, manifestScript)
	}
}
