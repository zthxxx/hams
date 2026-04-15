#!/usr/bin/env bash
# Integration test for the code-ext (VS Code Extensions) provider.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh
source /e2e/base/lib/provider_flow.sh

echo "=== hams integration test: code-ext (VS Code Extensions) ==="
echo ""

export HAMS_STORE=/tmp/test-codeext-store
export HAMS_MACHINE_ID=e2e-codeext
export HAMS_CONFIG_HOME=/tmp/test-codeext-config

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

assert_output_contains "hams --version" "hams version" hams --version
assert_success "code CLI is on PATH" command -v code

# Post-install check: `code --list-extensions | grep <pkg>`.
# Extensions install into the VS Code CLI's user data dir; there's no PATH
# binary to probe with `command -v`. Export the function so it survives any
# subshell that standard_cli_flow might use.
ext_installed_check() {
  code --list-extensions 2>/dev/null | grep -qiE "^$1\$"
}
export -f ext_installed_check
export POST_INSTALL_CHECK=ext_installed_check

# vscodeext provider has Name=code-ext but FilePrefix=vscodeext, so override
# the state-file name explicitly. Two small well-maintained extensions:
# `vscode-icons-team.vscode-icons` is ubiquitous; `tamasfe.even-better-toml`
# succeeded the deprecated bungcip.better-toml.
export STATE_FILE_PREFIX=vscodeext
standard_cli_flow code-ext install vscode-icons-team.vscode-icons tamasfe.even-better-toml
