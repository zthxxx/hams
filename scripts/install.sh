#!/usr/bin/env bash
#
# hams installer — downloads the latest release binary for the current platform.
#
# Usage:
#   bash -c "$(curl -fsSL https://github.com/zthxxx/hams/raw/main/scripts/install.sh)"
#
# Design note — WHY no api.github.com:
#   GitHub's Releases API (api.github.com) rate-limits anonymous clients to
#   60 requests per hour per public IP. A shared-egress fleet (offices, CI,
#   NAT'd households) exhausts that quota in seconds, and every subsequent
#   `curl | bash` invocation fails with 403 long before the binary is ever
#   fetched. The public Releases CDN at github.com/<repo>/releases/... has
#   no such cap; we rely on its two stable behaviors:
#
#     1. HEAD https://github.com/<repo>/releases/latest
#          → 302 Location: https://github.com/<repo>/releases/tag/<tag>
#        We parse the tag out of the final URL. No JSON, no API call.
#
#     2. Deterministic asset URLs:
#          https://github.com/<repo>/releases/download/<tag>/<filename>
#        The filenames are fixed by the release workflow (hams-<os>-<arch>,
#        checksums.txt), so we construct them directly — no enumeration.
#
set -euo pipefail

REPO="zthxxx/hams"
GITHUB_RELEASES_URL="https://github.com/${REPO}/releases"

# Determine install directory with priority-based detection.
# Priority: HAMS_INSTALL_DIR > ~/.local/bin > /usr/local/bin > /usr/bin > /bin > shortest $PATH entry.
resolve_install_dir() {
  # Priority 1: explicit env var.
  if [ -n "${HAMS_INSTALL_DIR:-}" ] && [ -d "${HAMS_INSTALL_DIR}" ]; then
    echo "${HAMS_INSTALL_DIR}"
    return
  fi

  # Priority 2-5: well-known directories in order.
  local candidates=(
    "${HOME}/.local/bin"
    "/usr/local/bin"
    "/usr/bin"
    "/bin"
  )

  for dir in "${candidates[@]}"; do
    if [ -d "${dir}" ]; then
      echo "${dir}"
      return
    fi
  done

  # Fallback: shortest directory in $PATH that exists.
  local shortest=""
  local shortest_len=9999
  IFS=':' read -ra path_dirs <<< "${PATH}"
  for dir in "${path_dirs[@]}"; do
    if [ -d "${dir}" ] && [ ${#dir} -lt ${shortest_len} ]; then
      shortest="${dir}"
      shortest_len=${#dir}
    fi
  done

  if [ -n "${shortest}" ]; then
    echo "${shortest}"
    return
  fi

  # Last resort.
  echo "/usr/local/bin"
}

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "${os}" in
    darwin) os="darwin" ;;
    linux)  os="linux" ;;
    *)
      echo "Error: unsupported OS '${os}'. hams supports darwin and linux." >&2
      exit 1
      ;;
  esac

  case "${arch}" in
    x86_64|amd64)  arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      echo "Error: unsupported architecture '${arch}'. hams supports amd64 and arm64." >&2
      exit 1
      ;;
  esac

  echo "${os}/${arch}"
}

# Returns the latest release tag by HEAD-ing /releases/latest and reading
# the final URL after following the 302 redirect to /releases/tag/<tag>.
# No api.github.com call — see design note at top of file.
get_latest_version() {
  local url="${GITHUB_RELEASES_URL}/latest"
  local final=""
  if command -v curl &>/dev/null; then
    # -sSLI: silent, show errors, follow redirects, HEAD.
    # -w '%{url_effective}': print the final URL after all redirects.
    final=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${url}")
  elif command -v wget &>/dev/null; then
    # wget's -S --spider prints response headers to stderr for each hop.
    # Capture the last Location: line; on busybox wget that is usually the
    # only one (auto-follow is limited) but GNU wget will show them all —
    # either way, the last one is the tag URL.
    final=$(wget --server-response --spider -O /dev/null "${url}" 2>&1 \
            | awk '/^[ \t]*Location:/ {loc=$2} END {if (loc) print loc}')
  else
    echo "Error: curl or wget required." >&2
    exit 1
  fi

  # Guard against unexpected redirect targets (login wall, repo moved).
  case "${final}" in
    */releases/tag/*) ;;
    *)
      echo "Error: unexpected redirect target '${final}' — expected .../releases/tag/<tag>" >&2
      exit 1
      ;;
  esac
  # Tag name is the last path segment.
  echo "${final##*/}"
}

