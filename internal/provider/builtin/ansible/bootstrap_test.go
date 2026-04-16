package ansible

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestBootstrap_AnsiblePresentReturnsNil(t *testing.T) {
	p := New()
	original := ansibleBinaryLookup
	defer func() { ansibleBinaryLookup = original }()

	ansibleBinaryLookup = func(string) (string, error) { return "/usr/local/bin/ansible-playbook", nil }

	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap with ansible-playbook present should return nil, got %v", err)
	}
}

func TestBootstrap_AnsibleMissingReturnsStructuredError(t *testing.T) {
	p := New()
	original := ansibleBinaryLookup
	defer func() { ansibleBinaryLookup = original }()

	ansibleBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected *BootstrapRequiredError, got %T", err)
	}
	if brerr.Binary != "ansible-playbook" {
		t.Errorf("Binary = %q, want 'ansible-playbook'", brerr.Binary)
	}
	if brerr.Script != ansibleInstallScript {
		t.Errorf("Script = %q, want %q", brerr.Script, ansibleInstallScript)
	}
	if !errors.Is(err, provider.ErrBootstrapRequired) {
		t.Errorf("error should wrap ErrBootstrapRequired")
	}
}

func TestBootstrap_ScriptUsesPipxNotPip(t *testing.T) {
	// PEP 668 rejects `pip install` on modern Python installations
	// (Debian 12+, brew-python) with "externally-managed environment".
	// Ansible's install script MUST use pipx to avoid that failure.
	p := New()
	original := ansibleBinaryLookup
	defer func() { ansibleBinaryLookup = original }()
	ansibleBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

	err := p.Bootstrap(context.Background())
	var brerr *provider.BootstrapRequiredError
	if !errors.As(err, &brerr) {
		t.Fatalf("expected BootstrapRequiredError, got %T", err)
	}
	if brerr.Script == "pip install ansible" {
		t.Errorf("Script uses bare `pip install ansible` which hits PEP 668; should use pipx")
	}
	if brerr.Script != "pipx install --include-deps ansible" {
		t.Errorf("Script should be `pipx install --include-deps ansible`, got %q", brerr.Script)
	}
}

func TestBootstrap_ScriptMatchesManifest(t *testing.T) {
	p := New()
	original := ansibleBinaryLookup
	defer func() { ansibleBinaryLookup = original }()
	ansibleBinaryLookup = func(string) (string, error) { return "", exec.ErrNotFound }

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
