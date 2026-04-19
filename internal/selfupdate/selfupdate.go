// Package selfupdate implements the hams self-upgrade mechanism via GitHub Releases or Homebrew.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/zthxxx/hams/internal/config"
	"github.com/zthxxx/hams/internal/version"
)

// Channel represents how hams was installed.
type Channel string

const (
	// ChannelBinary indicates hams was installed as a standalone binary.
	ChannelBinary Channel = "binary"
	// ChannelHomebrew indicates hams was installed via Homebrew.
	ChannelHomebrew Channel = "homebrew"
)

const (
	githubRepo      = "zthxxx/hams"
	channelFileName = "install-channel"

	// releasesBaseURL and the /latest + /download/<tag>/<asset> paths
	// below are the two public, CDN-backed endpoints we rely on. They
	// are NOT rate-limited the same way api.github.com is (60 req/h for
	// anonymous clients), which matters for a CLI auto-updater running
	// across a NAT'd fleet: if everyone shares one egress IP, the API
	// ceiling is hit in minutes and every update attempt errors out.
	//
	// Discovery flow:
	//   HEAD https://github.com/<repo>/releases/latest
	//     → 302 Location: https://github.com/<repo>/releases/tag/<tag>
	//   The tag is read from the final URL's last path segment.
	//
	// Asset URL (deterministic, no enumeration):
	//   https://github.com/<repo>/releases/download/<tag>/<asset>
	//   Where <asset> is hams-<goos>-<goarch> or "checksums.txt".
	releasesBaseURL = "https://github.com/" + githubRepo + "/releases"
)

// DetectChannel determines the install channel for the running binary.
// It first checks for a marker file at ${HAMS_DATA_HOME}/install-channel.
// If absent, it infers from the binary's path: paths containing "/homebrew/"
// or "/Cellar/" indicate Homebrew; everything else is treated as binary.
func DetectChannel(paths config.Paths) Channel {
	markerPath := filepath.Join(paths.DataHome, channelFileName)
	data, err := os.ReadFile(markerPath) //nolint:gosec // marker file path is derived from config, not user input
	if err == nil {
		ch := Channel(strings.TrimSpace(string(data)))
		if ch == ChannelHomebrew || ch == ChannelBinary {
			return ch
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return ChannelBinary
	}
	resolved, evalErr := filepath.EvalSymlinks(exe)
	if evalErr == nil {
		exe = resolved
	}

	if strings.Contains(exe, "/homebrew/") || strings.Contains(exe, "/Cellar/") {
		return ChannelHomebrew
	}
	return ChannelBinary
}

// Updater performs self-upgrade operations.
type Updater struct {
	// HTTPClient allows injection for testing.
	HTTPClient *http.Client
}

// NewUpdater creates an Updater with a default HTTP client.
func NewUpdater() *Updater {
	return &Updater{HTTPClient: http.DefaultClient}
}

// LatestVersion returns the latest released version string (without the "v" prefix).
//
// Implementation: HEAD the public /releases/latest URL; GitHub's CDN
// redirects to /releases/tag/<tag>. The tag is read from the final URL
// after following redirects. No api.github.com call, no rate limit.
func (u *Updater) LatestVersion(ctx context.Context) (string, error) {
	tag, err := u.latestTag(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(tag, "v"), nil
}

// latestTag returns the raw tag string (e.g. "v0.0.1") for the latest release.
func (u *Updater) latestTag(ctx context.Context) (string, error) {
	target := releasesBaseURL + "/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close errors are non-actionable
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned status %d for %s", resp.StatusCode, target)
	}
	// After following redirects, resp.Request.URL points at .../releases/tag/<tag>.
	// Guard against unexpected structures (no redirect happened, or ended somewhere else).
	finalURL := resp.Request.URL
	u2, parseErr := url.Parse(finalURL.String())
	if parseErr != nil {
		return "", fmt.Errorf("parsing final URL %q: %w", finalURL, parseErr)
	}
	segments := strings.Split(strings.TrimSuffix(u2.Path, "/"), "/")
	// Expect .../releases/tag/<tag>
	if len(segments) < 2 || segments[len(segments)-2] != "tag" {
		return "", fmt.Errorf("unexpected redirect target %q — expected path ending in /releases/tag/<tag>", finalURL)
	}
	tag := segments[len(segments)-1]
	if tag == "" || tag == "latest" {
		return "", fmt.Errorf("could not extract tag from redirect target %q", finalURL)
	}
	return tag, nil
}

// AssetURL returns the public CDN download URL for the named asset at
// the given release version. The URL is constructed deterministically
// from the release workflow's naming convention — no API enumeration
// required. Both bare "0.0.1" and "v0.0.1" forms of `ver` are accepted.
func AssetURL(ver, asset string) string {
	tag := "v" + strings.TrimPrefix(ver, "v")
	return releasesBaseURL + "/download/" + tag + "/" + asset
}

// IsUpToDate returns true when the running version matches or exceeds the latest release.
// It compares major.minor.patch numerically, ignoring pre-release suffixes.
func IsUpToDate(current, latest string) bool {
	cur := normalizeVersion(current)
	lat := normalizeVersion(latest)
	if cur == "" || lat == "" {
		// Fallback to string equality if parsing fails.
		return strings.TrimPrefix(current, "v") == strings.TrimPrefix(latest, "v")
	}
	return cur == lat || compareVersions(cur, lat) >= 0
}

// normalizeVersion strips the "v" prefix and any pre-release/build metadata.
func normalizeVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	// Strip pre-release suffix (-rc1, -beta, etc.) and build metadata (+build123).
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	return v
}

