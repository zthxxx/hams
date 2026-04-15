#!/usr/bin/env bash
# Integration test for the goinstall provider.
# Uses standard_cli_flow with two small Go-installable binaries.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh
source /e2e/base/lib/provider_flow.sh

echo "=== hams integration test: goinstall ==="
echo ""

export HAMS_STORE=/tmp/test-goinstall-store
export HAMS_MACHINE_ID=e2e-goinstall
export HAMS_CONFIG_HOME=/tmp/test-goinstall-config
export GOPATH=/root/go
export PATH="/usr/local/go/bin:${GOPATH}/bin:${PATH}"

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME" "$GOPATH/bin"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

assert_output_contains "hams --version" "hams version" hams --version
assert_success "go is on PATH" command -v go

# go install <module>@version produces a binary named after the last
# path component (before @version), e.g. github.com/rakyll/hey@latest
# installs /root/go/bin/hey. default_post_install_check would look for
# a literal binary of the full module path, which doesn't exist. Supply
# a custom check that parses the binary name out of the module path.
goinstall_installed_check() {
  local pkg="$1"
  local no_ver="${pkg%@*}"         # strip @version
  local bin="${no_ver##*/}"        # last path segment
  command -v "$bin" >/dev/null 2>&1
}
export -f goinstall_installed_check
export POST_INSTALL_CHECK=goinstall_installed_check

# hey (load gen) + revive (lint). Revive pulls in Go 1.25 via the
# embedded toolchain directive — slower than hey but still sub-minute
# on a warm cache.
standard_cli_flow goinstall install "github.com/rakyll/hey@latest" "github.com/mgechev/revive@latest"
