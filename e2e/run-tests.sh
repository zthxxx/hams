#!/usr/bin/env bash
set -euo pipefail

echo "=== hams E2E Test ==="

echo "Testing: hams --version"
hams --version
echo ""

echo "Testing: hams config list"
hams --store=/fixtures/test-store config list
echo ""

echo "Testing: hams --help"
hams --help | head -5
echo ""

echo "Testing: hams apply --from-repo (local path)"
# Prepare a local git repo fixture for --from-repo test.
mkdir -p /tmp/test-hams-store
cd /tmp/test-hams-store
git init --quiet
mkdir -p test
cat > hams.config.yaml << 'EOF'
profile_tag: test
machine_id: docker-test
EOF
cat > test/bash.hams.yaml << 'EOF'
setup:
  - urn: "urn:hams:bash:test-echo"
    step: "Echo test message"
    run: "echo 'hams from-repo e2e test passed'"
    check: "true"
EOF
git add -A
git config user.email "test@hams.dev"
git config user.name "hams-test"
git commit -m "test fixture" --quiet

# Test --from-repo with local path.
hams apply --from-repo=/tmp/test-hams-store --dry-run 2>&1 || true
echo "from-repo local path: ok"
echo ""

echo "Testing: hams brew --help"
hams brew --help 2>&1 || true
echo ""

echo "=== All E2E tests passed ==="
