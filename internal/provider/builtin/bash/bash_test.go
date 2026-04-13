package bash

import (
	"context"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

func TestManifest(t *testing.T) {
	p := New()
	m := p.Manifest()
	if m.Name != "bash" {
		t.Errorf("Name = %q, want 'bash'", m.Name)
	}
	if m.ResourceClass != provider.ClassCheckBased {
		t.Errorf("ResourceClass = %d, want ClassCheckBased", m.ResourceClass)
	}
}

func TestBootstrap(t *testing.T) {
	p := New()
	if err := p.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap error: %v", err)
	}
}

func TestApply_SimpleCommand(t *testing.T) {
	p := New()
	action := provider.Action{
		ID:       "test-echo",
		Type:     provider.ActionInstall,
		Resource: "echo hello",
	}

	err := p.Apply(context.Background(), action)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
}

func TestApply_FailingCommand(t *testing.T) {
	p := New()
	action := provider.Action{
		ID:       "test-fail",
		Type:     provider.ActionInstall,
		Resource: "exit 1",
	}

	err := p.Apply(context.Background(), action)
	if err == nil {
		t.Fatal("expected error for failing command")
	}
}

func TestApply_EmptyResource(t *testing.T) {
	p := New()
	action := provider.Action{
		ID:       "test-empty",
		Type:     provider.ActionInstall,
		Resource: "",
	}

	err := p.Apply(context.Background(), action)
	if err == nil {
		t.Fatal("expected error for empty resource")
	}
}

func TestRunCheck_Success(t *testing.T) {
	output, ok := RunCheck(context.Background(), "echo hello")
	if !ok {
		t.Error("RunCheck should succeed for 'echo hello'")
	}
	if output != "hello" {
		t.Errorf("output = %q, want 'hello'", output)
	}
}

func TestRunCheck_Failure(t *testing.T) {
	_, ok := RunCheck(context.Background(), "exit 1")
	if ok {
		t.Error("RunCheck should fail for 'exit 1'")
	}
}

func TestRunCheck_Empty(t *testing.T) {
	_, ok := RunCheck(context.Background(), "")
	if ok {
		t.Error("RunCheck should fail for empty command")
	}
}

func TestRemove_NoOp(t *testing.T) {
	p := New()
	err := p.Remove(context.Background(), "some-script")
	if err != nil {
		t.Fatalf("Remove should be no-op: %v", err)
	}
}

func TestProbe(t *testing.T) {
	p := New()
	sf := state.New("bash", "test")
	sf.SetResource("init-zsh", state.StateOK)
	sf.SetResource("removed-script", state.StateRemoved)

	results, err := p.Probe(context.Background(), sf)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}

	// Should return only non-removed resources.
	if len(results) != 1 {
		t.Fatalf("Probe returned %d results, want 1", len(results))
	}
	if results[0].ID != "init-zsh" {
		t.Errorf("Probe result ID = %q, want 'init-zsh'", results[0].ID)
	}
}
