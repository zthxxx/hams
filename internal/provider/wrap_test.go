package provider

import (
	"testing"
)

func TestInjectFlags_AddsAbsent(t *testing.T) {
	args := []string{"install", "serve"}
	result := injectFlags(args, map[string]string{"--global": ""})

	found := false
	for _, a := range result {
		if a == "--global" {
			found = true
		}
	}
	if !found {
		t.Errorf("injectFlags = %v, want --global injected", result)
	}
}

func TestInjectFlags_SkipsExisting(t *testing.T) {
	args := []string{"install", "--global", "serve"}
	result := injectFlags(args, map[string]string{"--global": ""})

	count := 0
	for _, a := range result {
		if a == "--global" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("--global appears %d times, want 1 (no duplicate injection)", count)
	}
}

func TestInjectFlags_WithValue(t *testing.T) {
	args := []string{"install", "nginx"}
	result := injectFlags(args, map[string]string{"--prefix": "/usr/local"})

	found := false
	for _, a := range result {
		if a == "--prefix=/usr/local" {
			found = true
		}
	}
	if !found {
		t.Errorf("injectFlags = %v, want --prefix=/usr/local", result)
	}
}

func TestInjectFlags_Empty(t *testing.T) {
	args := []string{"install", "htop"}
	result := injectFlags(args, nil)
	if len(result) != 2 {
		t.Errorf("injectFlags with nil = %v, want original args", result)
	}
}

func TestParseVerb_Basic(t *testing.T) {
	verb, remaining := ParseVerb([]string{"install", "htop", "--cask"})
	if verb != "install" {
		t.Errorf("verb = %q, want 'install'", verb)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %v, want 2 items", remaining)
	}
}

func TestParseVerb_NoVerb(t *testing.T) {
	verb, remaining := ParseVerb([]string{"--flag", "-x"})
	if verb != "" {
		t.Errorf("verb = %q, want empty", verb)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining = %v, want 2 items", remaining)
	}
}

func TestParseVerb_Empty(t *testing.T) {
	verb, remaining := ParseVerb(nil)
	if verb != "" {
		t.Errorf("verb = %q, want empty", verb)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining = %v, want empty", remaining)
	}
}
