#!/usr/bin/env bash
# Integration test for the git package: git-config + git-clone providers.
#
# git-config manages `git config --global <key> <value>` entries declaratively
# in git-config.hams.yaml. git-clone manages `git clone <remote> <path>`
# declaratively in git-clone.hams.yaml. Both update state on apply/remove.
#
# Neither has a PATH binary to check; the "post-install" check is a
# provider-specific side-effect: git-config → `git config --global --get`;
# git-clone → `test -d <path>/.git`.

set -euo pipefail
source /e2e/base/lib/assertions.sh
source /e2e/base/lib/yaml_assert.sh

echo "=== hams integration test: git (config + clone) ==="
echo ""

export HAMS_STORE=/tmp/test-git-store
export HAMS_MACHINE_ID=e2e-git
export HAMS_CONFIG_HOME=/tmp/test-git-config

mkdir -p "$HAMS_STORE/test" "$HAMS_STORE/.state/$HAMS_MACHINE_ID" "$HAMS_CONFIG_HOME"
cat > "$HAMS_CONFIG_HOME/hams.config.yaml" <<YAML
profile_tag: test
machine_id: ${HAMS_MACHINE_ID}
YAML
cat > "$HAMS_STORE/hams.config.yaml" <<'YAML'
YAML

# Seed a minimal git identity so commits work.
git config --global user.email "integration@hams.dev"
git config --global user.name "hams-integration"

# ======================================================================
# git-config
# ======================================================================
echo ""
echo "--- git-config ---"

GCFG_HAMS="$HAMS_STORE/test/git-config.hams.yaml"
GCFG_STATE="$HAMS_STORE/.state/$HAMS_MACHINE_ID/git-config.state.yaml"

cat > "$GCFG_HAMS" <<'YAML'
configs:
  - app: "integration.hams.test=alpha"
  - app: "integration.hams.flag=true"
YAML

assert_success "hams apply --only=git-config" \
  hams --store="$HAMS_STORE" apply --only=git-config
assert_output_contains "git-config wrote alpha" "alpha" \
  git config --global --get integration.hams.test
assert_output_contains "git-config wrote flag" "true" \
  git config --global --get integration.hams.flag
assert_yaml_field_eq "git-config.state has alpha" \
  "$GCFG_STATE" '.resources."integration.hams.test=alpha".state' 'ok'
FIRST_INSTALL=$(yq -r '.resources."integration.hams.test=alpha".first_install_at' "$GCFG_STATE")

# Refresh re-probes, bumps updated_at.
sleep 1
assert_success "hams refresh --only=git-config" \
  hams --store="$HAMS_STORE" refresh --only=git-config
AFTER_REFRESH=$(yq -r '.resources."integration.hams.test=alpha".updated_at' "$GCFG_STATE")
if [ "$AFTER_REFRESH" \> "$FIRST_INSTALL" ]; then
  echo "  ok: refresh bumped git-config updated_at"
else
  echo "FAIL: refresh did not bump git-config updated_at"
  exit 1
fi

# Remove the flag entry, keep alpha → state shows flag=removed, alpha=ok.
cat > "$GCFG_HAMS" <<'YAML'
configs:
  - app: "integration.hams.test=alpha"
YAML
assert_success "hams apply after removing flag entry" \
  hams --store="$HAMS_STORE" apply --only=git-config
assert_yaml_field_eq "flag transitioned to removed" \
  "$GCFG_STATE" '.resources."integration.hams.flag=true".state' 'removed'
assert_yaml_field_eq "alpha still ok" \
  "$GCFG_STATE" '.resources."integration.hams.test=alpha".state' 'ok'

# ======================================================================
# git-clone
# ======================================================================
echo ""
echo "--- git-clone ---"

GCLONE_HAMS="$HAMS_STORE/test/git-clone.hams.yaml"
GCLONE_STATE="$HAMS_STORE/.state/$HAMS_MACHINE_ID/git-clone.state.yaml"
FIXTURE_REMOTE=/tmp/test-git-clone-fixture.git
CLONE_TARGET=/tmp/test-git-clone-target

# Create a minimal bare repo fixture as the "remote".
rm -rf "$FIXTURE_REMOTE" "$CLONE_TARGET"
mkdir -p "$FIXTURE_REMOTE"
git init --bare --quiet "$FIXTURE_REMOTE"
seed=/tmp/git-clone-seed
rm -rf "$seed"
git init --quiet "$seed"
cd "$seed"
echo "hello hams" > README.md
git add README.md
git -c user.email=integration@hams.dev -c user.name=hams-integration commit -m "seed" --quiet
git remote add origin "$FIXTURE_REMOTE"
git push origin HEAD:master --quiet
cd -
rm -rf "$seed"

cat > "$GCLONE_HAMS" <<YAML
repos:
  - urn: "urn:hams:git-clone:integration-fixture"
    app: integration-fixture
    remote: "${FIXTURE_REMOTE}"
    path: "${CLONE_TARGET}"
YAML

assert_success "hams apply --only=git-clone" \
  hams --store="$HAMS_STORE" apply --only=git-clone
assert_success "clone target is a git repo" test -d "$CLONE_TARGET/.git"
assert_yaml_field_eq "git-clone state records the clone" \
  "$GCLONE_STATE" '.resources."urn:hams:git-clone:integration-fixture".state' 'ok'

CLONE_FIRST_INSTALL=$(yq -r '.resources."urn:hams:git-clone:integration-fixture".first_install_at' "$GCLONE_STATE")

# Refresh re-probes clone presence.
sleep 1
assert_success "hams refresh --only=git-clone" \
  hams --store="$HAMS_STORE" refresh --only=git-clone
CLONE_AFTER_REFRESH=$(yq -r '.resources."urn:hams:git-clone:integration-fixture".updated_at' "$GCLONE_STATE")
if [ "$CLONE_AFTER_REFRESH" \> "$CLONE_FIRST_INSTALL" ]; then
  echo "  ok: refresh bumped git-clone updated_at"
else
  echo "FAIL: refresh did not bump git-clone updated_at"
  exit 1
fi

# Remove hamsfile entry + apply → clone dir removed, state=removed.
cat > "$GCLONE_HAMS" <<'YAML'
repos: []
YAML
assert_success "hams apply removes the clone" \
  hams --store="$HAMS_STORE" apply --only=git-clone
assert_yaml_field_eq "git-clone state=removed after hamsfile delete" \
  "$GCLONE_STATE" '.resources."urn:hams:git-clone:integration-fixture".state' 'removed'

echo ""
echo "=== git integration test passed ==="
