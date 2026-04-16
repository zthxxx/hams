package defaults

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "defaults" {
		t.Errorf("Name = %q", m.Name)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformDarwin {
		t.Errorf("Platforms = %v, want [darwin]", m.Platforms)
	}
	if m.ResourceClass != provider.ClassKVConfig {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestParseDomainKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input      string
		wantDomain string
		wantKey    string
	}{
		{"com.apple.dock.autohide=bool:true", "com.apple.dock", "autohide"},
		{"com.apple.dock.autohide", "com.apple.dock", "autohide"},
		{"NSGlobalDomain.AppleShowAllExtensions", "NSGlobalDomain", "AppleShowAllExtensions"},
		{"nodots", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			domain, key := parseDomainKey(tt.input)
			if domain != tt.wantDomain || key != tt.wantKey {
				t.Errorf("parseDomainKey(%q) = (%q, %q), want (%q, %q)",
					tt.input, domain, key, tt.wantDomain, tt.wantKey)
			}
		})
	}
}

func TestParseDefaultsResource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input      string
		wantDomain string
		wantKey    string
		wantType   string
		wantVal    string
		wantErr    bool
	}{
		{
			input:      "com.apple.dock.autohide=bool:true",
			wantDomain: "com.apple.dock", wantKey: "autohide",
			wantType: "bool", wantVal: "true",
		},
		{
			input:      "com.apple.dock.tilesize=int:36",
			wantDomain: "com.apple.dock", wantKey: "tilesize",
			wantType: "int", wantVal: "36",
		},
		{
			input:   "no-equals-sign",
			wantErr: true,
		},
		{
			input:   "domain.key=nocolon",
			wantErr: true,
		},
		{
			input:   "nodots=bool:true",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			domain, key, typeStr, value, err := parseDefaultsResource(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if domain != tt.wantDomain || key != tt.wantKey || typeStr != tt.wantType || value != tt.wantVal {
				t.Errorf("got (%q, %q, %q, %q), want (%q, %q, %q, %q)",
					domain, key, typeStr, value, tt.wantDomain, tt.wantKey, tt.wantType, tt.wantVal)
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	if p.Name() != "defaults" || p.DisplayName() != "defaults" {
		t.Errorf("Name()=%q DisplayName()=%q", p.Name(), p.DisplayName())
	}
}