# Download a URL to a local path. Fails on non-2xx.
http_fetch() {
  local url="$1" dest="$2"
  if command -v curl &>/dev/null; then
    curl -fsSL -o "${dest}" "${url}"
  else
    wget -q -O "${dest}" "${url}"
  fi
}

# Compute the SHA256 of a file (hex-encoded, lowercase).
compute_sha256() {
  local file="$1"
  if command -v sha256sum &>/dev/null; then
    sha256sum "${file}" | awk '{print $1}'
  elif command -v shasum &>/dev/null; then
    shasum -a 256 "${file}" | awk '{print $1}'
  else
    echo ""  # No hasher available.
  fi
}

# Fetch checksums.txt for the tag and return the expected SHA256 for
# `wantBinary`. Prints empty string (exit 0) when the manifest is absent
# (older releases predate it) or no hasher is available. Exits non-zero
# only on unexpected HTTP errors.
lookup_checksum() {
  local tag="$1" wantBinary="$2"
  local manifest_url="${GITHUB_RELEASES_URL}/download/${tag}/checksums.txt"
  local tmp_manifest="${TMPDIR}/checksums.txt"
  if ! http_fetch "${manifest_url}" "${tmp_manifest}" 2>/dev/null; then
    # 404 or other error — older release without checksums.txt is an
    # allowed fallback; the caller logs a warning.
    echo ""
    return 0
  fi
  awk -v want="${wantBinary}" '$NF == want { print $1; exit }' "${tmp_manifest}"
}

TMPDIR=""
cleanup() {
  if [ -n "${TMPDIR}" ] && [ -d "${TMPDIR}" ]; then
    rm -rf "${TMPDIR}"
  fi
}
trap cleanup EXIT

download_binary() {
  local version="$1" platform="$2" install_dir="$3"
  local os="${platform%/*}"
  local arch="${platform#*/}"
  local filename="hams-${os}-${arch}"
  local tag="${version}"
  local url="${GITHUB_RELEASES_URL}/download/${tag}/${filename}"

  echo "Downloading hams ${version} for ${os}/${arch}..."

  TMPDIR="$(mktemp -d)"

  http_fetch "${url}" "${TMPDIR}/hams"

  # Optional integrity verification — identical mental model to
  # selfupdate.LookupChecksum (internal/selfupdate/selfupdate.go).
  local expected_sha
  expected_sha="$(lookup_checksum "${tag}" "${filename}")"
  if [ -n "${expected_sha}" ]; then
    local actual_sha
    actual_sha="$(compute_sha256 "${TMPDIR}/hams")"
    if [ -z "${actual_sha}" ]; then
      echo "Warning: no sha256sum/shasum available — skipping integrity check." >&2
    elif [ "${expected_sha}" != "${actual_sha}" ]; then
      echo "Error: SHA256 mismatch for ${filename}" >&2
      echo "  expected: ${expected_sha}" >&2
      echo "  got:      ${actual_sha}" >&2
      exit 1
    else
      echo "SHA256 verified: ${expected_sha}"
    fi
  else
    echo "Warning: checksums.txt not published for this release — skipping integrity check." >&2
  fi

  chmod +x "${TMPDIR}/hams"

  echo "Installing to ${install_dir}/hams..."
  if [ -w "${install_dir}" ]; then
    mv "${TMPDIR}/hams" "${install_dir}/hams"
  else
    sudo mv "${TMPDIR}/hams" "${install_dir}/hams"
  fi
}

main() {
  echo "hams installer"
  echo ""

  local platform version install_dir
  platform="$(detect_platform)"
  echo "Detected platform: ${platform}"

  install_dir="$(resolve_install_dir)"
  echo "Install directory: ${install_dir}"

  version="$(get_latest_version)"
  if [ -z "${version}" ]; then
    echo "Error: could not determine latest version. Check ${GITHUB_RELEASES_URL}" >&2
    exit 1
  fi
  echo "Latest version: ${version}"

  download_binary "${version}" "${platform}" "${install_dir}"

  # Verify installation succeeded.
  echo ""
  if command -v hams &>/dev/null; then
    echo "hams installed successfully at: $(command -v hams)"
    echo "Run 'hams --help' to get started."
  else
    echo "Error: hams was installed to ${install_dir}/hams but is not found in PATH." >&2
    echo "  suggestion: Add '${install_dir}' to your PATH:" >&2
    echo "    export PATH=\"${install_dir}:\$PATH\"" >&2
    exit 1
  fi
}

main "$@"
