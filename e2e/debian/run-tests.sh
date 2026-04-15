#!/usr/bin/env bash
# Debian OS-level E2E: cross-provider bootstrap smoke + config-scope rejection.
# Per-provider imperative scenarios live next to each provider at
# `internal/provider/builtin/<provider>/integration/integration.sh` and run
# via `task ci:itest:run PROVIDER=<name>`.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh
source /e2e/debian/assert-config-scope.sh

echo "=== hams E2E Test (Debian) ==="
echo ""

# --- CLI smoke tests ---
run_smoke_tests

# --- Bootstrap from local git repo with Debian-specific providers ---
echo "--- Debian provider tests (apt + bash + git-config) ---"
STORE_DIR=/tmp/test-hams-store
create_store_repo "$STORE_DIR" "/fixtures/debian-store" "e2e-debian"

# apt-get update is required because the Docker image prunes package lists.
apt-get update -qq

# Apply with apt + bash + git-config providers.
assert_success "hams apply --from-repo (apt + bash + git-config)" \
  hams apply --from-repo="$STORE_DIR" --only=apt,bash,git-config

# --- Verify apt installation ---
assert_success "jq is installed via apt" jq --version

# --- Verify bash provider ---
verify_bash_marker

# --- Verify git config ---
verify_git_config

# --- Idempotent re-apply ---
verify_idempotent_reapply "$STORE_DIR" "apt,bash,git-config"

# --- Config set/get round-trip ---
verify_config_roundtrip

# --- List command ---
assert_success "hams list --only=apt,bash,git-config" \
  hams --store="$STORE_DIR" list --only=apt,bash,git-config

# --- Cross-provider store-config scope rejection ---
run_config_scope_tests "$STORE_DIR"

echo "=== All Debian E2E tests passed ==="
