#!/usr/bin/env bash
#
# scripts/ci/bump-homebrew-tap.sh — bump the hams Formula in zthxxx/homebrew-tap
#
# Triggered by the release workflow (.github/workflows/release.yml) after a
# successful GitHub Release publish. Downloads the release's checksums.txt,
# rewrites the three per-platform `sha256` fields and the top-level `version`
# in Formula/hams.rb, and opens a pull request against zthxxx/homebrew-tap.
#
# Usage:
#   scripts/ci/bump-homebrew-tap.sh <version>
#
# Arguments:
#   <version>   Semantic version with optional leading `v` (e.g. "0.1.2" or "v0.1.2").
#
# Environment:
#   HOMEBREW_TAP_TOKEN   Required. PAT with `repo` scope on zthxxx/homebrew-tap.
#                        Used for both `git push` and `gh pr create`. The CI
#                        runner's default GITHUB_TOKEN cannot reach another
#                        repo, so a separate secret is mandatory.
#   GITHUB_REPOSITORY    Optional. Source repo, defaults to zthxxx/hams; used
#                        only in PR body links.
#
# Exit codes:
#   0   success (PR opened or already in sync)
#   1   runtime error (missing args, fetch failure, bad checksums, etc.)
#
# Design notes — WHY this shape:
#   * Uses curl + sed instead of pulling a formula-bump GitHub Action. The
#     three-platform shape (version + 3× sha256) fits neatly in ~20 lines of
#     sed and avoids the "does Action X support raw-binary URLs?" guessing
#     game. Inline logic is easier to audit and fix than third-party actions.
#   * Opens a PR rather than pushing directly to main — auditability and a
#     natural rollback point if the formula ever regresses. The PR branch
#     name `bump-hams-v<version>` is deterministic so re-runs amend the same
#     PR instead of piling up duplicates.
#   * force-with-lease on push: safe for both the fresh-branch case and the
#     "previous run left a stale ref" case, while refusing to clobber commits
#     an outside actor may have pushed to the bump branch concurrently.
set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "usage: $(basename "$0") <version>" >&2
  exit 1
fi

VERSION="${1#v}"

if [ -z "${VERSION}" ]; then
  echo "error: empty version" >&2
  exit 1
fi

: "${HOMEBREW_TAP_TOKEN:?HOMEBREW_TAP_TOKEN is required (PAT with repo scope on zthxxx/homebrew-tap)}"

SOURCE_REPO="${GITHUB_REPOSITORY:-zthxxx/hams}"
TAP_REPO="zthxxx/homebrew-tap"
BRANCH="bump-hams-v${VERSION}"
RELEASE_BASE="https://github.com/${SOURCE_REPO}/releases/download/v${VERSION}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "==> Fetching checksums from ${RELEASE_BASE}/checksums.txt"
# GitHub's Release asset URLs can lag a few seconds behind the "release
# created" event on rare occasions. Retry a handful of times before giving
# up, but fail loudly if the asset is still absent — that almost always
# means the upstream `release` job didn't upload checksums.txt, not a
# transient CDN blip.
attempts=0
max_attempts=6
until curl -fsSL -o "${work}/checksums.txt" "${RELEASE_BASE}/checksums.txt"; do
  attempts=$((attempts + 1))
  if [ "${attempts}" -ge "${max_attempts}" ]; then
    echo "error: checksums.txt still 404 after ${max_attempts} attempts" >&2
    exit 1
  fi
  echo "    attempt ${attempts} failed; retrying in 5s"
  sleep 5
done

sha_for() {
  # Extract the SHA that appears before the given filename in checksums.txt.
  # Format per line: `<sha256>  <filename>` (two spaces, as emitted by sha256sum).
  local filename="$1"
  awk -v f="$filename" '$2 == f { print $1; found=1 } END { exit !found }' "${work}/checksums.txt"
}

SHA_DARWIN_ARM64="$(sha_for hams-darwin-arm64)" \
  || { echo "error: hams-darwin-arm64 missing from checksums.txt" >&2; exit 1; }
