#!/usr/bin/env bash
#
# main.sh — dev-sandbox orchestrator.
#
# Usage:
#   main.sh --example <name>
#
# Pipeline:
#   1. validate args + tooling (docker, go)
#   2. ensure examples/<name>/ exists (seed from .template/ if missing)
#   3. detect host arch                → GOARCH
#   4. initial `go build` for linux/$GOARCH so the container has a binary
#      the moment it comes up
#   5. docker build -t hams-dev-<name>
#   6. docker run -d --name hams-<name> with bind mounts; post-start
#      symlink /usr/local/bin/hams → /hams-bin/hams-linux-$GOARCH
#   7. print the attach hint
#   8. hand off to `go run ./internal/devtools/watch` for hot-reload
#
# On Ctrl+C, the trap stops the container (auto-removed via --rm).
set -Eeuo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: task dev EXAMPLE=<name>

Starts a throwaway Docker sandbox for the named example with a
Go hot-reload watcher. Creates examples/<name>/ from the baseline
template on first use.

Existing examples:
EOF
  if [[ -d examples ]]; then
    find examples -mindepth 1 -maxdepth 1 -type d -not -name '.*' \
      | sort \
      | sed 's#examples/#  - #' >&2 || true
  fi
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'task dev: required command %q is not installed or not in PATH\n' "$1" >&2
    exit 1
  }
}

example=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --example)
      example="$2"
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      printf 'task dev: unknown flag %q\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "${example}" ]]; then
  echo "task dev: EXAMPLE is required (usage: task dev EXAMPLE=<name>)" >&2
  usage
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../../.." && pwd)"
cd "${repo_root}"

require_cmd docker
require_cmd go

if ! docker info >/dev/null 2>&1; then
  echo "task dev: docker daemon is not reachable (is Docker Desktop running?)" >&2
  exit 1
fi

bash "${script_dir}/ensure-example.sh" --example "${example}"

arch="$(bash "${script_dir}/detect-arch.sh")"
printf 'task dev: target arch = %s\n' "${arch}"

output="bin/hams-linux-${arch}"
mkdir -p bin
# Inject the short commit SHA so `hams --version` inside the container shows
# `dev (<sha>)` from the very first invocation, matching what the watcher's
# later rebuilds will produce. `git rev-parse` is allowed to fail — the
# watcher will correct the SHA on the next build if the repo resolves later.
initial_commit="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
initial_ldflags="-X github.com/zthxxx/hams/internal/version.commit=${initial_commit}"

# Install the cleanup trap *before* starting any background work. The trap
# covers: (a) the backgrounded `go build` child so it can't orphan if we
# abort during `docker build`; (b) the container, whose name we can fix
# upfront since it's deterministic. Under `set -Eeuo pipefail`, subcommand
# failures abort bash mid-function — without a trap already in place, the
# go-build PID would leak and any already-created container would survive.
container_name="hams-${example}"
go_pid=""
cleanup() {
  local status=$?
  if [[ -n "${go_pid}" ]] && kill -0 "${go_pid}" 2>/dev/null; then
    kill "${go_pid}" 2>/dev/null || true
    wait "${go_pid}" 2>/dev/null || true
  fi
  if docker inspect "${container_name}" >/dev/null 2>&1; then
    printf '\ntask dev: stopping %s\n' "${container_name}"
    docker stop "${container_name}" >/dev/null 2>&1 || true
  fi
  exit "${status}"
}
trap cleanup INT TERM EXIT

printf 'task dev: building %s (initial, commit=%s) in parallel with image\n' \
  "${output}" "${initial_commit}"

# Run the initial `go build` and `docker build` concurrently: they touch
# disjoint inputs (Go sources on one side, examples/<name>/Dockerfile on
# the other) so there's no ordering requirement between them. The container
# only needs the binary once start-container.sh mounts ./bin/.
GOOS=linux GOARCH="${arch}" CGO_ENABLED=0 \
  go build -ldflags "${initial_ldflags}" -o "${output}" ./cmd/hams &
go_pid=$!

# Use `if !` so a non-zero exit isn't swallowed by `set -e` before we get
# a chance to surface a descriptive error.
if ! bash "${script_dir}/build-image.sh" --example "${example}"; then
  printf 'task dev: docker build failed\n' >&2
  exit 1
fi

if ! wait "${go_pid}"; then
  printf 'task dev: initial go build failed\n' >&2
  exit 1
fi
go_pid=""

if ! bash "${script_dir}/start-container.sh" --example "${example}" --arch "${arch}"; then
  printf 'task dev: start-container failed\n' >&2
  exit 1
fi

cat <<EOF

task dev: sandbox ready.

  Attach: docker exec -it ${container_name} bash
     or: task dev:shell EXAMPLE=${example}

  Watching ./cmd ./internal ./pkg for .go changes (500ms debounce).
  Press Ctrl+C to stop.

EOF

# Hand off to the Go watcher. signal.NotifyContext inside the watcher
# plus the trap above keep the container cleanup path symmetrical.
exec go run ./internal/devtools/watch --arch "${arch}"
