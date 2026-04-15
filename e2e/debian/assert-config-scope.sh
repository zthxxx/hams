#!/usr/bin/env bash
# Debian E2E scenario: store-level config scope rejection.
#
# Verifies that hams hard-fails when a machine-scoped field
# (profile_tag / machine_id) appears in the store-level config file —
# those fields belong in $HAMS_CONFIG_HOME/hams.config.yaml only.
#
# This concern is cross-provider (config loader), not apt-specific;
# per-provider imperative scenarios live next to each provider at
# `internal/provider/builtin/<provider>/integration/integration.sh`.
#
# Source this file and call `run_config_scope_tests <STORE_DIR>`.

run_config_scope_tests() {
  local store_dir="$1"

  echo ""
  echo "--- config scope rejection (store-level machine-scoped fields) ---"

  local store_config="$store_dir/hams.config.yaml"
  local backup="$store_dir/hams.config.yaml.bak"
  cp "$store_config" "$backup"
  cat > "$store_config" <<'YAML'
profile_tag: dev
YAML

  # Capture both output and exit code. Avoid two bash gotchas:
  #   1. `cmd || true` masks the real exit code.
  #   2. `if ! output=$(cmd); then rc=$?; fi` reads $? AFTER the `!` flip
  #      so rc is always 0 inside the then-branch.
  # `cmd || rc=$?` is the reliable pattern.
  local output rc=0
  output=$(hams --store="$store_dir" config list 2>&1) || rc=$?

  if [ "$rc" -eq 0 ]; then
    echo "FAIL: hams should exit non-zero when store-level config sets profile_tag"
    echo "  output: $output"
    cp "$backup" "$store_config"
    rm "$backup"
    exit 1
  fi
  if ! printf '%s\n' "$output" | grep -qF 'profile_tag'; then
    echo "FAIL: error should name profile_tag"
    echo "  output: $output"
    cp "$backup" "$store_config"
    rm "$backup"
    exit 1
  fi
  if ! printf '%s\n' "$output" | grep -qF "$store_config"; then
    echo "FAIL: error should contain offending file path $store_config"
    echo "  output: $output"
    cp "$backup" "$store_config"
    rm "$backup"
    exit 1
  fi
  if ! printf '%s\n' "$output" | grep -qF 'hams.config.yaml'; then
    echo "FAIL: error should point to global hams.config.yaml"
    echo "  output: $output"
    cp "$backup" "$store_config"
    rm "$backup"
    exit 1
  fi
  echo "  ok: store-level profile_tag rejected with actionable error"

  # Restore the fixture so downstream tests see the original file.
  cp "$backup" "$store_config"
  rm "$backup"

  echo ""
  echo "--- config scope rejection passed ---"
}
