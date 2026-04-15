#!/usr/bin/env bash
# Integration test for the ansible provider.
#
# Ansible has no imperative CLI install/remove — it runs playbooks via
# `hams ansible <playbook.yml>` (one-shot) or declaratively via
# `hams apply` with an ansible.hams.yaml that lists playbooks.
# This test exercises both paths.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh

echo "=== hams integration test: ansible ==="
echo ""

export HAMS_STORE=/tmp/test-ansible-store
export HAMS_MACHINE_ID=e2e-ansible
export HAMS_CONFIG_HOME=/tmp/test-ansible-config

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

assert_output_contains "hams --version" "hams version" hams --version
assert_success "ansible-playbook is on PATH" command -v ansible-playbook

# --- Playbook fixture: creates a marker file on localhost ---
PLAYBOOK=/tmp/integration-ansible-playbook.yml
MARKER=/tmp/hams-integration-ansible-marker
rm -f "$MARKER"
cat > "$PLAYBOOK" <<'YAML'
---
- name: hams ansible integration
  hosts: localhost
  connection: local
  gather_facts: false
  tasks:
    - name: touch marker
      ansible.builtin.file:
        path: /tmp/hams-integration-ansible-marker
        state: touch
YAML

# --- Step 1: one-shot `hams ansible <playbook>` runs the playbook directly ---
assert_success "hams ansible <playbook> (one-shot)" \
  hams --store="$HAMS_STORE" ansible "$PLAYBOOK"
assert_success "one-shot run created the marker" test -f "$MARKER"
rm -f "$MARKER"

# --- Step 2: declarative — declare the playbook in ansible.hams.yaml ---
ANSIBLE_HAMS="$HAMS_STORE/test/ansible.hams.yaml"
ANSIBLE_STATE="$HAMS_STORE/.state/$HAMS_MACHINE_ID/ansible.state.yaml"
cat > "$ANSIBLE_HAMS" <<YAML
playbooks:
  - app: "${PLAYBOOK}"
YAML

assert_success "hams apply --only=ansible runs the playbook" \
  hams --store="$HAMS_STORE" apply --only=ansible
assert_success "apply created the marker" test -f "$MARKER"
# Use bracket-notation for the resource key — the playbook path contains
# `.` which yq would otherwise interpret as a child-lookup separator.
assert_yaml_field_eq "ansible.state records the playbook" \
  "$ANSIBLE_STATE" ".resources[\"${PLAYBOOK}\"].state" 'ok'

FIRST_INSTALL=$(yq -r ".resources[\"${PLAYBOOK}\"].first_install_at" "$ANSIBLE_STATE")

# --- Step 3: refresh re-probes (no-op for ansible since probe returns state unchanged) ---
sleep 1
assert_success "hams refresh --only=ansible" \
  hams --store="$HAMS_STORE" refresh --only=ansible
AFTER_REFRESH=$(yq -r ".resources[\"${PLAYBOOK}\"].updated_at" "$ANSIBLE_STATE")
if [ "$AFTER_REFRESH" \> "$FIRST_INSTALL" ]; then
  echo "  ok: refresh bumped ansible updated_at"
else
  echo "FAIL: refresh did not bump ansible updated_at"
  exit 1
fi

echo ""
echo "=== ansible integration test passed ==="
