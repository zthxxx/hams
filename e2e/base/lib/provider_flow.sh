#!/usr/bin/env bash
# Standardized per-provider integration-test flow.
#
# Every in-scope provider's integration.sh calls `standard_cli_flow` (or
# its `post_install_check`-hook variant for providers without a PATH
# binary, like bash / git-config). The function exercises the canonical
# user lifecycle:
#
#   seed existing_pkg      ->  state row created (first_install_at set)
#   re-install existing    ->  updated_at bumps, first_install_at immutable
#   pre-check new_pkg      ->  post-install check must fail before install
#   install new_pkg        ->  post-install check succeeds, state row created
#   refresh                ->  updated_at bumps for tracked resources
#   remove new_pkg         ->  post-install check fails again, state row
#                              transitions to state=removed with removed_at set
#
# All state-file assertions route through assert_yaml_field_* (sourced
# from yaml_assert.sh). The function fails fast on the first bad
# assertion with a descriptive message.
#
# Usage:
#   source /e2e/base/lib/assertions.sh
#   source /e2e/base/lib/yaml_assert.sh
#   source /e2e/base/lib/provider_flow.sh
#   export HAMS_STORE=/tmp/test-hams-store
#   export HAMS_MACHINE_ID=e2e-<provider>
#   standard_cli_flow <provider> <install_verb> <existing_pkg> <new_pkg>
#
# Optional: when the provider has no PATH binary, set
# `POST_INSTALL_CHECK=<fn-name>` before calling. The function receives
# the package name as $1 and MUST exit non-zero when the resource is
# absent and zero when it is installed (mirrors `command -v` semantics).

# default_post_install_check uses `command -v <pkg>`; mirrors the user's
# manual sandbox verification.
default_post_install_check() {
  command -v "$1" >/dev/null 2>&1
}

