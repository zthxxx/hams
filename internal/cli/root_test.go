package cli

import (
	"context"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

func TestNewApp_CreatesApp(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
	if app.Name != "hams" {
		t.Errorf("app.Name = %q, want 'hams'", app.Name)
	}
}

func TestNewApp_VersionFlag(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "--version"})
	if err != nil {
		t.Fatalf("--version error: %v", err)
	}
}

func TestNewApp_HelpFlag(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry, sudo.NoopAcquirer{})

	err := app.Run(context.Background(), []string{"hams", "--help"})
	if err != nil {
		t.Fatalf("--help error: %v", err)
	}
}