SHA_LINUX_AMD64="$(sha_for hams-linux-amd64)" \
  || { echo "error: hams-linux-amd64 missing from checksums.txt" >&2; exit 1; }
SHA_LINUX_ARM64="$(sha_for hams-linux-arm64)" \
  || { echo "error: hams-linux-arm64 missing from checksums.txt" >&2; exit 1; }

echo "    darwin-arm64: ${SHA_DARWIN_ARM64}"
echo "    linux-amd64:  ${SHA_LINUX_AMD64}"
echo "    linux-arm64:  ${SHA_LINUX_ARM64}"

echo "==> Cloning ${TAP_REPO}"
git clone --depth 1 \
  "https://x-access-token:${HOMEBREW_TAP_TOKEN}@github.com/${TAP_REPO}.git" \
  "${work}/tap"

cd "${work}/tap"

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

git checkout -B "${BRANCH}"

echo "==> Rewriting Formula/hams.rb"
# Top-level `version`.
sed -i \
  -e "s/^  version \"[^\"]*\"/  version \"${VERSION}\"/" \
  Formula/hams.rb

# Per-platform sha256 blocks. Each block contains a `url "...hams-<triple>"`
# line immediately followed by a `sha256 "..."` line. We anchor the sed
# replacement on the url line and use the `n` command to advance to the next
# line before substituting. This is robust against block reordering and
# whitespace variation because every sha256 update is keyed on its own URL.
sed -i \
  -e "/hams-darwin-arm64/{n;s|sha256 \"[^\"]*\"|sha256 \"${SHA_DARWIN_ARM64}\"|;}" \
  -e "/hams-linux-amd64/{n;s|sha256 \"[^\"]*\"|sha256 \"${SHA_LINUX_AMD64}\"|;}" \
  -e "/hams-linux-arm64/{n;s|sha256 \"[^\"]*\"|sha256 \"${SHA_LINUX_ARM64}\"|;}" \
  Formula/hams.rb

if git diff --quiet Formula/hams.rb; then
  echo "==> Formula already at v${VERSION} with matching SHAs; nothing to bump"
  exit 0
fi

echo "==> Diff:"
git --no-pager diff Formula/hams.rb

git add Formula/hams.rb
git commit -m "hams ${VERSION}

Automated bump produced by ${SOURCE_REPO}'s release workflow.

Release notes: https://github.com/${SOURCE_REPO}/releases/tag/v${VERSION}
Checksums:     ${RELEASE_BASE}/checksums.txt"

echo "==> Pushing ${BRANCH}"
git push --force-with-lease origin "${BRANCH}"

echo "==> Opening / updating PR"
export GH_TOKEN="${HOMEBREW_TAP_TOKEN}"
PR_TITLE="hams ${VERSION}"
PR_BODY="Automated bump from [${SOURCE_REPO} release v${VERSION}](https://github.com/${SOURCE_REPO}/releases/tag/v${VERSION}).

- **version** → \`${VERSION}\`
- **darwin-arm64** sha256 → \`${SHA_DARWIN_ARM64}\`
- **linux-amd64** sha256 → \`${SHA_LINUX_AMD64}\`
- **linux-arm64** sha256 → \`${SHA_LINUX_ARM64}\`

Source: [checksums.txt](${RELEASE_BASE}/checksums.txt)

Merge to ship \`brew upgrade zthxxx/tap/hams\` to users."

if gh pr view "${BRANCH}" --repo "${TAP_REPO}" >/dev/null 2>&1; then
  echo "    existing PR found — updating title/body in case release notes changed"
  gh pr edit "${BRANCH}" --repo "${TAP_REPO}" \
    --title "${PR_TITLE}" \
    --body "${PR_BODY}"
else
  gh pr create --repo "${TAP_REPO}" \
    --head "${BRANCH}" \
    --base main \
    --title "${PR_TITLE}" \
    --body "${PR_BODY}"
fi

echo "==> Done"
