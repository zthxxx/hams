#!/usr/bin/env bash
#
# start-container.sh — launch the dev sandbox container for a given example.
#
# Usage:
#   start-container.sh --example <name> --arch <amd64|arm64>
#
# Starts a throwaway container `hams-<example>`, bind-mounting:
#   host ./bin/                          → /hams-bin/ (read-only)
#   host examples/<name>/config/         → $HOME/.config/hams/
#   host examples/<name>/store/          → /workspace/store/
#   host examples/<name>/state/          → /workspace/store/.state/
#
# Container runs as the host user's uid/gid so files created inside are
# cleanable without sudo. After docker run, the script creates a symlink
# /usr/local/bin/hams → /hams-bin/hams-linux-<arch> via docker exec, so
# the image itself stays arch-agnostic.
#
# Any previous container with the same name (for example, left over from
# a SIGKILL) is force-stopped before the new one starts.
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./_lib.sh
source "${script_dir}/_lib.sh"

example=""
arch=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --example)
      example="$2"
      shift 2
      ;;
    --arch)
      arch="$2"
      shift 2
      ;;
    *)
      printf 'start-container: unknown flag %q\n' "$1" >&2
      exit 2
      ;;
  esac
done

validate_example_name start-container "${example}"

if [[ -z "${arch}" ]]; then
  echo "start-container: --arch <amd64|arm64> is required" >&2
  exit 2
fi

case "${arch}" in
  amd64 | arm64) ;;
  *)
    printf 'start-container: unsupported arch %q (want amd64 or arm64)\n' "${arch}" >&2
    exit 2
    ;;
esac

context_dir="examples/${example}"
for d in config store state; do
  if [[ ! -d "${context_dir}/${d}" ]]; then
    printf 'start-container: %s/%s is missing (run ensure-example first)\n' "${context_dir}" "${d}" >&2
    exit 1
  fi
done

container_name="hams-${example}"
image_tag="hams-dev-${example}"
host_uid="$(id -u)"
host_gid="$(id -g)"
repo_root="$(pwd)"

# Remove any prior container with this name (self-healing after SIGKILL,
# panic, or docker daemon restart). `|| true` because 'No such container'
# is a normal state.
docker rm --force "${container_name}" >/dev/null 2>&1 || true

# The baked container user is "dev" (uid 1000) but we run as the host user
# so bind-mounted files are owned by the developer, not root or dev@1000.
# HOME is set explicitly so hams config resolution lands on the mounted
# config path regardless of /etc/passwd contents.
docker run \
  --detach \
  --name "${container_name}" \
  --rm \
  --user "${host_uid}:${host_gid}" \
  --env "HOME=/home/dev" \
  --env "HAMS_CONFIG_HOME=/home/dev/.config/hams" \
  --volume "${repo_root}/bin:/hams-bin:ro" \
  --volume "${repo_root}/${context_dir}/config:/home/dev/.config/hams" \
  --volume "${repo_root}/${context_dir}/store:/workspace/store" \
  --volume "${repo_root}/${context_dir}/state:/workspace/store/.state" \
  "${image_tag}" \
  sleep infinity >/dev/null

# Post-start setup runs all container mutations in a single `docker exec`
# to amortize the per-exec syscall round-trip. Two responsibilities:
#
#   1. Symlink /usr/local/bin/hams → /hams-bin/hams-linux-<arch>. The
#      host already knows GOARCH, so the image stays arch-agnostic; the
#      symlink points through the bind-mounted /hams-bin/ directory, so
#      rebuilds by the watcher become visible on the very next invocation.
#
#   2. Register the host uid/gid in /etc/passwd and /etc/group (and
#      /etc/shadow if present). The container runs as --user ${host_uid}
#      so bind-mounted files land with correct host ownership, but sudo,
#      su, and PAM require a resolvable passwd + shadow entry. Direct
#      append is used rather than useradd/groupadd for portability across
#      debian, alpine, and other slim bases.
#
# Collision handling: `getent` short-circuits when the uid (e.g., 1000 =
# baked "dev") or gid is already known, so nothing is written twice.
# Interpolation into bash -c is safe because uid/gid come from `id -u`/
# `id -g` on the host (integers only); arch is validated above.
docker exec --user root "${container_name}" bash -eu -c "
ln -sf '/hams-bin/hams-linux-${arch}' /usr/local/bin/hams
if ! getent group '${host_gid}' >/dev/null 2>&1; then
  printf 'hostgroup:x:%s:\n' '${host_gid}' >> /etc/group
fi
if ! getent passwd '${host_uid}' >/dev/null 2>&1; then
  printf 'hostuser:x:%s:%s:dev sandbox host user:/home/dev:/bin/bash\n' \
    '${host_uid}' '${host_gid}' >> /etc/passwd
  # shadow entry with '*' password (disabled login). PAM requires it to
  # skip the 'account locked' check even under NOPASSWD.
  if [[ -f /etc/shadow ]]; then
    printf 'hostuser:*:1::::::\n' >> /etc/shadow
  fi
fi
"

printf 'start-container: %s is running (image %s, arch %s)\n' "${container_name}" "${image_tag}" "${arch}"
