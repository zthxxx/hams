#!/usr/bin/env bash
# Integration test for the apt provider. Runs inside hams-itest-apt.
#
# Exercises:
#   - the canonical provider flow via standard_cli_flow:
#     seed jq, re-install jq (updated_at bumps), install btop (new row),
#     refresh (updated_at bumps), remove btop (removed_at set).
#   - apt-specific lifecycle scenarios (install htop / remove / re-install)
#     asserting first_install_at immutability + removed_at tracking.
#   - state schema v1 → v2 migration via `hams refresh`.
#
# All state-file assertions are driven by the shared helpers at
# /e2e/base/lib/. No `hams apply` is used anywhere in this script —
# the imperative CLI is expected to reconcile state on its own.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh
source /e2e/base/lib/provider_flow.sh

echo "=== hams integration test: apt ==="
echo ""

# -----------------------------------------------------------------------
# Test environment
# -----------------------------------------------------------------------
export HAMS_STORE=/tmp/test-apt-store
export HAMS_MACHINE_ID=e2e-apt
export HAMS_CONFIG_HOME=/tmp/test-apt-config

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
# Store-level config. Machine-scoped fields live in $HAMS_CONFIG_HOME.
YAML

# Smoke: the mounted hams binary runs + reports its version.
assert_output_contains "hams --version" "hams version" hams --version

# -----------------------------------------------------------------------
# apt cache prime
# -----------------------------------------------------------------------
apt-get update -qq

# -----------------------------------------------------------------------
# Canonical provider flow (seed jq, install/refresh/remove btop).
# -----------------------------------------------------------------------
standard_cli_flow apt install jq btop

# -----------------------------------------------------------------------
# Apt-specific scenarios: E1 install htop / E2 remove / E3 re-install
# asserting first_install_at immutability and removed_at tracking.
# Historically lived in e2e/debian/assert-apt-imperative.sh; moved here
# now that apt owns its own integration test.
# -----------------------------------------------------------------------
APT_HAMS="$HAMS_STORE/test/apt.hams.yaml"
APT_STATE="$HAMS_STORE/.state/$HAMS_MACHINE_ID/apt.state.yaml"

echo ""
echo "--- apt imperative scenarios (htop install/remove/re-install cycle) ---"

echo ""
echo "E1: hams apt install htop writes hamsfile and state"
assert_success "hams apt install htop" \
  hams --store="$HAMS_STORE" apt install htop
assert_success "htop is in PATH after imperative install" command -v htop
assert_yaml_field_eq "apt.hams.yaml has htop entry" \
  "$APT_HAMS" '.cli[] | select(.app == "htop") | .app' 'htop'
assert_yaml_field_eq "apt.state.yaml schema_version=2" \
  "$APT_STATE" '.schema_version' '2'
assert_yaml_field_eq "apt.state.yaml htop.state=ok" \
  "$APT_STATE" '.resources.htop.state' 'ok'
assert_yaml_field_present "apt.state.yaml htop.first_install_at set" \
  "$APT_STATE" '.resources.htop.first_install_at'
assert_yaml_field_absent "apt.state.yaml htop.removed_at absent after install" \
  "$APT_STATE" '.resources.htop.removed_at'

HTOP_FIRST_INSTALL=$(yq -r '.resources.htop.first_install_at' "$APT_STATE")
echo "  captured first_install_at=$HTOP_FIRST_INSTALL"
sleep 1

echo ""
echo "E2: hams apt remove htop writes state directly"
assert_success "hams apt remove htop" \
  hams --store="$HAMS_STORE" apt remove htop
if command -v htop >/dev/null 2>&1; then
  echo "FAIL: E2 — htop is still in PATH after remove"
  exit 1
fi
echo "  ok: htop is no longer in PATH"
assert_yaml_field_eq "apt.state.yaml htop.state=removed" \
  "$APT_STATE" '.resources.htop.state' 'removed'
assert_yaml_field_eq "apt.state.yaml htop.first_install_at unchanged" \
  "$APT_STATE" '.resources.htop.first_install_at' "$HTOP_FIRST_INSTALL"
assert_yaml_field_present "apt.state.yaml htop.removed_at set" \
  "$APT_STATE" '.resources.htop.removed_at'
assert_yaml_field_lex_gt "apt.state.yaml htop.updated_at > first_install_at" \
  "$APT_STATE" '.resources.htop.updated_at' '.resources.htop.first_install_at'
