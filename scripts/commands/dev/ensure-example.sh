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

if [[ -z "${example}" ]]; then
  echo "ensure-example: --example <name> is required" >&2
  exit 2
fi

case "${example}" in
  *[!a-zA-Z0-9._-]*)
    printf 'ensure-example: example name %q must contain only [a-zA-Z0-9._-]\n' "${example}" >&2
    exit 2
    ;;
  "" | "." | ".." | /* | */*)
    printf 'ensure-example: example name %q is not a safe single-segment name\n' "${example}" >&2
    exit 2
    ;;
esac

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