// compareVersions compares two dot-separated version strings numerically.
// Returns -1, 0, or 1.
func compareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	maxLen := max(len(aParts), len(bParts))
	for i := range maxLen {
		var ai, bi int
		if i < len(aParts) {
			ai, _ = strconv.Atoi(aParts[i]) //nolint:errcheck // non-numeric parts default to 0
		}
		if i < len(bParts) {
			bi, _ = strconv.Atoi(bParts[i]) //nolint:errcheck // non-numeric parts default to 0
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// AssetName returns the expected release asset filename for the current platform.
func AssetName() string {
	return fmt.Sprintf("hams-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// ChecksumAssetName is the conventional filename of the SHA256 manifest
// generated by `.github/workflows/release.yml` (line: `sha256sum hams-* >
// checksums.txt`). LookupChecksum downloads the manifest from the release
// CDN using this constant. Exposed so callers/tests can construct the
// same URL surface without hard-coding the literal in multiple places.
const ChecksumAssetName = "checksums.txt"

// LookupChecksum fetches the release's checksums.txt manifest for the
// given version and returns the hex-encoded SHA256 of the asset whose
// name matches `wantBinary`. The manifest is a sequence of lines in
// `<sha256> <whitespace> <filename>` form (matching `sha256sum`'s
// default output). Returns ("", nil) when the manifest is absent (HTTP
// 404) — older releases predate the manifest and we still allow
// upgrade with a logged warning. Returns an error when the manifest IS
// present but doesn't list the requested binary; that's a real
// integrity gap we must NOT silently fall through.
//
// Without this verification, runBinaryUpgrade calls ReplaceBinary
// with empty `expectedSHA256`, skipping the SHA256 integrity check
// entirely. A MITM on the GitHub Releases CDN could swap the binary
// undetected; HTTPS catches transport tampering but not a hostile
// origin response. The repo's release workflow already publishes
// `checksums.txt` alongside the binaries — this helper closes the
// loop.
func (u *Updater) LookupChecksum(ctx context.Context, ver, wantBinary string) (string, error) {
	manifestURL := AssetURL(ver, ChecksumAssetName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating %s request: %w", ChecksumAssetName, err)
	}
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", ChecksumAssetName, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close errors are non-actionable
	if resp.StatusCode == http.StatusNotFound {
		// Older releases pre-date the manifest. Caller logs a warning.
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: status %d", ChecksumAssetName, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", ChecksumAssetName, err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Filename is the LAST field — sha256sum's output uses two
		// spaces between hash and filename, but other producers may
		// use varying whitespace; Fields handles both.
		if fields[len(fields)-1] == wantBinary {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %q not found in %s", wantBinary, ChecksumAssetName)
}

// DownloadAsset fetches a release asset body. Caller must close the reader.
func (u *Updater) DownloadAsset(ctx context.Context, assetURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating download request: %w", err)
	}
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading asset: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close() //nolint:errcheck,gosec // closing on error path
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// ReplaceBinary atomically replaces the running binary at exePath with newBinary content.
// If expectedSHA256 is non-empty, the written content is verified against the checksum
// before the rename. Order: write temp → check integrity → overwrite binary.
func ReplaceBinary(exePath string, newBinary io.Reader, expectedSHA256 string) error {
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, "hams-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	cleanup := func() {
		tmp.Close()        //nolint:errcheck,gosec // best-effort cleanup
		os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
	}

	// Step 1: write new binary to temp file and compute checksum.
	hasher := sha256.New()
	w := io.MultiWriter(tmp, hasher)
	if _, err := io.Copy(w, newBinary); err != nil {
		cleanup()
		return fmt.Errorf("writing binary: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("syncing binary: %w", err)
	}
	tmp.Close() //nolint:errcheck,gosec // already synced

	// Step 2: verify integrity before overwriting.
	if expectedSHA256 != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if got != expectedSHA256 {
			os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA256, got)
		}
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil { //nolint:gosec // binary must be executable
		os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
		return fmt.Errorf("setting permissions: %w", err)
	}

	// Step 3: atomic rename to overwrite binary.
	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
		return fmt.Errorf("replacing binary: %w", err)
	}
	return nil
}

// CurrentVersion returns the running binary's version.
func CurrentVersion() string {
	return version.Version()
}
