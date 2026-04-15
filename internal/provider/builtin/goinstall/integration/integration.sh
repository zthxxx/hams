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

# hey and goreleaser are fast-to-install Go modules. If either breaks, swap
# for another small Go binary.
standard_cli_flow goinstall install "github.com/rakyll/hey@latest" "github.com/mgechev/revive@latest"
