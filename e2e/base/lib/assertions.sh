#!/usr/bin/env bash
# Shared E2E assertion helpers.
# Source this file from per-distro test scripts.

# assert_log_contains asserts that after `hams apply` / `hams refresh` /
# any provider invocation, the rolling slog log file under
# `${HAMS_DATA_HOME}/<YYYY-MM>/hams.YYYYMM.log` contains the expected
# substring. Closes the CLAUDE.md task: "Whether logging is emitted —
# for each provider as well as for hams itself — must be verified in
# integration tests."
#
# Usage: assert_log_contains "<description>" "<expected-substring>"
# Requires HAMS_DATA_HOME to point at the integration test sandbox so
# the helper does not pick up unrelated logs from the host's real
# `~/.local/share/hams/`.
assert_log_contains() {
  local desc="$1"
  local expected="$2"
  local data_home="${HAMS_DATA_HOME:-$HOME/.local/share/hams}"
  echo "Testing log: $desc"
  local log_file
  log_file=$(find "$data_home" -type f -name 'hams.*.log' 2>/dev/null | sort | tail -1)
  if [ -z "$log_file" ] || [ ! -s "$log_file" ]; then
    echo "FAIL: $desc"
    echo "  no rolling log file found under $data_home"
    exit 1
  fi
  if ! grep -qF "$expected" "$log_file"; then
    echo "FAIL: $desc"
    echo "  expected log to contain: $expected"
    echo "  log file: $log_file"
    echo "  recent log lines:"
    tail -20 "$log_file" | sed 's/^/    /'
    exit 1
  fi
  echo "  ok: log contains '$expected'"
  echo ""
}

# assert_log_records_session asserts that the rolling log file records
# at least one "hams session started" entry — the per-invocation slog
# bootstrap line emitted by SetupLogging in apply / refresh paths.
# Catches regressions where the log file is created but no records
# actually flow through (wrong slog handler, output redirected, etc.).
assert_log_records_session() {
  local desc="$1"
  assert_log_contains "$desc — session started" "hams session started"
}

assert_success() {
  local desc="$1"
  shift
  echo "Testing: $desc"
  if ! "$@"; then
    echo "FAIL: $desc"
    exit 1
  fi
  echo ""
}

assert_output_contains() {
  local desc="$1"
  local expected="$2"
  shift 2
  echo "Testing: $desc"
  local output
  output=$("$@" 2>&1)
  if ! printf '%s\n' "$output" | grep -qF "$expected"; then
    echo "FAIL: $desc"
    echo "  expected output to contain: $expected"
    echo "  actual output: $output"
    exit 1
  fi
  echo "  ok: output contains '$expected'"
  echo ""
}

# assert_stderr_contains runs a command, captures ONLY stderr, and asserts
# that it contains the expected substring. Use this for verifying slog
# output — hams emits structured logs to stderr by default, so
# assert_output_contains (which bundles stdout+stderr) works but is noisier.
# assert_stderr_contains keeps the intent explicit: "this log line MUST fire".
#
# Usage:
#   assert_stderr_contains "desc" "expected-substring" cmd arg1 arg2 ...
assert_stderr_contains() {
  local desc="$1"
  local expected="$2"
  shift 2
  echo "Testing: $desc"
  local stderr_out
  # Redirect stdout to /dev/null so the captured output is stderr-only.
  stderr_out=$("$@" 2>&1 1>/dev/null)
  if ! printf '%s\n' "$stderr_out" | grep -qF "$expected"; then
    echo "FAIL: $desc"
    echo "  expected stderr to contain: $expected"
    echo "  actual stderr: $stderr_out"
    exit 1
  fi
  echo "  ok: stderr contains '$expected'"
  echo ""
}

# assert_log_line asserts that a hams command emits a slog line matching the
# given substring. Thin wrapper around assert_stderr_contains with a label
# that makes "which provider is logging what" greppable from test output.
#
# Usage:
#   assert_log_line "<provider>" "<substring>" cmd arg1 arg2 ...
assert_log_line() {
  local provider="$1"
  local expected="$2"
  shift 2
  assert_stderr_contains "${provider} log emits '${expected}'" "$expected" "$@"
}

