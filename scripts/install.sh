#!/usr/bin/env bash
#
# hams installer — downloads the latest release binary for the current platform.
#
# Usage:
#   bash -c "$(curl -fsSL https://github.com/zthxxx/hams/raw/master/scripts/install.sh)"
#
set -euo pipefail

REPO="zthxxx/hams"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"

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

download_binary() {
  local version="$1" platform="$2"
  local os="${platform%/*}"
  local arch="${platform#*/}"
  local filename="hams-${os}-${arch}"
  local url="https://github.com/${REPO}/releases/download/${version}/${filename}"

  echo "Downloading hams ${version} for ${os}/${arch}..."

  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' EXIT

  if command -v curl &>/dev/null; then
    curl -fsSL "${url}" -o "${tmpdir}/hams"
  else
    wget -qO "${tmpdir}/hams" "${url}"
  fi

  chmod +x "${tmpdir}/hams"

  echo "Installing to ${INSTALL_DIR}/hams..."
  if [ -w "${INSTALL_DIR}" ]; then
    mv "${tmpdir}/hams" "${INSTALL_DIR}/hams"
  else
    sudo mv "${tmpdir}/hams" "${INSTALL_DIR}/hams"
  fi
}

main() {
  echo "hams installer"
  echo ""

  local platform version
  platform="$(detect_platform)"
  echo "Detected platform: ${platform}"

  version="$(get_latest_version)"
  if [ -z "${version}" ]; then
    echo "Error: could not determine latest version. Check https://github.com/${REPO}/releases" >&2
    exit 1
  fi
  echo "Latest version: ${version}"

  download_binary "${version}" "${platform}"

  echo ""
  echo "hams installed successfully!"
  echo "Run 'hams --help' to get started."
}

main "$@"
