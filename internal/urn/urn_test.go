package urn

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestParse_ValidURN(t *testing.T) {
	tests := []struct {
		input      string
		provider   string
		resourceID string
	}{
		{"urn:hams:defaults:com.apple.dock.autohide", "defaults", "com.apple.dock.autohide"},
		{"urn:hams:bash:init-zsh", "bash", "init-zsh"},
		{"urn:hams:git:global.user.name", "git", "global.user.name"},
		{"urn:hams:duti:public.source-code.vscode", "duti", "public.source-code.vscode"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			u, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if u.Provider != tt.provider {
				t.Errorf("Provider = %q, want %q", u.Provider, tt.provider)
			}
			if u.ResourceID != tt.resourceID {
				t.Errorf("ResourceID = %q, want %q", u.ResourceID, tt.resourceID)
			}
		})
	}
}

func TestParse_InvalidURN(t *testing.T) {
	tests := []struct {
		input   string
		wantErr string
	}{
		{"", "empty string"},
		{"foo", "expected 4"},
		{"urn:hams:bash", "expected 4"},
		{"xyz:hams:bash:script", "must start with 'urn:'"},
		{"urn:nix:bash:script", "namespace must be 'hams'"},
		{"urn:hams::script", "provider must not be empty"},
		{"urn:hams:bash:", "resource-id must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := Parse(tt.input)
			if err == nil {
				t.Fatalf("Parse(%q) expected error, got nil", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse(%q) error = %q, want to contain %q", tt.input, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParse_ProviderLowercase(t *testing.T) {
	u, err := Parse("urn:hams:Homebrew:htop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Provider != "homebrew" {
		t.Errorf("Provider = %q, want 'homebrew' (lowercased)", u.Provider)
	}
}

func TestString_Roundtrip(t *testing.T) {
	input := "urn:hams:defaults:com.apple.dock.autohide"
	u, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if got := u.String(); got != input {
		t.Errorf("String() = %q, want %q", got, input)
	}
}

func TestNew_Valid(t *testing.T) {
	u, err := New("bash", "init-zsh")
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	if u.Provider != "bash" || u.ResourceID != "init-zsh" {
		t.Errorf("New = %+v, want {bash, init-zsh}", u)
	}
}

func TestNew_InvalidEmpty(t *testing.T) {
	if _, err := New("", "script"); err == nil {
		t.Error("New('', 'script') expected error")
	}
	if _, err := New("bash", ""); err == nil {
		t.Error("New('bash', '') expected error")
	}
}

func TestNew_ResourceIDNoColons(t *testing.T) {
	_, err := New("bash", "foo:bar")
	if err == nil {
		t.Fatal("New('bash', 'foo:bar') expected error for colon in resource-id")
	}
}

func TestIsValid(t *testing.T) {
	valid := URN{Provider: "bash", ResourceID: "init"}
	if !valid.IsValid() {
		t.Error("expected valid URN")
	}

	invalid := URN{}
	if invalid.IsValid() {
		t.Error("expected invalid URN for zero value")
	}
}

// Property-based tests.
func TestProperty_ParseRoundtrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.StringMatching(`[a-z][a-z0-9\-]{0,20}`).Draw(t, "provider")
		resourceID := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9.\-_]{0,50}`).Draw(t, "resourceID")

		u, err := New(provider, resourceID)
		if err != nil {
			t.Fatalf("New(%q, %q) error: %v", provider, resourceID, err)
		}

		s := u.String()
		parsed, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", s, err)
		}

		if parsed.Provider != u.Provider {
			t.Errorf("roundtrip Provider: got %q, want %q", parsed.Provider, u.Provider)
		}
		if parsed.ResourceID != u.ResourceID {
			t.Errorf("roundtrip ResourceID: got %q, want %q", parsed.ResourceID, u.ResourceID)
		}
	})
}

func TestProperty_ParseRejectsNoColonPrefix(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate strings that don't start with "urn:"
		s := rapid.StringMatching(`[a-z]{1,5}:[a-z]+:[a-z]+:[a-z]+`).Draw(t, "input")
		if strings.HasPrefix(s, "urn:hams:") {
			t.Skip("generated valid URN prefix, skip")
		}

		_, err := Parse(s)
		if err == nil {
			t.Errorf("Parse(%q) expected error for non-urn prefix", s)
		}
	})
}
