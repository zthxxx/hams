package homebrew

import (
	"context"
	"errors"
	"os/exec"
	"slices"
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

// After install.sh runs, the brew binary sits in /opt/homebrew/bin
// (Apple Silicon) or /home/linuxbrew/.linuxbrew/bin (Linuxbrew) — not
// on the hams process's $PATH. Bootstrap MUST augment $PATH and
// re-check, or the --bootstrap flow bails with "still unavailable
// after bootstrap" on every fresh Mac/Linux install.
func TestBootstrap_PathAugmentationAfterInstall(t *testing.T) {
	p := New(&config.Config{})
	origLookup := brewBinaryLookup
	origAugment := envPathAugment
	defer func() {
		brewBinaryLookup = origLookup
		envPathAugment = origAugment
	}()

	var augmentCalls [][]string
	var pathAugmented bool

	brewBinaryLookup = func(string) (string, error) {
		if pathAugmented {
			return "/opt/homebrew/bin/brew", nil
		}
		return "", exec.ErrNotFound
	}
	envPathAugment = func(paths []string) {
		augmentCalls = append(augmentCalls, paths)
		pathAugmented = true
	}

	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap should succeed after PATH augmentation; got %v", err)
	}
	if len(augmentCalls) != 1 {
		t.Fatalf("envPathAugment should be called exactly once; got %d calls", len(augmentCalls))
	}
	if !containsAll(augmentCalls[0], []string{"/opt/homebrew/bin", "/home/linuxbrew/.linuxbrew/bin"}) {
		t.Errorf("augment list should include /opt/homebrew/bin AND /home/linuxbrew/.linuxbrew/bin; got %v", augmentCalls[0])
	}
}

// When PATH augmentation doesn't surface brew either, Bootstrap must
// still return the actionable BootstrapRequiredError (not swallow the
// error silently). This guards the "install.sh succeeded but dropped
// brew somewhere weird" edge case.
func TestBootstrap_PathAugmentationStillMissingSignalsConsent(t *testing.T) {
	p := New(&config.Config{})
	origLookup := brewBinaryLookup
	origAugment := envPathAugment
	defer func() {
		brewBinaryLookup = origLookup
		envPathAugment = origAugment
	}()

	brewBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }
	envPathAugment = func([]string) {}

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected BootstrapRequiredError even after failed augmentation, got %T (%v)", err, err)
	}
}

func containsAll(haystack, needles []string) bool {
	for _, n := range needles {
		if !slices.Contains(haystack, n) {
			return false
		}
	}
	return true
}
