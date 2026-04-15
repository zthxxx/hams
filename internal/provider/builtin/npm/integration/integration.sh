#!/usr/bin/env bash
# Integration test for the npm provider.
# Uses standard_cli_flow with small npm global packages.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh
source /e2e/base/lib/provider_flow.sh

echo "=== hams integration test: npm ==="
echo ""

export HAMS_STORE=/tmp/test-npm-store
export HAMS_MACHINE_ID=e2e-npm
export HAMS_CONFIG_HOME=/tmp/test-npm-config

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

assert_output_contains "hams --version" "hams version" hams --version
assert_success "npm is on PATH" command -v npm

# `serve` and `sort-package-json` are tiny global CLIs — fast to install.
standard_cli_flow npm install serve sort-package-json