sleep 1

echo ""
echo "E3: hams apt install htop again clears removed_at"
assert_success "hams apt install htop after remove" \
  hams --store="$HAMS_STORE" apt install htop
assert_success "htop is in PATH again" command -v htop
assert_yaml_field_eq "apt.state.yaml htop.state=ok" \
  "$APT_STATE" '.resources.htop.state' 'ok'
assert_yaml_field_eq "apt.state.yaml htop.first_install_at still immutable" \
  "$APT_STATE" '.resources.htop.first_install_at' "$HTOP_FIRST_INSTALL"
assert_yaml_field_absent "apt.state.yaml htop.removed_at cleared" \
  "$APT_STATE" '.resources.htop.removed_at'

# Cleanup so subsequent test runs start clean. We tolerate failure here
# because the test is essentially done; an apt-get remove glitch on the
# very last cleanup step shouldn't mask the success of the actual
# scenarios above. If this becomes a problem, switch to `|| log_warning`.
hams --store="$HAMS_STORE" apt remove htop || true

# -----------------------------------------------------------------------
# Schema v1 → v2 auto-migration on refresh.
# Synthetic v1 file written directly to disk; refresh re-reads, migrates,
# and rewrites as v2. Moved from e2e/debian/assert-apt-imperative.sh::E5.
# -----------------------------------------------------------------------
echo ""
echo "E5: state schema v1 → v2 auto-migration"
mkdir -p "$HAMS_STORE/.state/$HAMS_MACHINE_ID"
cat > "$APT_STATE" <<'YAML'
schema_version: 1
provider: apt
machine_id: e2e-apt
resources:
  htop:
    state: removed
    install_at: "20200101T000000"
    updated_at: "20200101T000000"
YAML
assert_success "hams refresh triggers v1→v2 migration" \
  hams --store="$HAMS_STORE" refresh --only=apt
assert_yaml_field_eq "migrated file schema_version=2" \
  "$APT_STATE" '.schema_version' '2'
assert_yaml_field_eq "migrated htop.first_install_at preserved" \
  "$APT_STATE" '.resources.htop.first_install_at' '20200101T000000'
if grep -qE '^[[:space:]]*install_at:' "$APT_STATE"; then
  echo "FAIL: E5 — legacy install_at key still present after migration"
  cat "$APT_STATE"
  exit 1
fi
echo "  ok: legacy install_at key removed after migration"

# -----------------------------------------------------------------------
# E6: --prune-orphans destructive opt-in. Default skip; flag uninstalls.
# -----------------------------------------------------------------------
echo ""
echo "E6: hams apply --prune-orphans removes state-only resources"

# Reset state from the v1-migration probe above.
rm -f "$APT_STATE"

assert_success "seed install of jq via CLI" \
  hams --store="$HAMS_STORE" apt install jq
assert_yaml_field_eq "jq.state=ok after seed install" \
  "$APT_STATE" '.resources.jq.state' 'ok'
assert_success "jq is on PATH" command -v jq

# Delete the hamsfile to create the state-only scenario.
rm -f "$APT_HAMS"

# Default: skip. State and PATH must be unchanged.
assert_success "hams apply (no flag) skips state-only providers" \
  hams --store="$HAMS_STORE" apply --only=apt
assert_yaml_field_eq "jq.state still ok after default apply" \
  "$APT_STATE" '.resources.jq.state' 'ok'
assert_success "jq still on PATH after default apply" command -v jq

# Opt-in: --prune-orphans must remove jq from both apt and state.
assert_success "hams apply --prune-orphans removes jq" \
  hams --store="$HAMS_STORE" apply --only=apt --prune-orphans
assert_yaml_field_eq "jq.state=removed after prune" \
  "$APT_STATE" '.resources.jq.state' 'removed'
assert_yaml_field_present "jq.removed_at set after prune" \
  "$APT_STATE" '.resources.jq.removed_at'
if command -v jq >/dev/null 2>&1; then
  echo "FAIL: E6 — jq is still in PATH after --prune-orphans"
  exit 1
fi
echo "  ok: jq is no longer in PATH"

# -----------------------------------------------------------------------
# E7: --prune-orphans flow leaves no state row when the hamsfile is
# already empty AND we want to verify version-pin recording lands cleanly
# in a fresh hamsfile + state pair.
# -----------------------------------------------------------------------
echo ""
echo "E7: hams apt install <pkg>=<version> records structured version pin"

