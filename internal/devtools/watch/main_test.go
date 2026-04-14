package main

import (
	"strings"
	"testing"
)

func TestParseFlags_RequiresArch(t *testing.T) {
	_, err := parseFlags(nil)
	if err == nil {
		t.Fatal("expected error when --arch is missing")
	}
	if !strings.Contains(err.Error(), "--arch is required") {
		t.Errorf("err = %v, want --arch required message", err)
	}
}

func TestParseFlags_RejectsUnknownArch(t *testing.T) {
	_, err := parseFlags([]string{"--arch", "riscv64"})
	if err == nil {
		t.Fatal("expected error for unknown arch")
	}
	if !strings.Contains(err.Error(), "riscv64") {
		t.Errorf("err = %v, want arch name in message", err)
	}
}

func TestParseFlags_AcceptsAmd64AndArm64(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		cfg, err := parseFlags([]string{"--arch", arch})
		if err != nil {
			t.Fatalf("unexpected err for arch %s: %v", arch, err)
		}
		if cfg.arch != arch {
			t.Errorf("cfg.arch = %q, want %q", cfg.arch, arch)
		}
		wantOut := "bin/hams-linux-" + arch
		if cfg.output != wantOut {
			t.Errorf("cfg.output = %q, want %q", cfg.output, wantOut)
		}
		if cfg.pkg != "./cmd/hams" {
			t.Errorf("cfg.pkg = %q, want ./cmd/hams", cfg.pkg)
		}
	}
}

func TestValidateArch(t *testing.T) {
	cases := map[string]bool{
		"amd64":  true,
		"arm64":  true,
		"":       false,
		"x86":    false,
		"riscv":  false,
		"arm":    false,
		"AMD64":  false,
		" amd64": false,
	}
	for arch, ok := range cases {
		err := validateArch(arch)
		if ok && err != nil {
			t.Errorf("validateArch(%q) = %v, want nil", arch, err)
		}
		if !ok && err == nil {
			t.Errorf("validateArch(%q) = nil, want err", arch)
		}
	}
}
