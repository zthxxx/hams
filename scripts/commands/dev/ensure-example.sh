#!/usr/bin/env bash
#
# ensure-example.sh — copy the .template skeleton into examples/<name>/
# on first use, so a developer can pick any scenario name and start coding
# without hand-assembling the directory layout.
#
# Usage:
#   ensure-example.sh --example <name> [--templates-root <dir>]
#
# On second+ runs, this is a no-op — existing example directories are
# never overwritten. The caller is responsible for argument validation at
# the task level; we re-validate here for defense in depth.
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./_lib.sh
source "${script_dir}/_lib.sh"

templates_root="examples/.template"
example=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --example)
      example="$2"
      shift 2
      ;;
    --templates-root)
      templates_root="$2"
      shift 2
      ;;
    *)
      printf 'ensure-example: unknown flag %q\n' "$1" >&2
      exit 2
      ;;
  esac
done

validate_example_name ensure-example "${example}"

target="examples/${example}"

if [[ -d "${target}" ]]; then
  # Already exists. Do nothing — respect whatever state the developer has
  # written by hand or committed.
  exit 0
fi

if [[ ! -d "${templates_root}" ]]; then
  printf 'ensure-example: templates root %q not found\n' "${templates_root}" >&2
  exit 1
fi

mkdir -p "$(dirname "${target}")"
cp -R "${templates_root}" "${target}"
printf 'ensure-example: created %s from %s\n' "${target}" "${templates_root}"
