#!/usr/bin/env bash
#
# hams installer — downloads the latest release binary for the current platform.
#
# Usage:
#   bash -c "$(curl -fsSL https://github.com/zthxxx/hams/raw/main/scripts/install.sh)"
#
set -euo pipefail

REPO="zthxxx/hams"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"

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

get_latest_version() {
  if command -v curl &>/dev/null; then
    curl -fsSL "${GITHUB_API}" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
  elif command -v wget &>/dev/null; then
    wget -qO- "${GITHUB_API}" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
  else
    echo "Error: curl or wget required." >&2
    exit 1
  fi
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
  local url="https://github.com/${REPO}/releases/download/${version}/${filename}"

  echo "Downloading hams ${version} for ${os}/${arch}..."

  TMPDIR="$(mktemp -d)"

  if command -v curl &>/dev/null; then
    curl -fsSL "${url}" -o "${TMPDIR}/hams"
  else
    wget -qO "${TMPDIR}/hams" "${url}"
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
    echo "Error: could not determine latest version. Check https://github.com/${REPO}/releases" >&2
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
