# shellcheck shell=bash
#
# _lib.sh — shared helpers sourced by scripts/commands/dev/*.sh.
#
# Kept intentionally small: this is not a general bash library, just a
# place to land functions that appear in more than one dev-sandbox
# script so reviewers see one canonical implementation.
#
# Source with: `source "${script_dir}/_lib.sh"`.
#
# shellcheck disable=SC2034 # callers re-read these after sourcing.

# validate_example_name rejects any example name that isn't a safe
# single-path-segment identifier. Returns 0 when the name is safe,
# writes an error to stderr and returns 2 otherwise. The allowlist
# matches what `ensure-example.sh` originally enforced; calling it
# from every entry point closes a defense-in-depth gap flagged in
# code review (an example name like `../..` would otherwise let
# build-image.sh or start-container.sh resolve paths outside examples/).
validate_example_name() {
  local caller="${1:?caller label required}"
  local name="${2-}"
  if [[ -z "${name}" ]]; then
    printf '%s: example name is required\n' "${caller}" >&2
    return 2
  fi
  case "${name}" in
    *[!a-zA-Z0-9._-]*)
      printf '%s: example name %q must contain only [a-zA-Z0-9._-]\n' "${caller}" "${name}" >&2
      return 2
      ;;
    "" | "." | ".." | /* | */*)
      printf '%s: example name %q is not a safe single-segment name\n' "${caller}" "${name}" >&2
      return 2
      ;;
  esac
  return 0
}
