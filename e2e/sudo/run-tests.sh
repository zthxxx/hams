#!/usr/bin/env bash
set -euo pipefail

echo "=== Running sudo tests as root ==="
go test -v -tags=sudo -race -count=1 -run 'TestAcquire_AsRoot|TestBuilder_AsRoot' ./internal/sudo/...

echo ""
echo "=== Running sudo tests as non-root (testuser with NOPASSWD) ==="
su testuser -c 'cd /src && go test -v -tags=sudo -race -count=1 -run "TestAcquire_AsNonRoot|TestBuilder_AsNonRoot" ./internal/sudo/...'

echo ""
echo "All sudo tests passed."
