#!/usr/bin/env bash
# Integration test for the pnpm provider.
# pnpm CLI uses `add` (alias: `install`, `i`). standard_cli_flow accepts the
# verb explicitly so the same helper works.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh
source /e2e/base/lib/provider_flow.sh

echo "=== hams integration test: pnpm ==="
echo ""

export HAMS_STORE=/tmp/test-pnpm-store
export HAMS_MACHINE_ID=e2e-pnpm
export HAMS_CONFIG_HOME=/tmp/test-pnpm-config

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

assert_output_contains "hams --version" "hams version" hams --version
assert_success "pnpm is on PATH" command -v pnpm

standard_cli_flow pnpm add serve nodemon
