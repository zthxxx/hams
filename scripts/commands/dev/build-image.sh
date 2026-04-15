#!/usr/bin/env bash
#
# build-image.sh — docker build -t hams-dev-<example> for a given scenario.
#
# Usage:
#   build-image.sh --example <name>
#
# Image tags are stable per example (no version/hash suffix). Docker's
# native layer cache handles incremental rebuilds; rebuild on unchanged
# sources is a ~100ms no-op.
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./_lib.sh
source "${script_dir}/_lib.sh"

example=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --example)
      example="$2"
      shift 2
      ;;
    *)
      printf 'build-image: unknown flag %q\n' "$1" >&2
      exit 2
      ;;
  esac
done

validate_example_name build-image "${example}"

context_dir="examples/${example}"
dockerfile="${context_dir}/Dockerfile"
image_tag="hams-dev-${example}"

if [[ ! -d "${context_dir}" ]]; then
  printf 'build-image: %s does not exist\n' "${context_dir}" >&2
  exit 1
fi
if [[ ! -f "${dockerfile}" ]]; then
  printf 'build-image: %s does not exist\n' "${dockerfile}" >&2
  exit 1
fi

docker build \
  --tag "${image_tag}" \
  --file "${dockerfile}" \
  "${context_dir}"
