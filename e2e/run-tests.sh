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
echo "=== All E2E tests passed ==="
