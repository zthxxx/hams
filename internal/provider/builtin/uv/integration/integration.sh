#!/usr/bin/env bash
# Integration test for the uv provider.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh
source /e2e/base/lib/provider_flow.sh

echo "=== hams integration test: uv ==="
echo ""

export HAMS_STORE=/tmp/test-uv-store
export HAMS_MACHINE_ID=e2e-uv
export HAMS_CONFIG_HOME=/tmp/test-uv-config
export PATH="/root/.local/bin:${PATH}"

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

assert_output_contains "hams --version" "hams version" hams --version
assert_success "uv is on PATH" command -v uv

# ruff (linter) and tomli-lint are tiny uv-installable CLIs.
standard_cli_flow uv install ruff tomli-lint
