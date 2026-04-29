#!/usr/bin/env bash
# Integration test for the homebrew provider (linuxbrew path).
#
# Linuxbrew refuses to run as root, so the actual hams brew install/remove
# calls are wrapped in `sudo -u brew -i`. The state file and hamsfile still
# live under /tmp/test-brew-store (owned by brew, accessible to root via
# NOPASSWD sudo).

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh

echo "=== hams integration test: homebrew (linuxbrew) ==="
echo ""

export HAMS_STORE=/tmp/test-brew-store
export HAMS_MACHINE_ID=e2e-brew
export HAMS_CONFIG_HOME=/home/brew/.config/hams
export PATH="/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin:${PATH}"

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID"
chown -R brew:brew "$HAMS_STORE"
sudo -u brew mkdir -p "$HAMS_CONFIG_HOME"
sudo -u brew bash -c "cat > $HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML
chown brew:brew "$HAMS_STORE/hams.config.yaml"

assert_output_contains "hams --version" "hams version" hams --version

# `env -i` bypasses Debian's non-interactive-login `.bashrc` early-return
# guard that would otherwise skip the linuxbrew shellenv. PATH is set
# explicitly so hams can exec `brew`; HOME/USER/LOGNAME satisfy brew's
# Ruby subprocess hygiene checks.
STATE_FILE="$HAMS_STORE/.state/$HAMS_MACHINE_ID/Homebrew.state.yaml"

BREW_RUN() {
  sudo -u brew -H \
    env -i \
        HOME=/home/brew USER=brew LOGNAME=brew \
        PATH=/home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin:/usr/local/bin:/usr/bin:/bin \
        HAMS_STORE="$HAMS_STORE" \
        HAMS_MACHINE_ID="$HAMS_MACHINE_ID" \
        HAMS_CONFIG_HOME="$HAMS_CONFIG_HOME" \
    hams --store="$HAMS_STORE" "$@"
}

# Brew CLI writes only the hamsfile; state is reconciled by `apply` (see
# e2e/base/lib/provider_flow.sh — apt is the only provider whose CLI
# writes state directly). Each `apply --only=brew` here re-probes every
# installed formula via `brew info --json=v2 --installed`, so we batch:
# step-1 reconcile is required to capture jq's first_install_at; step-2
# adds NO new state — its assertions are folded into step-3's reconcile,
# which probes both jq (re-installed) and htop (new) in one pass.

# Step 1: seed install of `jq` (brew formula, tiny). Reconcile to capture
# first_install_at for the immutability assertion in step 3.
assert_success "hams brew install jq (seed)" \
  BREW_RUN brew install jq
assert_success "apply reconciles state after seed" \
  BREW_RUN apply --only=brew
assert_yaml_field_eq "Homebrew.state.yaml jq.state=ok after seed" \
  "$STATE_FILE" '.resources.jq.state' 'ok'
FIRST_INSTALL=$(yq -r '.resources.jq.first_install_at' "$STATE_FILE")

# The install flow populates `intro:` from `brew info --json=v2 jq` so
# the Hamsfile is self-documenting right after install — no LLM pass
# needed. Upstream's `desc` text may evolve over time; assert only that
# the field is non-empty and non-null.
HAMSFILE="$HAMS_STORE/test/Homebrew.hams.yaml"
JQ_INTRO=$(sudo -u brew yq -r '.cli[] | select(.app == "jq") | .intro' "$HAMSFILE")
if [ -z "${JQ_INTRO:-}" ] || [ "$JQ_INTRO" = "null" ]; then
  echo "FAIL: jq entry missing intro field in $HAMSFILE"
  sudo -u brew cat "$HAMSFILE"
  exit 1
fi
echo "  ok: jq.intro populated from brew info: '$JQ_INTRO'"

sleep 1

# Step 2: re-install jq (no state assertion yet — folded into step 3).
assert_success "hams brew install jq (re-install)" \
  BREW_RUN brew install jq

# Step 3: install a new pkg (htop). One `apply --only=brew` reconciles
# both jq's re-install (updated_at bump, first_install_at immutable) and
# htop's new install in a single probe pass.
assert_success "hams brew install htop (new)" \
  BREW_RUN brew install htop
assert_success "apply reconciles state after re-install + new install" \
  BREW_RUN apply --only=brew
assert_yaml_field_eq "jq.first_install_at unchanged after re-install" \
  "$STATE_FILE" '.resources.jq.first_install_at' "$FIRST_INSTALL"
assert_yaml_field_lex_gt "jq.updated_at > first_install_at after re-install" \
  "$STATE_FILE" '.resources.jq.updated_at' '.resources.jq.first_install_at'
assert_yaml_field_eq "htop.state=ok after install" \
  "$STATE_FILE" '.resources.htop.state' 'ok'

# Step 4: refresh bumps updated_at.
BEFORE=$(yq -r '.resources.htop.updated_at' "$STATE_FILE")
sleep 1
assert_success "hams refresh --only=brew" \
  BREW_RUN refresh --only=brew
AFTER=$(yq -r '.resources.htop.updated_at' "$STATE_FILE")
if [ "$AFTER" \> "$BEFORE" ]; then
  echo "  ok: refresh bumped htop.updated_at"
else
  echo "FAIL: refresh did not bump htop.updated_at"
  exit 1
fi

# Step 5: hamsfile-delete + apply → state=removed (see provider_flow.sh
# step 5 — imperative remove + apply double-executes brew uninstall).
# HAMSFILE was captured earlier (step 1) so we reuse it here.
chown brew:brew "$HAMSFILE"
sudo -u brew yq -i 'del(.cli[] | select(.app == "htop"))' "$HAMSFILE"
assert_success "apply after hamsfile-delete transitions htop to removed" \
  BREW_RUN apply --only=brew
assert_yaml_field_eq "htop.state=removed" \
  "$STATE_FILE" '.resources.htop.state' 'removed'

echo ""
echo "=== homebrew integration test passed ==="
