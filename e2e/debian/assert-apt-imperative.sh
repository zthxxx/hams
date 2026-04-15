#!/usr/bin/env bash
# Debian E2E scenarios for the imperative apt flow + store-level config
# rejection + state schema v1 → v2 migration.
#
# Source this file and call run_apt_imperative_tests <STORE_DIR>.

# run_apt_imperative_tests exercises:
#   E1 install bat + apply  → state ok + first_install_at set, no removed_at
#   E2 remove bat + apply   → state removed + removed_at set, first_install_at preserved
#   E3 install bat again    → state ok, first_install_at unchanged, removed_at cleared
#   E4 store-level profile_tag rejected with actionable error
#   E5 synthetic v1 state file is migrated to v2 on next apply
run_apt_imperative_tests() {
  local store_dir="$1"
  local profile_dir="$store_dir/test"
  local state_dir="$store_dir/.state/e2e-debian"
  local apt_hams="$profile_dir/apt.hams.yaml"
  local apt_state="$state_dir/apt.state.yaml"
  local pkg="bat"

  echo ""
  echo "--- apt imperative scenarios (install/remove/re-install cycle) ---"

  # Ensure the apt cache is primed so install of 'bat' does not fail.
  apt-get update -qq

  # ------------------------------------------------------------------------
  # E1: imperative install records + reconciles state with first_install_at.
  # ------------------------------------------------------------------------
  echo ""
  echo "E1: hams apt install $pkg"
  assert_success "hams apt install $pkg" \
    hams --store="$store_dir" apt install "$pkg"

  assert_success "$pkg is in PATH after imperative install" command -v "$pkg"

  # Reconcile state via apply.
  assert_success "hams apply after imperative install" \
    hams --store="$store_dir" apply --only=apt

  # apt.hams.yaml must contain the app.
  assert_yaml_field_eq "apt.hams.yaml has bat entry" \
    "$apt_hams" '.cli[] | select(.app == "bat") | .app' 'bat'

  # apt.state.yaml: state=ok, first_install_at set, no removed_at.
  assert_yaml_field_eq "apt.state.yaml schema_version=2" \
    "$apt_state" '.schema_version' '2'
  assert_yaml_field_eq "apt.state.yaml bat.state=ok" \
    "$apt_state" '.resources.bat.state' 'ok'
  assert_yaml_field_present "apt.state.yaml bat.first_install_at set" \
    "$apt_state" '.resources.bat.first_install_at'
  assert_yaml_field_absent "apt.state.yaml bat.removed_at absent after install" \
    "$apt_state" '.resources.bat.removed_at'

  # Capture the first_install_at value for E2/E3 immutability check.
  local first_install_at
  first_install_at=$(yq -r '.resources.bat.first_install_at' "$apt_state")
  echo "  captured first_install_at=$first_install_at"

  # Sleep one second so subsequent timestamps are strictly greater
  # (format YYYYMMDDTHHmmss resolves to seconds).
  sleep 1

  # ------------------------------------------------------------------------
  # E2: imperative remove flips state to removed + records removed_at.
  # ------------------------------------------------------------------------
  echo ""
  echo "E2: hams apt remove $pkg"
  assert_success "hams apt remove $pkg" \
    hams --store="$store_dir" apt remove "$pkg"

  if command -v "$pkg" >/dev/null 2>&1; then
    echo "FAIL: E2 — $pkg is still in PATH after remove"
    exit 1
  fi
  echo "  ok: $pkg is no longer in PATH"

  assert_success "hams apply after imperative remove" \
    hams --store="$store_dir" apply --only=apt

  assert_yaml_field_eq "apt.state.yaml bat.state=removed" \
    "$apt_state" '.resources.bat.state' 'removed'
  assert_yaml_field_eq "apt.state.yaml bat.first_install_at unchanged" \
    "$apt_state" '.resources.bat.first_install_at' "$first_install_at"
  assert_yaml_field_present "apt.state.yaml bat.removed_at set" \
    "$apt_state" '.resources.bat.removed_at'
  assert_yaml_field_lex_gt "apt.state.yaml bat.updated_at > first_install_at" \
    "$apt_state" '.resources.bat.updated_at' '.resources.bat.first_install_at'

  sleep 1

  # ------------------------------------------------------------------------
  # E3: re-install after remove clears removed_at, preserves first_install_at.
  # ------------------------------------------------------------------------
  echo ""
  echo "E3: hams apt install $pkg (re-install after remove)"
  assert_success "hams apt install $pkg after remove" \
    hams --store="$store_dir" apt install "$pkg"
  assert_success "$pkg is in PATH again" command -v "$pkg"
  assert_success "hams apply after re-install" \
    hams --store="$store_dir" apply --only=apt

  assert_yaml_field_eq "apt.state.yaml bat.state=ok" \
    "$apt_state" '.resources.bat.state' 'ok'
  assert_yaml_field_eq "apt.state.yaml bat.first_install_at still immutable" \
    "$apt_state" '.resources.bat.first_install_at' "$first_install_at"
  assert_yaml_field_absent "apt.state.yaml bat.removed_at cleared" \
    "$apt_state" '.resources.bat.removed_at'

  # Cleanup: remove bat so subsequent runs start clean.
  hams --store="$store_dir" apt remove "$pkg" || true

  # ------------------------------------------------------------------------
  # E4: store-level profile_tag in <store>/hams.config.yaml is rejected.
  # ------------------------------------------------------------------------
  echo ""
  echo "E4: store-level profile_tag hard-fails at load"
  local store_config="$store_dir/hams.config.yaml"
  local backup="$store_dir/hams.config.yaml.bak"
  cp "$store_config" "$backup"
  cat > "$store_config" <<'YAML'
profile_tag: dev
YAML

  local output rc
  output=$(hams --store="$store_dir" config list 2>&1 || true)
  rc=$?
  if [ $rc -eq 0 ]; then
    echo "FAIL: E4 — hams should exit non-zero with store-level profile_tag"
    echo "  output: $output"
    cp "$backup" "$store_config"
    rm "$backup"
    exit 1
  fi
  if ! printf '%s\n' "$output" | grep -qF 'profile_tag'; then
    echo "FAIL: E4 — error should name profile_tag"
    echo "  output: $output"
    cp "$backup" "$store_config"
    rm "$backup"
    exit 1
  fi
  if ! printf '%s\n' "$output" | grep -qF "$store_config"; then
    echo "FAIL: E4 — error should contain offending file path $store_config"
    echo "  output: $output"
    cp "$backup" "$store_config"
    rm "$backup"
    exit 1
  fi
  if ! printf '%s\n' "$output" | grep -qF 'hams.config.yaml'; then
    echo "FAIL: E4 — error should point to global hams.config.yaml"
    echo "  output: $output"
    cp "$backup" "$store_config"
    rm "$backup"
    exit 1
  fi
  echo "  ok: store-level profile_tag rejected with actionable error"

  # Restore the fixture.
  cp "$backup" "$store_config"
  rm "$backup"

  # ------------------------------------------------------------------------
  # E5: synthetic schema_version:1 state file is auto-migrated to v2 on load.
  # ------------------------------------------------------------------------
  echo ""
  echo "E5: state schema v1 → v2 auto-migration"
  local migration_state="$state_dir/apt.state.yaml"
  mkdir -p "$state_dir"
  cat > "$migration_state" <<'YAML'
schema_version: 1
provider: apt
machine_id: e2e-debian
resources:
  bat:
    state: removed
    install_at: "20200101T000000"
    updated_at: "20200101T000000"
YAML
  # A refresh re-reads the state file and rewrites it via Save.
  assert_success "hams refresh triggers v1→v2 migration" \
    hams --store="$store_dir" refresh --only=apt

  assert_yaml_field_eq "migrated file schema_version=2" \
    "$migration_state" '.schema_version' '2'
  assert_yaml_field_eq "migrated bat.first_install_at preserved" \
    "$migration_state" '.resources.bat.first_install_at' '20200101T000000'
  if grep -qE '^[[:space:]]*install_at:' "$migration_state"; then
    echo "FAIL: E5 — legacy install_at key still present after migration"
    cat "$migration_state"
    exit 1
  fi
  echo "  ok: legacy install_at key removed after migration"

  echo ""
  echo "--- apt imperative scenarios passed ---"
}
