// Package selfupdate implements the hams self-upgrade mechanism via GitHub Releases or Homebrew.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
)

// ghRelease represents the relevant fields of a GitHub Releases API response.
type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// ghAsset represents a single release asset.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

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

// LatestVersion queries GitHub Releases for the latest tag name.
func (u *Updater) LatestVersion(ctx context.Context) (string, error) {
	rel, err := u.latestRelease(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

func (u *Updater) latestRelease(ctx context.Context) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close errors are non-actionable

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}
	return &rel, nil
}

// IsUpToDate returns true when the running version matches the latest release.
func IsUpToDate(current, latest string) bool {
	return strings.TrimPrefix(current, "v") == strings.TrimPrefix(latest, "v")
}

// AssetName returns the expected release asset filename for the current platform.
func AssetName() string {
	return fmt.Sprintf("hams-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// DownloadAsset fetches a release asset body. Caller must close the reader.
func (u *Updater) DownloadAsset(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
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
