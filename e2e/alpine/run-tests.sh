#!/usr/bin/env bash
set -euo pipefail
source /e2e/base/lib/assertions.sh

echo "=== hams E2E Test (Alpine) ==="
echo ""

# --- CLI smoke tests ---
run_smoke_tests

# --- Bootstrap from local git repo with Alpine-specific providers ---
echo "--- Alpine provider tests (bash + git-config) ---"
STORE_DIR=/tmp/test-hams-store
create_store_repo "$STORE_DIR" "/fixtures/alpine-store" "e2e-alpine"

# Apply with bash + git-config providers.
assert_success "hams apply --from-repo (bash + git-config)" \
  hams apply --from-repo="$STORE_DIR" --only=bash,git-config

# --- Verify bash provider ---
verify_bash_marker

# --- Verify git config ---
verify_git_config

# --- Idempotent re-apply ---
verify_idempotent_reapply "$STORE_DIR" "bash,git-config"

# --- Config set/get round-trip ---
verify_config_roundtrip

# --- List command ---
assert_success "hams list --only=bash,git-config" \
  hams --store="$STORE_DIR" list --only=bash,git-config

echo "=== All Alpine E2E tests passed ==="
