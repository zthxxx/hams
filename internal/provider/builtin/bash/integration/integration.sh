#!/usr/bin/env bash
# Integration test for the bash provider (declarative-only; no PATH binary).
#
# bash resources are arbitrary shell commands declared in bash.hams.yaml with
# optional check/remove/sudo fields. This test exercises the declarative
# lifecycle: hams apply runs the command, check persists, refresh re-probes,
# remove runs the remove command. State file updates throughout.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh

echo "=== hams integration test: bash ==="
echo ""

export HAMS_STORE=/tmp/test-bash-store
export HAMS_MACHINE_ID=e2e-bash
export HAMS_CONFIG_HOME=/tmp/test-bash-config

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

BASH_HAMS="$HAMS_STORE/test/bash.hams.yaml"
BASH_STATE="$HAMS_STORE/.state/$HAMS_MACHINE_ID/bash.state.yaml"
MARKER=/tmp/hams-integration-bash-marker

assert_output_contains "hams --version" "hams version" hams --version

# --- Step 1: declare a bash resource with check + remove ---
rm -f "$MARKER"
cat > "$BASH_HAMS" <<YAML
setup:
  - urn: "urn:hams:bash:integration-marker"
    step: "Create integration marker"
    run: "touch ${MARKER}"
    check: "test -f ${MARKER}"
    remove: "rm -f ${MARKER}"
YAML

assert_success "hams apply --only=bash creates marker" \
  hams --store="$HAMS_STORE" apply --only=bash
assert_success "marker file exists" test -f "$MARKER"
assert_yaml_field_eq "bash.state.yaml marker.state=ok" \
  "$BASH_STATE" '.resources."urn:hams:bash:integration-marker".state' 'ok'
FIRST_INSTALL=$(yq -r '.resources."urn:hams:bash:integration-marker".first_install_at' "$BASH_STATE")
echo "  captured first_install_at=$FIRST_INSTALL"

# --- Step 2: refresh re-probes (runs the check again) and bumps updated_at ---
sleep 1
assert_success "hams refresh --only=bash re-probes" \
  hams --store="$HAMS_STORE" refresh --only=bash
AFTER_REFRESH=$(yq -r '.resources."urn:hams:bash:integration-marker".updated_at' "$BASH_STATE")
if [ "$AFTER_REFRESH" \> "$FIRST_INSTALL" ]; then
  echo "  ok: refresh bumped updated_at ($FIRST_INSTALL → $AFTER_REFRESH)"
else
  echo "FAIL: refresh did not bump updated_at"
  exit 1
fi

# --- Step 3: drift detection — delete the marker, refresh should mark pending ---
rm -f "$MARKER"
sleep 1
assert_success "hams refresh after drift" \
  hams --store="$HAMS_STORE" refresh --only=bash
# Bash provider marks state=pending when the check fails on refresh.
DRIFT_STATE=$(yq -r '.resources."urn:hams:bash:integration-marker".state' "$BASH_STATE")
if [ "$DRIFT_STATE" != "pending" ] && [ "$DRIFT_STATE" != "failed" ]; then
  echo "FAIL: refresh after drift — expected state=pending or failed, got $DRIFT_STATE"
  exit 1
fi
echo "  ok: refresh after drift reports state=$DRIFT_STATE"

# --- Step 4: re-apply runs the command again, marker recreated ---
assert_success "hams apply recovers from drift" \
  hams --store="$HAMS_STORE" apply --only=bash
assert_success "marker exists after recovery" test -f "$MARKER"
assert_yaml_field_eq "bash.state.yaml marker.state=ok again" \
  "$BASH_STATE" '.resources."urn:hams:bash:integration-marker".state' 'ok'

# --- Step 5: remove the hamsfile entry + apply → remove command runs ---
cat > "$BASH_HAMS" <<'YAML'
setup: []
YAML
assert_success "hams apply after hamsfile edit runs remove command" \
  hams --store="$HAMS_STORE" apply --only=bash
if [ -f "$MARKER" ]; then
  echo "FAIL: marker file should have been removed"
  exit 1
fi
echo "  ok: marker removed via bash remove command"
assert_yaml_field_eq "bash.state.yaml marker.state=removed" \
  "$BASH_STATE" '.resources."urn:hams:bash:integration-marker".state' 'removed'

echo ""
echo "=== bash integration test passed ==="
