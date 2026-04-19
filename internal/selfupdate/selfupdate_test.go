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

// TestIsUpToDate_CurrentNewerThanLatest asserts that a local
// build ahead of the latest release is still "up-to-date" — the
// user doesn't need to downgrade. This path was untested and a
// regression could make `hams self-upgrade` on a dev/pre-release
// build try to downgrade to an older stable tag.
func TestIsUpToDate_CurrentNewerThanLatest(t *testing.T) {
	t.Parallel()
	if !IsUpToDate("2.0.0", "1.9.9") {
		t.Error("IsUpToDate(2.0.0, 1.9.9) = false, want true")
	}
	if !IsUpToDate("v1.10.0", "1.9.9") {
		t.Error("IsUpToDate(v1.10.0, 1.9.9) = false, want true")
	}
}

// TestIsUpToDate_PreReleaseStripped asserts pre-release suffixes
// (-rc1, -beta) are ignored when comparing. A user running
// 1.0.0-rc1 against latest 1.0.0 is still considered up-to-date
// per the "ignore pre-release suffixes" comment — otherwise
// `hams self-upgrade` would try to "upgrade" from rc1 to final
// every time the rc is tagged, producing confusing churn.
func TestIsUpToDate_PreReleaseStripped(t *testing.T) {
	t.Parallel()
	if !IsUpToDate("1.0.0-rc1", "1.0.0") {
		t.Error("IsUpToDate(1.0.0-rc1, 1.0.0) = false, want true (rc stripped)")
	}
	if !IsUpToDate("1.0.0", "1.0.0-rc1") {
		t.Error("IsUpToDate(1.0.0, 1.0.0-rc1) = false, want true")
	}
	if !IsUpToDate("1.0.0+build123", "1.0.0") {
		t.Error("IsUpToDate with build metadata stripped should be equal")
	}
}

// TestIsUpToDate_DifferentLengths asserts versions of different
// dot-depths compare correctly. `1.0` vs `1.0.0` SHOULD be equal
// (missing parts default to 0 in the numeric comparator).
func TestIsUpToDate_DifferentLengths(t *testing.T) {
	t.Parallel()
	if !IsUpToDate("1.0", "1.0.0") {
		t.Error("IsUpToDate(1.0, 1.0.0) = false, want true (missing parts = 0)")
	}
	if !IsUpToDate("1.0.0.0", "1.0.0") {
		t.Error("IsUpToDate(1.0.0.0, 1.0.0) = false, want true")
	}
}

// TestIsUpToDate_NonNumericFallback asserts that when normalize
// strips everything (e.g., "dev" → "dev", which fails numeric
// parse), the function falls back to string equality. Mixing
// "dev" with a real version returns false.
func TestIsUpToDate_NonNumericFallback(t *testing.T) {
	t.Parallel()
	// Two "dev" builds compare equal via string fallback.
	if !IsUpToDate("dev", "dev") {
		t.Error("IsUpToDate(dev, dev) = false, want true (string equality fallback)")
	}
	// dev vs a real version is NOT up-to-date.
	if IsUpToDate("dev", "1.0.0") {
		t.Error("IsUpToDate(dev, 1.0.0) = true, want false")
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

// TestNewUpdater asserts the constructor returns a non-nil Updater
// bound to http.DefaultClient. One-liner delegate was 0% covered.
func TestNewUpdater(t *testing.T) {
	t.Parallel()
	got := NewUpdater()
	if got == nil {
		t.Fatal("NewUpdater() returned nil")
	}
	if got.HTTPClient == nil {
		t.Error("HTTPClient should default to http.DefaultClient")
	}
}

// TestCurrentVersion just verifies delegation to version.Version().
// Protects against accidental direct reads of the unexported
// `version` var bypassing the version package.
func TestCurrentVersion(t *testing.T) {
	t.Parallel()
	if got := CurrentVersion(); got == "" {
		t.Error("CurrentVersion() should never return empty string")
	}
}

// TestLatestVersion_RedirectDiscovery asserts the happy-path
// discovery flow: HEAD /releases/latest returns 302 → /releases/tag/<tag>,
// and LatestVersion returns the tag (without "v" prefix) read from the
// final URL after following redirects. This is how the refactored
// updater avoids api.github.com's 60-req/h rate limit.
func TestLatestVersion_RedirectDiscovery(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			// Redirect to the tagged release URL, as GitHub's CDN does.
			http.Redirect(w, r, "/zthxxx/hams/releases/tag/v1.2.3", http.StatusFound)
		case strings.HasSuffix(r.URL.Path, "/releases/tag/v1.2.3"):
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	ver, err := u.LatestVersion(context.Background())
	if err != nil {
		t.Fatalf("LatestVersion() error: %v", err)
	}
	if ver != "1.2.3" {
		t.Errorf("LatestVersion() = %q, want %q", ver, "1.2.3")
	}
}

// TestLatestVersion_HTTPError asserts a non-2xx final response
// (after following redirects) is surfaced as an error. Silent
// failure would make the updater stuck on the current version
// without the user noticing.
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

// TestLatestVersion_UnexpectedRedirectTarget asserts the updater
// rejects a redirect that doesn't look like /releases/tag/<tag>.
// Without this guard, a server returning e.g. a login-wall redirect
// could yield an empty or nonsense version that would then be
// pasted into download URLs, producing confusing 404s.
func TestLatestVersion_UnexpectedRedirectTarget(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			http.Redirect(w, r, "/login", http.StatusFound)
		case strings.HasSuffix(r.URL.Path, "/login"):
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	_, err := u.LatestVersion(context.Background())
	if err == nil {
		t.Fatal("expected error for redirect target not matching /releases/tag/<tag>")
	}
	if !strings.Contains(err.Error(), "unexpected redirect target") {
		t.Errorf("error should mention unexpected redirect target, got: %v", err)
	}
}

// TestAssetURL_FormatAndPrefix asserts the deterministic
// construction of public-CDN download URLs. The format is
// contractually tied to .github/workflows/release.yml's artifact
// naming + softprops/action-gh-release upload path — if this test
// breaks we've drifted from the release workflow and install paths
// silently 404.
func TestAssetURL_FormatAndPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		version string
		asset   string
		want    string
	}{
		{"0.0.1", "hams-linux-amd64", "https://github.com/zthxxx/hams/releases/download/v0.0.1/hams-linux-amd64"},
		{"v0.0.1", "hams-darwin-arm64", "https://github.com/zthxxx/hams/releases/download/v0.0.1/hams-darwin-arm64"},
		{"1.2.3", ChecksumAssetName, "https://github.com/zthxxx/hams/releases/download/v1.2.3/checksums.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.version+"/"+tt.asset, func(t *testing.T) {
			t.Parallel()
			if got := AssetURL(tt.version, tt.asset); got != tt.want {
				t.Errorf("AssetURL(%q, %q) = %q, want %q", tt.version, tt.asset, got, tt.want)
			}
		})
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
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("reading temp dir: %v", readErr)
	}
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
		entries, readErr := os.ReadDir(tmpDir)
		if readErr != nil {
			t.Fatalf("reading temp dir: %v", readErr)
		}
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
		fmt.Fprint(w, "binary-data")
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

