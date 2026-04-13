//go:build integration

package runner_test

import (
	"context"
	"testing"

	"pgregory.net/rapid"

	"github.com/zthxxx/hams/internal/runner"
)

func TestOSRunner_Run_EchoProperty(t *testing.T) {
	t.Parallel()
	r := runner.DefaultRunner()
	rapid.Check(t, func(t *rapid.T) {
		msg := rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(t, "msg")
		out, err := r.Run(context.Background(), "echo", "-n", msg)
		if err != nil {
			t.Fatalf("echo failed: %v", err)
		}
		if string(out) != msg {
			t.Fatalf("echo output %q != %q", string(out), msg)
		}
	})
}

func TestOSRunner_LookPath_Echo(t *testing.T) {
	t.Parallel()
	r := runner.DefaultRunner()
	path, err := r.LookPath("echo")
	if err != nil {
		t.Fatalf("LookPath(echo) failed: %v", err)
	}
	if path == "" {
		t.Fatal("LookPath(echo) returned empty path")
	}
}