# assert_hams_apply_session_logged runs a trivial `hams apply` (a plain
# --only=<provider>) and asserts BOTH the rolling slog file under
# ${HAMS_DATA_HOME}/**/hams.*.log AND stderr contain the framework
# "hams session started" line. Complements the per-provider assertions
# by verifying the framework itself — not just an individual provider —
# emits session logs through both sinks. Not wired into any provider
# integration.sh; a future framework-level integration test will call
# it directly.
#
# Usage:
#   assert_hams_apply_session_logged <provider> [extra hams apply args...]
assert_hams_apply_session_logged() {
  local provider="$1"
  shift
  if [ -z "${HAMS_STORE:-}" ]; then
    echo "FAIL: assert_hams_apply_session_logged requires HAMS_STORE to be set" >&2
    exit 1
  fi
  assert_stderr_contains "framework: hams apply emits session-start log on stderr" \
    "hams session started" \
    hams --store="$HAMS_STORE" apply --only="$provider" "$@"
  assert_log_contains "framework: hams apply writes session-start to rolling log" \
    "hams session started"
}

# run_smoke_tests runs CLI framework tests common to all distros.
run_smoke_tests() {
  echo "--- CLI smoke tests ---"
  assert_output_contains "hams --version" "hams version" hams --version
  assert_output_contains "hams --help renders" "Declarative IaC environment management" hams --help
  assert_output_contains "hams brew --help routes to provider" "Manage Homebrew packages" hams brew --help

  # Seed a machine-scoped global config so `config list` has something to
  # report. Store-level configs cannot contain profile_tag / machine_id —
  # hams rejects them at load time.
  local config_home="${HAMS_CONFIG_HOME:-$HOME/.config/hams}"
  mkdir -p "$config_home"
  cat > "$config_home/hams.config.yaml" << 'YAML'
profile_tag: test
machine_id: smoke-test
YAML

  # Config loading from the fixture store merges with the seeded global.
  assert_output_contains "config list reads merged config" "Profile tag:       test" \
    hams --store=/fixtures/test-store config list
  assert_output_contains "config get profile_tag" "test" \
    hams --store=/fixtures/test-store config get profile_tag
  echo ""
}

# create_store_repo creates a local git repo from fixture files, and writes
# the global hams config with machine-scoped fields (profile_tag, machine_id).
#
# Usage: create_store_repo <store_dir> <fixture_src_dir> <machine_id>
#
# Machine-scoped fields (profile_tag, machine_id) MUST live in the global
# config at $HAMS_CONFIG_HOME (or ~/.config/hams/). Store-level configs that
# contain them are rejected by hams at load time.
#
# Runs in a subshell to avoid changing the caller's working directory.
create_store_repo() {
  local store_dir="$1"
  local fixture_src="$2"
  local machine_id="$3"

  # Write machine-scoped fields to the global config.
  local config_home="${HAMS_CONFIG_HOME:-$HOME/.config/hams}"
  mkdir -p "$config_home"
  cat > "$config_home/hams.config.yaml" << YAML
profile_tag: test
machine_id: ${machine_id}
YAML

  # Store-level config omits machine-scoped fields — they belong to the
  # machine, not the shared store.
  mkdir -p "$store_dir/test"
  (
    cd "$store_dir" || exit 1
    git init --quiet

    cat > hams.config.yaml << 'YAML'
# Store-level config. Machine-scoped fields live in $HAMS_CONFIG_HOME.
YAML

    cp "$fixture_src"/test/*.hams.yaml test/

    git add -A
    git config user.email "test@hams.dev"
    git config user.name "hams-test"
    git commit -m "e2e fixture" --quiet
  )
}

# verify_bash_marker checks that the bash provider created the marker file.
verify_bash_marker() {
  assert_success "bash marker file exists" test -f /tmp/hams-e2e-marker
}

# verify_git_config checks that git config values were applied.
verify_git_config() {
  assert_output_contains "git config e2e.hams.test was applied" "true" \
    git config --global --get e2e.hams.test
}

# verify_config_roundtrip tests config set/get persistence, then restores
# the original machine_id so subsequent tests that scope state files by
# machine_id keep writing to / asserting against the same .state/<id>/ dir.
verify_config_roundtrip() {
  local original_machine_id
  original_machine_id=$(hams config get machine_id)

  assert_success "hams config set machine_id" \
    hams config set machine_id "e2e-verified"
  assert_output_contains "hams config get machine_id reads back" "e2e-verified" \
    hams config get machine_id

  # Restore the original machine_id so the apt imperative tests (and any
  # other state-scoped assertions) read/write under the same .state/<id>/
  # directory the bootstrap apply already populated.
  assert_success "hams config set machine_id (restore)" \
    hams config set machine_id "$original_machine_id"
}

# verify_idempotent_reapply checks that re-applying is idempotent.
verify_idempotent_reapply() {
  local store_dir="$1"
  local only_flag="$2"
  assert_success "hams apply idempotent re-run" \
    hams apply --from-repo="$store_dir" --only="$only_flag"
}