# standard_cli_flow <provider> <install_verb> <existing_pkg> <new_pkg>
#
# Requires these env vars:
#   HAMS_STORE       — store directory (value for --store and state root).
#   HAMS_MACHINE_ID  — machine id; state file lives at $HAMS_STORE/.state/$HAMS_MACHINE_ID/<state-prefix>.state.yaml.
# Optional env vars:
#   STATE_FILE_PREFIX  — overrides the state-file basename. Required when
#                        the provider's `Manifest().FilePrefix` differs
#                        from `Manifest().Name` (e.g., vscodeext: Name
#                        `code-ext`, FilePrefix `vscodeext`; homebrew:
#                        Name `brew`, FilePrefix `Homebrew`). Defaults
#                        to the provider name passed as $1.
#   POST_INSTALL_CHECK — bash function name used in place of `command -v`.
#                        Set + `export -f <fn>` it for providers that
#                        don't install a PATH binary (bash, git-config,
#                        git-clone, code-ext).
#
# State-write semantics: today only the apt provider writes state directly
# from its CLI install/remove handlers (per the spec delta in
# `fix-apt-cli-state-write-and-htop-rename`). Every other provider still
# follows the original "CLI mutates hamsfile; apply reconciles state"
# pattern. To keep the helper portable across both, every install / remove
# is followed by `hams apply --only=<provider>` — a no-op for apt (state
# already up-to-date) and the load-bearing reconciliation step for the
# other ten providers.
standard_cli_flow() {
  local provider="$1"
  local install_verb="$2"
  local existing_pkg="$3"
  local new_pkg="$4"

  if [ -z "$HAMS_STORE" ]; then
    echo "FAIL: standard_cli_flow requires HAMS_STORE to be set" >&2
    exit 1
  fi
  if [ -z "$HAMS_MACHINE_ID" ]; then
    echo "FAIL: standard_cli_flow requires HAMS_MACHINE_ID to be set" >&2
    exit 1
  fi

  local check_fn="${POST_INSTALL_CHECK:-default_post_install_check}"
  local state_prefix="${STATE_FILE_PREFIX:-$provider}"
  local state_file="$HAMS_STORE/.state/$HAMS_MACHINE_ID/${state_prefix}.state.yaml"

  echo ""
  echo "--- standard_cli_flow ($provider: seed=$existing_pkg, new=$new_pkg) ---"

  # _reconcile runs `hams apply --only=<provider>` so providers that don't
  # write state from their CLI handler (everyone except apt today) still
  # converge their state file. For apt this call is a no-op against state
  # — the install handler already wrote it.
  _reconcile() {
    hams --store="$HAMS_STORE" apply --only="$provider"
  }

  # -------------------------------------------------------------------
  # Step 1: seed install of the "existing" package establishes the
  # baseline state row. Capture first_install_at for step 3's immutability
  # assertion. Idempotent re-installs are safe on already-installed hosts.
  # -------------------------------------------------------------------
  assert_success "seed install: hams $provider $install_verb $existing_pkg" \
    hams --store="$HAMS_STORE" "$provider" "$install_verb" "$existing_pkg"
  assert_success "reconcile after seed install" _reconcile
  assert_yaml_field_eq "$existing_pkg.state=ok after seed install" \
    "$state_file" ".resources.$existing_pkg.state" 'ok'

  local first_install_at
  first_install_at=$(yq -r ".resources.$existing_pkg.first_install_at" "$state_file")
  echo "  captured $existing_pkg.first_install_at=$first_install_at"

  # Sleep 1s so the next timestamp is strictly greater in the compact
  # ISO format (seconds resolution).
  sleep 1

  # -------------------------------------------------------------------
  # Step 2: re-install of the existing package bumps updated_at but
  # leaves first_install_at immutable. This is the canonical "update"
  # case — the exact scenario the user reports as "update normal".
  # -------------------------------------------------------------------
  assert_success "re-install: hams $provider $install_verb $existing_pkg" \
    hams --store="$HAMS_STORE" "$provider" "$install_verb" "$existing_pkg"
  assert_success "reconcile after re-install" _reconcile
  assert_yaml_field_eq "$existing_pkg.first_install_at unchanged after re-install" \
    "$state_file" ".resources.$existing_pkg.first_install_at" "$first_install_at"
  assert_yaml_field_lex_gt "$existing_pkg.updated_at > first_install_at after re-install" \
    "$state_file" ".resources.$existing_pkg.updated_at" ".resources.$existing_pkg.first_install_at"

  # -------------------------------------------------------------------
  # Step 3: install a brand-new package. The check hook must fail
  # BEFORE install and succeed AFTER install.
  # -------------------------------------------------------------------
  if "$check_fn" "$new_pkg"; then
    echo "FAIL: precondition — $new_pkg should not be installed before the install step"
    exit 1
  fi
  assert_success "install new package: hams $provider $install_verb $new_pkg" \
    hams --store="$HAMS_STORE" "$provider" "$install_verb" "$new_pkg"
  assert_success "reconcile after new-package install" _reconcile
  if ! "$check_fn" "$new_pkg"; then
    echo "FAIL: $new_pkg should be installed after hams $provider $install_verb"
    exit 1
  fi
  assert_yaml_field_eq "$new_pkg.state=ok after install" \
    "$state_file" ".resources.$new_pkg.state" 'ok'
  assert_yaml_field_present "$new_pkg.first_install_at set on first install" \
    "$state_file" ".resources.$new_pkg.first_install_at"
  assert_yaml_field_absent "$new_pkg.removed_at absent on first install" \
    "$state_file" ".resources.$new_pkg.removed_at"

  # -------------------------------------------------------------------
  # Step 4: refresh re-probes tracked resources and bumps updated_at.
  # Validates the user's "refresh should update state" expectation.
  # -------------------------------------------------------------------
  local before_refresh
  before_refresh=$(yq -r ".resources.$new_pkg.updated_at" "$state_file")
  sleep 1
  assert_success "refresh: hams refresh --only=$provider" \
    hams --store="$HAMS_STORE" refresh --only="$provider"
  local after_refresh
  after_refresh=$(yq -r ".resources.$new_pkg.updated_at" "$state_file")
  if [ "$after_refresh" \> "$before_refresh" ]; then
    echo "  ok: refresh bumped $new_pkg.updated_at ($before_refresh → $after_refresh)"
  else
    echo "FAIL: refresh did not bump $new_pkg.updated_at"
    echo "  before: $before_refresh"
    echo "  after:  $after_refresh"
    exit 1
  fi

  # -------------------------------------------------------------------
  # Step 5: remove the new package. State transitions to removed and
  # records removed_at; the check hook must fail again.
  # -------------------------------------------------------------------
  assert_success "remove: hams $provider remove $new_pkg" \
    hams --store="$HAMS_STORE" "$provider" remove "$new_pkg"
  assert_success "reconcile after remove" _reconcile
  if "$check_fn" "$new_pkg"; then
    echo "FAIL: $new_pkg should be absent after hams $provider remove"
    exit 1
  fi
  assert_yaml_field_eq "$new_pkg.state=removed" \
    "$state_file" ".resources.$new_pkg.state" 'removed'
  assert_yaml_field_present "$new_pkg.removed_at set after remove" \
    "$state_file" ".resources.$new_pkg.removed_at"

  echo ""
  echo "--- standard_cli_flow ($provider) passed ---"
}
