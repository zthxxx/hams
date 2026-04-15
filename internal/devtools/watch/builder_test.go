package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBuildEnv_ForcesLinuxArchAndCGO(t *testing.T) {
	t.Setenv("GOOS", "darwin")   // caller leak — watcher must override
	t.Setenv("GOARCH", "amd64")  // caller leak — watcher must override
	t.Setenv("CGO_ENABLED", "1") // caller leak — watcher must override
	t.Setenv("GOCACHE", "/tmp/custom-cache")

	got := buildEnv("arm64", nil)

	want := map[string]string{
		"GOOS":        "linux",
		"GOARCH":      "arm64",
		"CGO_ENABLED": "0",
		"GOCACHE":     "/tmp/custom-cache", // must pass through — incremental builds
	}

	actual := parseEnv(got)
	for k, v := range want {
		if actual[k] != v {
			t.Errorf("buildEnv key %s = %q, want %q", k, actual[k], v)
		}
	}

	// Ensure no duplicate GOOS/GOARCH/CGO_ENABLED leaked through.
	counts := make(map[string]int)
	for _, kv := range got {
		if key, _, ok := strings.Cut(kv, "="); ok {
			counts[key]++
		}
	}
	for _, k := range []string{"GOOS", "GOARCH", "CGO_ENABLED"} {
		if counts[k] != 1 {
			t.Errorf("env key %s appears %d times, want 1", k, counts[k])
		}
	}
}

func TestBuildEnv_AppendsExtra(t *testing.T) {
	got := buildEnv("amd64", []string{"HAMS_EXTRA=1"})
	if !slices.Contains(got, "HAMS_EXTRA=1") {
		t.Errorf("buildEnv did not include extra env var")
	}
}

func parseEnv(kv []string) map[string]string {
	m := make(map[string]string, len(kv))
	for _, e := range kv {
		if key, val, ok := strings.Cut(e, "="); ok {
			m[key] = val
		}
	}
	return m
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		50 * time.Millisecond:                "50ms",
		999 * time.Millisecond:               "999ms",
		time.Second:                          "1.00s",
		1230 * time.Millisecond:              "1.23s",
		5*time.Second + 678*time.Millisecond: "5.68s",
	}
	for d, want := range cases {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}
