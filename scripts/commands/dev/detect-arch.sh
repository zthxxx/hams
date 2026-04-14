#!/usr/bin/env bash
#
# detect-arch.sh — map the host's uname -m to a linux GOARCH.
#
# Used by scripts/commands/dev/main.sh so the host-driven `go build`
# and the container both agree on what arch the in-container hams
# binary should be.
#
# Output on stdout is exactly one of: amd64, arm64
# Exits non-zero on unknown architectures.
set -Eeuo pipefail

arch="$(uname -m)"
case "${arch}" in
  x86_64 | amd64)
    echo "amd64"
    ;;
  aarch64 | arm64)
    echo "arm64"
    ;;
  *)
    printf 'detect-arch: unsupported architecture %q (want x86_64/amd64 or aarch64/arm64)\n' "${arch}" >&2
    exit 1
    ;;
esac
