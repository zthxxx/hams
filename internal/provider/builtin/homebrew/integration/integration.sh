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

# Run the canonical flow as the brew user. We bypass shell init files
# entirely: `env -i` starts from an empty environment and sets exactly
# what hams needs (PATH with linuxbrew bin, HOME/USER/LOGNAME for brew's
# Ruby subprocess hygiene, and the three HAMS_* vars). This avoids the
# non-interactive-login shell gotcha where Debian's stock .bashrc returns
# early before sourcing `brew shellenv`.
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

# State-write semantics: `hams brew install/remove` mutates only the
# Homebrew.hams.yaml; state reconciliation happens on `hams apply`. This
# matches every non-apt provider (see e2e/base/lib/provider_flow.sh), so
# each mutating step is followed by `apply --only=brew` before state-file
# assertions. Apt is the only provider whose CLI writes state directly.

# Step 1: seed install of `jq` (brew formula, tiny).
assert_success "hams brew install jq (seed)" \
  BREW_RUN brew install jq
assert_success "apply reconciles state after seed" \
  BREW_RUN apply --only=brew
assert_yaml_field_eq "Homebrew.state.yaml jq.state=ok after seed" \
  "$STATE_FILE" '.resources.jq.state' 'ok'
FIRST_INSTALL=$(yq -r '.resources.jq.first_install_at' "$STATE_FILE")

sleep 1

# Step 2: re-install bumps updated_at, leaves first_install_at immutable.
assert_success "hams brew install jq (re-install)" \
  BREW_RUN brew install jq
assert_success "apply reconciles state after re-install" \
  BREW_RUN apply --only=brew
assert_yaml_field_eq "jq.first_install_at unchanged" \
  "$STATE_FILE" '.resources.jq.first_install_at' "$FIRST_INSTALL"
assert_yaml_field_lex_gt "jq.updated_at > first_install_at" \
  "$STATE_FILE" '.resources.jq.updated_at' '.resources.jq.first_install_at'

# Step 3: install a new pkg (htop), assert state row created.
assert_success "hams brew install htop (new)" \
  BREW_RUN brew install htop
assert_success "apply reconciles state after new install" \
  BREW_RUN apply --only=brew
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

# Step 5: delete htop from the hamsfile + apply → state=removed.
#
# Using `hams brew remove htop` imperatively AND then `apply` would
# double-execute: the CLI uninstalls htop, but state still shows it as
# ok; the subsequent `apply` re-plans a remove action and brew errors
# out with "No such keg: htop". hamsfile-delete + apply is the
# canonical single-execute path for removal (see
# e2e/base/lib/provider_flow.sh step 5 note). `hams brew remove` is
# exercised in handleRemove unit tests; here we validate the apply-
# driven declarative remove path end-to-end.
HAMSFILE="$HAMS_STORE/test/Homebrew.hams.yaml"
chown brew:brew "$HAMSFILE"
sudo -u brew yq -i 'del(.cli[] | select(.app == "htop"))' "$HAMSFILE"
assert_success "apply after hamsfile-delete transitions htop to removed" \
  BREW_RUN apply --only=brew
assert_yaml_field_eq "htop.state=removed" \
  "$STATE_FILE" '.resources.htop.state' 'removed'

echo ""
echo "=== homebrew integration test passed ==="
