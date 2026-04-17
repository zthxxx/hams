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

// TestLatestRelease_SuccessPopulatesAssets is the ReleaseInfo counterpart
// of TestLatestVersion_Success — asserts the tag_name + assets are
// both mapped onto ReleaseInfo so the binary-upgrade flow can find
// the right download URL.
func TestLatestRelease_SuccessPopulatesAssets(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v2.0.0","assets":[{"name":"hams-linux-amd64","browser_download_url":"https://example.test/hams-linux-amd64"}]}`) //nolint:errcheck // test handler
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	rel, err := u.LatestRelease(context.Background())
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if rel.Version != "2.0.0" {
		t.Errorf("Version = %q, want 2.0.0", rel.Version)
	}
	if len(rel.Assets) != 1 {
		t.Fatalf("Assets len = %d, want 1", len(rel.Assets))
	}
	if rel.Assets[0].Name != "hams-linux-amd64" {
		t.Errorf("asset name = %q", rel.Assets[0].Name)
	}
	if rel.Assets[0].DownloadURL != "https://example.test/hams-linux-amd64" {
		t.Errorf("asset URL = %q", rel.Assets[0].DownloadURL)
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

// TestLookupChecksum_HappyPath asserts cycle 159: when the release
// includes a checksums.txt asset and the manifest contains the
// requested binary, LookupChecksum returns the hex sha256.
// Without this verification, runBinaryUpgrade was calling
// ReplaceBinary with empty expectedSHA256 — skipping the integrity
// check entirely. A MITM on the GitHub Releases CDN could swap the
// binary undetected.
func TestLookupChecksum_HappyPath(t *testing.T) {
	t.Parallel()
	const wantHash = "abc123def456000000000000000000000000000000000000000000000000abcd"
	const otherHash = "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Body mimics `sha256sum hams-* > checksums.txt` output.
		body := wantHash + "  hams-linux-amd64\n" +
			otherHash + "  hams-darwin-arm64\n"
		w.Write([]byte(body)) //nolint:errcheck // test handler
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	assets := []Asset{
		{Name: "hams-linux-amd64", DownloadURL: "https://example.test/hams-linux-amd64"},
		{Name: ChecksumAssetName, DownloadURL: "https://example.test/" + ChecksumAssetName},
	}

	got, err := u.LookupChecksum(context.Background(), assets, "hams-linux-amd64")
	if err != nil {
		t.Fatalf("LookupChecksum: %v", err)
	}
	if got != wantHash {
		t.Errorf("hash = %q, want %q", got, wantHash)
	}
}

// TestLookupChecksum_NoManifestAssetReturnsEmptyNoError asserts the
// older-release fallback path: when checksums.txt is absent from the
// asset list (older releases pre-date the manifest), LookupChecksum
// returns ("", nil) so the caller can warn-and-proceed without
// erroring out the upgrade.
func TestLookupChecksum_NoManifestAssetReturnsEmptyNoError(t *testing.T) {
	t.Parallel()
	u := &Updater{HTTPClient: http.DefaultClient}

	assets := []Asset{
		{Name: "hams-linux-amd64", DownloadURL: "https://example.test/hams-linux-amd64"},
		// No checksums.txt asset.
	}

	got, err := u.LookupChecksum(context.Background(), assets, "hams-linux-amd64")
	if err != nil {
		t.Errorf("expected nil err for missing manifest, got %v", err)
	}
	if got != "" {
		t.Errorf("expected empty hash for missing manifest, got %q", got)
	}
}

// TestLookupChecksum_ManifestPresentButBinaryMissingErrors asserts
// the security-critical case: when checksums.txt IS present but
// doesn't list the requested binary, LookupChecksum returns an
// error instead of silently falling through. We must NOT skip
// integrity verification when the manifest disagrees with our
// expectations — that's the MITM attack surface we're trying to
// close.
func TestLookupChecksum_ManifestPresentButBinaryMissingErrors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Manifest lists ONLY hams-darwin-arm64; not the requested linux build.
		body := "1234567890123456789012345678901234567890123456789012345678901234  hams-darwin-arm64\n"
		w.Write([]byte(body)) //nolint:errcheck // test handler
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	assets := []Asset{
		{Name: ChecksumAssetName, DownloadURL: "https://example.test/" + ChecksumAssetName},
	}

	_, err := u.LookupChecksum(context.Background(), assets, "hams-linux-amd64")
	if err == nil {
		t.Fatal("expected error for binary missing from manifest")
	}
	if !strings.Contains(err.Error(), "hams-linux-amd64") {
		t.Errorf("error should name the missing binary, got: %v", err)
	}
}

// TestLookupChecksum_ManifestNetworkErrorPropagates asserts a
// transient HTTP failure on the manifest fetch surfaces as an
// error so we don't proceed with an unverified install.
func TestLookupChecksum_ManifestNetworkErrorPropagates(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := &Updater{HTTPClient: &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, target: srv.URL},
	}}

	assets := []Asset{
		{Name: ChecksumAssetName, DownloadURL: "https://example.test/" + ChecksumAssetName},
	}

	_, err := u.LookupChecksum(context.Background(), assets, "hams-linux-amd64")
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