# Reset hamsfile + state.
rm -f "$APT_HAMS" "$APT_STATE"

# Pick a version that's actually available in the bookworm archive.
# Probe apt-cache for jq's installed-candidate version, then pin to it
# so the install succeeds deterministically.
JQ_VER=$(apt-cache madison jq 2>/dev/null | head -1 | awk '{print $3}')
if [ -z "$JQ_VER" ]; then
  echo "  skipped: apt-cache madison returned no candidate version for jq"
else
  echo "  pinning jq to candidate version: $JQ_VER"

  assert_success "hams apt install jq=<pin>" \
    hams --store="$HAMS_STORE" apt install "jq=$JQ_VER"

  assert_yaml_field_eq "hamsfile records jq.version pin" \
    "$APT_HAMS" '.cli[] | select(.app == "jq") | .version' "$JQ_VER"
  assert_yaml_field_eq "state records jq.requested_version pin" \
    "$APT_STATE" '.resources.jq.requested_version' "$JQ_VER"
  assert_yaml_field_eq "state observed jq.version matches the pin" \
    "$APT_STATE" '.resources.jq.version' "$JQ_VER"

  # Drift-free apply: jq is already pinned + installed; expect Skip.
  assert_success "hams apply --only=apt is a no-op when state matches pin" \
    hams --store="$HAMS_STORE" apply --only=apt
  assert_yaml_field_eq "jq.state still ok after no-op apply" \
    "$APT_STATE" '.resources.jq.state' 'ok'

  # ---------------------------------------------------------------------
  # Sub-section: fresh-machine restore replays the hamsfile pin.
  # Simulates `hams apply --from-repo=...` on a brand-new machine — the
  # cycle-3 implementation silently dropped the pin here because Plan
  # only saw `desired.ListApps()` (names, no structured fields). The
  # cycle-4 fix uses `AppFields` + `action.Resource`.
  # ---------------------------------------------------------------------
  echo ""
  echo "  E7.b: fresh-machine restore replays the hamsfile pin"

  # Save the pinned hamsfile content (this is what would be in the store
  # repo on a real fresh-machine restore).
  PINNED_HAMS_CONTENT=$(cat "$APT_HAMS")

  # Wipe state + hamsfile + un-pin the host so apply has real work.
  rm -f "$APT_STATE" "$APT_HAMS"
  apt-get remove -y jq >/dev/null
  if command -v jq >/dev/null 2>&1; then
    echo "  FAIL: precondition — apt-get remove jq did not actually uninstall"
    exit 1
  fi

  # Restore the pinned hamsfile (simulate the post-clone state).
  printf '%s\n' "$PINNED_HAMS_CONTENT" > "$APT_HAMS"

  # Apply: the executor must read the pin out of the hamsfile and
  # invoke apt-get with `jq=<pin>`.
  assert_success "hams apply --only=apt restores the pinned version" \
    hams --store="$HAMS_STORE" apply --only=apt
  assert_success "jq is on PATH after restore" command -v jq

  INSTALLED_VER=$(dpkg-query -W -f='${Version}' jq)
  if [ "$INSTALLED_VER" != "$JQ_VER" ]; then
    echo "  FAIL: restored version $INSTALLED_VER, want pinned $JQ_VER"
    exit 1
  fi
  echo "  ok: dpkg reports the pinned version after restore: $INSTALLED_VER"

  assert_yaml_field_eq "state records the pin after restore" \
    "$APT_STATE" '.resources.jq.requested_version' "$JQ_VER"

  # State must have a SINGLE row keyed on the bare name — no orphan
  # `jq=<ver>` row from the cycle-3 ID-mutation bug.
  ROW_COUNT=$(yq '.resources | keys | length' "$APT_STATE")
  if [ "$ROW_COUNT" != "1" ]; then
    echo "  FAIL: state has $ROW_COUNT resource rows for jq, want exactly 1"
    yq '.resources | keys' "$APT_STATE"
    exit 1
  fi
  ROW_KEY=$(yq -r '.resources | keys | .[0]' "$APT_STATE")
  if [ "$ROW_KEY" != "jq" ]; then
    echo "  FAIL: state row key = $ROW_KEY, want bare 'jq' (pin must NOT be in state key)"
    exit 1
  fi
  echo "  ok: state has exactly one row keyed on the bare 'jq' name"
fi

echo ""
echo "=== apt integration test passed ==="