// TestLookupChecksum_HappyPath asserts: when the release CDN serves
// checksums.txt and the manifest contains the requested binary,
// LookupChecksum returns its hex sha256. Without this verification,
// runBinaryUpgrade falls back to ReplaceBinary with empty
// expectedSHA256 — skipping the integrity check entirely. A MITM on
// the GitHub Releases CDN could swap the binary undetected.
func TestLookupChecksum_HappyPath(t *testing.T) {
	t.Parallel()
	const wantHash = "abc123def456000000000000000000000000000000000000000000000000abcd"
	const otherHash = "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/"+ChecksumAssetName) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body := wantHash + "  hams-linux-amd64\n" +
			otherHash + "  hams-darwin-arm64\n"
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	got, err := u.LookupChecksum(context.Background(), "0.0.1", "hams-linux-amd64")
	if err != nil {
		t.Fatalf("LookupChecksum: %v", err)
	}
	if got != wantHash {
		t.Errorf("hash = %q, want %q", got, wantHash)
	}
}

// TestLookupChecksum_Manifest404ReturnsEmptyNoError asserts the
// older-release fallback path: when the CDN returns 404 for
// checksums.txt (a release predating the manifest), LookupChecksum
// returns ("", nil) so the caller can warn-and-proceed without
// erroring out the upgrade.
func TestLookupChecksum_Manifest404ReturnsEmptyNoError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	got, err := u.LookupChecksum(context.Background(), "0.0.1", "hams-linux-amd64")
	if err != nil {
		t.Errorf("expected nil err for missing manifest, got %v", err)
	}
	if got != "" {
		t.Errorf("expected empty hash for missing manifest, got %q", got)
	}
}

// TestLookupChecksum_ManifestPresentButBinaryMissingErrors asserts
// the security-critical case: when checksums.txt IS served but
// doesn't list the requested binary, LookupChecksum returns an
// error instead of silently falling through. We must NOT skip
// integrity verification when the manifest disagrees with our
// expectations — that's the MITM attack surface we're trying to
// close.
func TestLookupChecksum_ManifestPresentButBinaryMissingErrors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := "1234567890123456789012345678901234567890123456789012345678901234  hams-darwin-arm64\n"
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	_, err := u.LookupChecksum(context.Background(), "0.0.1", "hams-linux-amd64")
	if err == nil {
		t.Fatal("expected error for binary missing from manifest")
	}
	if !strings.Contains(err.Error(), "hams-linux-amd64") {
		t.Errorf("error should name the missing binary, got: %v", err)
	}
}

// TestLookupChecksum_ManifestNetworkErrorPropagates asserts a
// transient HTTP failure (non-404 non-200) on the manifest fetch
// surfaces as an error so we don't proceed with an unverified install.
func TestLookupChecksum_ManifestNetworkErrorPropagates(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	_, err := u.LookupChecksum(context.Background(), "0.0.1", "hams-linux-amd64")
	if err == nil {
		t.Fatal("expected error for 500 response")
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
