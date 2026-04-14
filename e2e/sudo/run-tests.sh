#!/usr/bin/env bash
set -euo pipefail

echo "=== Running sudo tests as root ==="
go test -v -tags=sudo -race -count=1 -run 'TestAcquire_AsRoot|TestBuilder_AsRoot' ./internal/sudo/...

echo ""
echo "=== Running sudo tests as non-root (testuser with NOPASSWD) ==="
su testuser -c 'cd /src && go test -v -tags=sudo -race -count=1 -run "TestAcquire_AsNonRoot_WithNOPASSWD|TestBuilder_AsNonRoot" ./internal/sudo/...'

echo ""
echo "=== Running sudo tests as non-root WITHOUT sudo access (nosudouser) ==="
su nosudouser -c 'cd /src && go test -v -tags=sudo -race -count=1 -run "TestAcquire_AsNonRoot_WithoutSudo" ./internal/sudo/...'

echo ""
echo "=== Running apt provider tests with real sudo (as root) ==="
go test -v -tags=sudo -race -count=1 -run 'TestApt_' ./internal/provider/builtin/apt/...

echo ""
echo "All sudo tests passed."
