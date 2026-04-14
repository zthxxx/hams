package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"pgregory.net/rapid"

	"github.com/zthxxx/hams/internal/config"
)

func TestDetectChannel_MarkerFile(t *testing.T) {
	t.Parallel()
	for _, ch := range []Channel{ChannelBinary, ChannelHomebrew} {
		t.Run(string(ch), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, channelFileName), []byte(string(ch)+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			paths := config.Paths{DataHome: dir}
			got := DetectChannel(paths)
			if got != ch {
				t.Errorf("DetectChannel() = %q, want %q", got, ch)
			}
		})
	}
}

func TestDetectChannel_NoMarker_DefaultsBinary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	paths := config.Paths{DataHome: dir}
	got := DetectChannel(paths)
	if got != ChannelBinary {
		t.Errorf("DetectChannel() = %q, want %q", got, ChannelBinary)
	}
}

func TestDetectChannel_InvalidMarker_DefaultsBinary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, channelFileName), []byte("invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{DataHome: dir}
	got := DetectChannel(paths)
	// Falls through to binary path inference; since test binary is not in homebrew path, defaults to binary.
	if got != ChannelBinary {
		t.Errorf("DetectChannel() = %q, want %q", got, ChannelBinary)
	}
}

func TestDetectChannel_Property_MarkerFileRoundTrip(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()
	var counter atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		channel := rapid.SampledFrom([]Channel{ChannelBinary, ChannelHomebrew}).Draw(t, "channel")
		// Extra whitespace should be trimmed by DetectChannel.
		padding := rapid.StringMatching(`[ \t\n]{0,5}`).Draw(t, "padding")

		tmpDir := filepath.Join(baseDir, fmt.Sprintf("run-%d", counter.Add(1)))
		if err := os.MkdirAll(tmpDir, 0o750); err != nil {
			t.Fatal(err)
		}

		content := string(channel) + padding
		if err := os.WriteFile(filepath.Join(tmpDir, channelFileName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		paths := config.Paths{DataHome: tmpDir}
		got := DetectChannel(paths)
		if got != channel {
			t.Errorf("DetectChannel() = %q, want %q (content=%q)", got, channel, content)
		}
	})
}

func TestIsUpToDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"1.0.0", "1.0.0", true},
		{"v1.0.0", "1.0.0", true},
		{"1.0.0", "v1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"dev", "1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.current, tt.latest), func(t *testing.T) {
			t.Parallel()
			if got := IsUpToDate(tt.current, tt.latest); got != tt.want {
				t.Errorf("IsUpToDate(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestIsUpToDate_Property(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		ver := rapid.StringMatching(`[0-9]+\.[0-9]+\.[0-9]+`).Draw(t, "ver")
		// Same version with or without v prefix should be up-to-date.
		if !IsUpToDate(ver, ver) {
			t.Errorf("IsUpToDate(%q, %q) should be true", ver, ver)
		}
		if !IsUpToDate("v"+ver, ver) {
			t.Errorf("IsUpToDate(v%q, %q) should be true", ver, ver)
		}
	})
}

func TestAssetName(t *testing.T) {
	t.Parallel()
	name := AssetName()
	expected := fmt.Sprintf("hams-%s-%s", runtime.GOOS, runtime.GOARCH)
	if name != expected {
		t.Errorf("AssetName() = %q, want %q", name, expected)
	}
}

func TestLatestVersion_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v1.2.3","assets":[]}`) //nolint:errcheck // test handler
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: srv.Client()}
	// Override the URL by using a custom transport that redirects to our test server.
	u.HTTPClient = &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}

	ver, err := u.LatestVersion(context.Background())
	if err != nil {
		t.Fatalf("LatestVersion() error: %v", err)
	}
	if ver != "1.2.3" {
		t.Errorf("LatestVersion() = %q, want %q", ver, "1.2.3")
	}
}

func TestLatestVersion_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	_, err := u.LatestVersion(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code 500, got: %v", err)
	}
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestReplaceBinary_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exePath := filepath.Join(dir, "hams")
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	payload := []byte("new-binary-content")
	checksum := sha256Hex(payload)
	if err := ReplaceBinary(exePath, strings.NewReader(string(payload)), checksum); err != nil {
		t.Fatalf("ReplaceBinary() error: %v", err)
	}

	data, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-binary-content" {
		t.Errorf("binary content = %q, want %q", data, "new-binary-content")
	}

	info, err := os.Stat(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("binary should be executable")
	}
}

func TestReplaceBinary_SkipChecksumWhenEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exePath := filepath.Join(dir, "hams")
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceBinary(exePath, strings.NewReader("payload"), ""); err != nil {
		t.Fatalf("ReplaceBinary() with empty checksum should succeed: %v", err)
	}

	data, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Errorf("binary content = %q, want %q", data, "payload")
	}
}

func TestReplaceBinary_ChecksumMismatch_OriginalIntact(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exePath := filepath.Join(dir, "hams")
	originalContent := []byte("original-binary")
	if err := os.WriteFile(exePath, originalContent, 0o755); err != nil {
		t.Fatal(err)
	}

	err := ReplaceBinary(exePath, strings.NewReader("tampered-content"), "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error should mention checksum mismatch, got: %v", err)
	}

	// Original binary must remain intact.
	data, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(originalContent) {
		t.Errorf("original binary corrupted: got %q, want %q", data, originalContent)
	}

	// No temp files should remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "hams-update-") {
			t.Errorf("temp file %q not cleaned up", e.Name())
		}
	}
}

func TestReplaceBinary_Property_AtomicOnFailure(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()
	var counter atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		tmpDir := filepath.Join(baseDir, fmt.Sprintf("run-%d", counter.Add(1)))
		if err := os.MkdirAll(tmpDir, 0o750); err != nil {
			t.Fatal(err)
		}

		originalContent := rapid.SliceOfN(rapid.Byte(), 1, 256).Draw(t, "original")
		exePath := filepath.Join(tmpDir, "hams")
		if err := os.WriteFile(exePath, originalContent, 0o755); err != nil {
			t.Fatal(err)
		}

		newContent := rapid.SliceOfN(rapid.Byte(), 1, 256).Draw(t, "new")
		wrongChecksum := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		replaceErr := ReplaceBinary(exePath, strings.NewReader(string(newContent)), wrongChecksum)

		// Must fail with wrong checksum.
		if replaceErr == nil {
			t.Fatal("expected error with wrong checksum")
		}

		// Invariant: original binary byte-identical after failure.
		data, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(originalContent) {
			t.Error("original binary was corrupted after failed update")
		}

		// Invariant: no temp files left behind.
		entries, _ := os.ReadDir(tmpDir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "hams-update-") {
				t.Errorf("temp file %q not cleaned up", e.Name())
			}
		}
	})
}

func TestDownloadAsset_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "binary-data") //nolint:errcheck // test handler
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: srv.Client()}
	body, err := u.DownloadAsset(context.Background(), srv.URL+"/asset")
	if err != nil {
		t.Fatalf("DownloadAsset() error: %v", err)
	}
	defer body.Close() //nolint:errcheck // test cleanup

	data, err := readAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary-data" {
		t.Errorf("downloaded = %q, want %q", data, "binary-data")
	}
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}

// rewriteTransport redirects all requests to the test server.
type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.target, "http://")
	transport := t.base
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(req)
}
