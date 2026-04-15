#!/usr/bin/env bash
# Integration test for the cargo provider.
# Uses standard_cli_flow with two lightweight cargo-installable binaries.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh
source /e2e/base/lib/provider_flow.sh

echo "=== hams integration test: cargo ==="
echo ""

export HAMS_STORE=/tmp/test-cargo-store
export HAMS_MACHINE_ID=e2e-cargo
export HAMS_CONFIG_HOME=/tmp/test-cargo-config
export CARGO_HOME=/root/.cargo
export PATH="/root/.cargo/bin:${PATH}"

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

assert_output_contains "hams --version" "hams version" hams --version
assert_success "cargo is on PATH" command -v cargo

# xsv and xcp are small Rust tools that install in <1 min on a warm cache.
# If either starts timing out on CI, swap them for anything lighter.
standard_cli_flow cargo install xsv xcp
