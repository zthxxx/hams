package duti

import (
	"testing"

	"github.com/zthxxx/hams/internal/provider"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "duti" {
		t.Errorf("Name = %q", m.Name)
	}
	if len(m.Platforms) != 1 || m.Platforms[0] != provider.PlatformDarwin {
		t.Errorf("Platforms = %v, want [darwin]", m.Platforms)
	}
	if m.ResourceClass != provider.ClassKVConfig {
		t.Errorf("ResourceClass = %q", m.ResourceClass)
	}
}

func TestParseResourceID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input        string
		wantExt      string
		wantBundleID string
		wantErr      bool
	}{
		{"pdf=com.adobe.acrobat.pdf", "pdf", "com.adobe.acrobat.pdf", false},
		{"html=com.google.Chrome", "html", "com.google.Chrome", false},
		{".md=com.microsoft.vscode", "md", "com.microsoft.vscode", false},
		{"noequals", "", "", true},
		{"=empty", "", "", true},
		{"empty=", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			ext, bundleID, err := parseResourceID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext != tt.wantExt || bundleID != tt.wantBundleID {
				t.Errorf("got (%q, %q), want (%q, %q)", ext, bundleID, tt.wantExt, tt.wantBundleID)
			}
		})
	}
}

func TestParseDutiOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"standard output", "Preview\n/System/Applications/Preview.app\nnet.shinyfrog.bear\n", "Preview"},
		{"empty output", "", ""},
		{"whitespace only", "  \n  \n", ""},
		{"single line", "Google Chrome\n", "Google Chrome"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseDutiOutput(tt.output)
			if got != tt.want {
				t.Errorf("parseDutiOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNameDisplayName(t *testing.T) {
	t.Parallel()
	p := New(nil, NewFakeCmdRunner())
	if p.Name() != "duti" || p.DisplayName() != "duti" {
		t.Errorf("Name()=%q DisplayName()=%q", p.Name(), p.DisplayName())
	}
}
