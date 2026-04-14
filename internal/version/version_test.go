package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestInfo_ContainsVersion(t *testing.T) {
	info := Info()
	if !strings.Contains(info, "hams") {
		t.Errorf("Info() = %q, want to contain 'hams'", info)
	}
	if !strings.Contains(info, runtime.GOOS) {
		t.Errorf("Info() = %q, want to contain GOOS %q", info, runtime.GOOS)
	}
	if !strings.Contains(info, runtime.GOARCH) {
		t.Errorf("Info() = %q, want to contain GOARCH %q", info, runtime.GOARCH)
	}
}

func TestVersion_DefaultDev(t *testing.T) {
	if got := Version(); got != "dev" {
		t.Errorf("Version() = %q, want 'dev'", got)
	}
}

func TestCommit_DefaultUnknown(t *testing.T) {
	if got := Commit(); got != "unknown" {
		t.Errorf("Commit() = %q, want 'unknown'", got)
	}
}

func TestDate_DefaultUnknown(t *testing.T) {
	if got := Date(); got != "unknown" {
		t.Errorf("Date() = %q, want 'unknown'", got)
	}
}

func TestBrief_Format(t *testing.T) {
	origVersion, origCommit := version, commit
	t.Cleanup(func() {
		version, commit = origVersion, origCommit
	})

	cases := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{"dev with injected sha", "dev", "a6f4218", "dev (a6f4218)"},
		{"release with injected sha", "v1.2.4", "a6f4218", "v1.2.4 (a6f4218)"},
		{"dev without ldflags falls back to unknown", "dev", "unknown", "dev (unknown)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version, commit = tc.version, tc.commit
			if got := Brief(); got != tc.want {
				t.Errorf("Brief() = %q, want %q", got, tc.want)
			}
		})
	}
}
