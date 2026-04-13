#!/usr/bin/env bash
set -euo pipefail

APP_NAME="hams"
BUILD_DIR="${BUILD_DIR:-bin}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
DATE="${DATE:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"

LDFLAGS="-s -w \
  -X github.com/zthxxx/hams/internal/version.version=${VERSION} \
  -X github.com/zthxxx/hams/internal/version.commit=${COMMIT} \
  -X github.com/zthxxx/hams/internal/version.date=${DATE}"

TARGETS=(
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

mkdir -p "${BUILD_DIR}"

for target in "${TARGETS[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  output="${BUILD_DIR}/${APP_NAME}-${os}-${arch}"

  echo "Building ${output}..."
  CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" \
    go build -ldflags "${LDFLAGS}" -o "${output}" ./cmd/hams
done

echo "Build complete. Binaries in ${BUILD_DIR}/"
ls -lh "${BUILD_DIR}/${APP_NAME}-"*
