package homebrew

import (
	"path/filepath"
	"testing"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/provider/baseprovider"
)

func TestManifest(t *testing.T) {
	p := New(nil, NewFakeCmdRunner())
	m := p.Manifest()
	if m.Name != "brew" {
		t.Errorf("Name = %q, want 'brew'", m.Name)
	}
	if m.DisplayName != "Homebrew" {
		t.Errorf("DisplayName = %q, want 'Homebrew'", m.DisplayName)
	}
	if m.ResourceClass != provider.ClassPackage {
		t.Errorf("ResourceClass = %d, want ClassPackage", m.ResourceClass)
	}
	if len(m.DependsOn) != 1 {
		t.Fatalf("DependsOn = %d, want 1", len(m.DependsOn))
	}
	if m.DependsOn[0].Provider != "bash" {
		t.Errorf("DependsOn[0].Provider = %q, want 'bash'", m.DependsOn[0].Provider)
	}
}

func TestName(t *testing.T) {
	p := New(nil, NewFakeCmdRunner())
	if p.Name() != "brew" {
		t.Errorf("Name() = %q, want 'brew'", p.Name())
	}
	if p.DisplayName() != "Homebrew" {
		t.Errorf("DisplayName() = %q, want 'Homebrew'", p.DisplayName())
	}
}

// Asserts the empty-doc path through hamsfile.LoadOrCreateEmpty: a missing
// file returns a fresh File rooted at the expected path rather than an error
// (os.IsNotExist would not match here because Read wraps with %w).
func TestLoadOrCreateHamsfile_MissingFileReturnsEmpty(t *testing.T) {
	storeDir := t.TempDir()
	p := New(&config.Config{StorePath: storeDir, ProfileTag: "test"}, NewFakeCmdRunner())

	hf, err := baseprovider.LoadOrCreateHamsfile(p.cfg, p.Manifest().FilePrefix, nil, &provider.GlobalFlags{})
	if err != nil {
		t.Fatalf("LoadOrCreateHamsfile on missing file = %v, want nil", err)
	}
	if hf == nil {
		t.Fatal("LoadOrCreateHamsfile returned nil hamsfile")
	}
	wantPath := filepath.Join(storeDir, "test", "Homebrew.hams.yaml")
	if hf.Path != wantPath {
		t.Errorf("hf.Path = %q, want %q", hf.Path, wantPath)
	}
	if hf.Root == nil {
		t.Fatal("hf.Root is nil; expected an empty mapping document node")
	}
}

// TestIsTapFormat covers the parsing of "user/repo" style tap
// identifiers vs fully-qualified formula names ("user/repo/formula").
// The function is the gate between `brew tap` and `brew install` paths.
func TestIsTapFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  bool
	}{
		{"homebrew/core", true},                   // plain tap
		{"homebrew/core/gcc", false},              // formula in a tap (3 parts)
		{"gcc", false},                            // bare formula
		{"", false},                               // empty
		{"user/repo.git", false},                  // has dot → not a tap shortname
		{"user//repo", false},                     // double slash → 3 parts
		{"company/priv", true},                    // another plain tap
		{"homebrew/versions/postgresql10", false}, // formula with digits
	}
	for _, tc := range cases {
		if got := isTapFormat(tc.input); got != tc.want {
			t.Errorf("isTapFormat(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestParseInstallTag extracts the single tag used to annotate `hams
// brew install <pkg>` records. Defaults to `cli`, trims whitespace,
// takes the first CSV element.
func TestParseInstallTag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		flags map[string]string
		want  string
	}{
		{nil, tagCLI},                                   // no flag → default
		{map[string]string{}, tagCLI},                   // empty flags
		{map[string]string{"tag": ""}, tagCLI},          // empty value → default
		{map[string]string{"tag": "   "}, tagCLI},       // whitespace → default
		{map[string]string{"tag": "dev"}, "dev"},        // single value
		{map[string]string{"tag": "dev,mobile"}, "dev"}, // CSV → first wins
		{map[string]string{"tag": "  prod  "}, "prod"},  // outer whitespace trimmed
		{map[string]string{"tag": "  a ,  b  "}, "a"},   // trimmed+csv
		{map[string]string{"tag": ",dev"}, tagCLI},      // leading empty CSV → default
	}
	for _, tc := range cases {
		if got := parseInstallTag(tc.flags); got != tc.want {
			t.Errorf("parseInstallTag(%v) = %q, want %q", tc.flags, got, tc.want)
		}
	}
}

// TestPackageArgs keeps only positional (non-flag) args. Used by
// `hams brew install pkgA --cask pkgB` to extract [pkgA, pkgB]
// before recording.
func TestPackageArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []string
		want []string
	}{
		{nil, nil},
		{[]string{}, nil},
		{[]string{"pkg"}, []string{"pkg"}},
		{[]string{"--cask"}, nil},
		{[]string{"pkg", "--cask"}, []string{"pkg"}},
		{[]string{"--cask", "pkg"}, []string{"pkg"}},
		{[]string{"pkg1", "--cask", "pkg2"}, []string{"pkg1", "pkg2"}},
		{[]string{"--foo", "--bar", "one", "two"}, []string{"one", "two"}},
	}
	for _, tc := range cases {
		got := packageArgs(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("packageArgs(%v) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("packageArgs(%v)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

// TestHasCaskFlag detects whether --cask was passed. Governs whether
// the hamsfile records the package under the "cask" tag.
func TestHasCaskFlag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"pkg"}, false},
		{[]string{"--cask"}, true},
		{[]string{"pkg", "--cask"}, true},
		{[]string{"--cask", "pkg"}, true},
		{[]string{"--casks"}, false}, // different flag
		{[]string{"--no-cask"}, false},
	}
	for _, tc := range cases {
		if got := hasCaskFlag(tc.in); got != tc.want {
			t.Errorf("hasCaskFlag(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
