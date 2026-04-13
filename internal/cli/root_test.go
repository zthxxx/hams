package cli

import (
	"bytes"
	"testing"
)

func TestNewRootCmd_VersionFlag(t *testing.T) {
	root, _ := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--version"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("--version error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("--version produced empty output")
	}
}

func TestNewRootCmd_HelpFlag(t *testing.T) {
	root, _ := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--help"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("--help error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("--help produced empty output")
	}
}

func TestNewRootCmd_GlobalFlags(t *testing.T) {
	root, flags := NewRootCmd()
	root.SetArgs([]string{"--debug", "--dry-run", "--json", "--no-color", "--config=/tmp/cfg.yaml", "--store=/tmp/store", "--profile=macOS"})

	// Execute will show help since no subcommand.
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	_ = root.Execute() //nolint:errcheck // help output is expected

	if !flags.Debug {
		t.Error("Debug flag not set")
	}
	if !flags.DryRun {
		t.Error("DryRun flag not set")
	}
	if !flags.JSON {
		t.Error("JSON flag not set")
	}
	if !flags.NoColor {
		t.Error("NoColor flag not set")
	}
	if flags.Config != "/tmp/cfg.yaml" {
		t.Errorf("Config = %q, want /tmp/cfg.yaml", flags.Config)
	}
	if flags.Store != "/tmp/store" {
		t.Errorf("Store = %q, want /tmp/store", flags.Store)
	}
	if flags.Profile != "macOS" {
		t.Errorf("Profile = %q, want macOS", flags.Profile)
	}
}

func TestNewRootCmd_NoArgs_ShowsHelp(t *testing.T) {
	root, _ := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{})

	err := root.Execute()
	if err != nil {
		t.Fatalf("no args error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("no args should show help")
	}
}
