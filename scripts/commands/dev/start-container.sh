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

if [[ -z "${example}" || -z "${arch}" ]]; then
  echo "start-container: --example <name> and --arch <amd64|arm64> are required" >&2
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

# Post-start symlink: the host already knows GOARCH, so we don't need
# any arch branching inside the image. The symlink points through the
# bind-mounted /hams-bin/ directory, so rebuilds by the watcher become
# visible on the very next `hams` invocation.
docker exec \
  --user root \
  "${container_name}" \
  ln -sf "/hams-bin/hams-linux-${arch}" "/usr/local/bin/hams"

printf 'start-container: %s is running (image %s, arch %s)\n' "${container_name}" "${image_tag}" "${arch}"
