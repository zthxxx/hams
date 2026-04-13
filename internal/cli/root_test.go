package cli

import (
	"context"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestNewApp_CreatesApp(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry)
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
	if app.Name != "hams" {
		t.Errorf("app.Name = %q, want 'hams'", app.Name)
	}
}

func TestNewApp_VersionFlag(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry)

	err := app.Run(context.Background(), []string{"hams", "--version"})
	if err != nil {
		t.Fatalf("--version error: %v", err)
	}
}

func TestNewApp_HelpFlag(t *testing.T) {
	registry := provider.NewRegistry()
	app := NewApp(registry)

	err := app.Run(context.Background(), []string{"hams", "--help"})
	if err != nil {
		t.Fatalf("--help error: %v", err)
	}
}
