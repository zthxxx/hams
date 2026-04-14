//go:build sudo

package apt

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/sudo"
)

// These tests run inside Docker containers (Debian) where apt-get and sudo are available.
// Build tag "sudo" ensures they never run in normal `go test ./...`.

func TestApt_ApplyWithRealBuilder_AsRoot(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("apt tests require Linux")
	}
	if os.Getuid() != 0 {
		t.Skip("test requires root")
	}

	p := New(&sudo.Builder{})
	action := provider.Action{
		ID:   "hello",
		Type: provider.ActionInstall,
	}

	// As root, Builder skips sudo and runs apt-get directly.
	// Installing a small package validates the full Apply path.
	err := p.Apply(context.Background(), action)
	if err != nil {
		t.Fatalf("Apply(hello) as root failed: %v", err)
	}
}

func TestApt_RemoveWithRealBuilder_AsRoot(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("apt tests require Linux")
	}
	if os.Getuid() != 0 {
		t.Skip("test requires root")
	}

	p := New(&sudo.Builder{})

	// Remove the package installed in the previous test.
	err := p.Remove(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Remove(hello) as root failed: %v", err)
	}
}
