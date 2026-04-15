#!/usr/bin/env bash
# YAML field assertions for E2E tests. Requires yq (Mike Farah's Go impl).
#
# Usage:
#   source /e2e/lib/yaml_assert.sh
#   assert_yaml_field_eq <file> <yq-path> <expected>
#   assert_yaml_field_absent <file> <yq-path>
#   assert_yaml_field_present <file> <yq-path>
#   get_yaml_field <file> <yq-path>   # echoes the value (exits non-zero if missing)

# get_yaml_field echoes the value at the yq path, or exits non-zero.
get_yaml_field() {
  local file="$1"
  local path="$2"
  yq -r "$path" "$file"
}

# assert_yaml_field_eq fails if the value at <yq-path> does not equal <expected>.
assert_yaml_field_eq() {
  local desc="$1"
  local file="$2"
  local path="$3"
  local expected="$4"

  if [ ! -f "$file" ]; then
    echo "FAIL: $desc — file not found: $file"
    exit 1
  fi

  local actual
  actual=$(yq -r "$path" "$file")
  if [ "$actual" != "$expected" ]; then
    echo "FAIL: $desc"
    echo "  file:     $file"
    echo "  path:     $path"
    echo "  expected: $expected"
    echo "  actual:   $actual"
    exit 1
  fi
  echo "  ok: $desc ($path = $expected)"
}

# assert_yaml_field_present fails if the value at <yq-path> is null or missing.
assert_yaml_field_present() {
  local desc="$1"
  local file="$2"
  local path="$3"

  if [ ! -f "$file" ]; then
    echo "FAIL: $desc — file not found: $file"
    exit 1
  fi

  local actual
  actual=$(yq -r "$path" "$file")
  if [ -z "$actual" ] || [ "$actual" = "null" ]; then
    echo "FAIL: $desc — value at $path is absent"
    echo "  file: $file"
    exit 1
  fi
  echo "  ok: $desc ($path present: $actual)"
}

# assert_yaml_field_absent fails if the value at <yq-path> is present (non-null).
assert_yaml_field_absent() {
  local desc="$1"
  local file="$2"
  local path="$3"

  if [ ! -f "$file" ]; then
    # File absent counts as field absent.
    echo "  ok: $desc (file not present)"
    return 0
  fi

  local actual
  actual=$(yq -r "$path" "$file")
  if [ -n "$actual" ] && [ "$actual" != "null" ]; then
    echo "FAIL: $desc"
    echo "  file: $file"
    echo "  path: $path"
    echo "  unexpected value: $actual"
    exit 1
  fi
  echo "  ok: $desc ($path absent)"
}

# assert_yaml_field_lex_gt fails if <field-a> <= <field-b> (lex compare).
# Useful for timestamps in the 20060102T150405 format (lex-sortable).
assert_yaml_field_lex_gt() {
  local desc="$1"
  local file="$2"
  local path_a="$3"
  local path_b="$4"

  local a b
  a=$(yq -r "$path_a" "$file")
  b=$(yq -r "$path_b" "$file")
  if [ "$a" \> "$b" ]; then
    echo "  ok: $desc ($path_a=$a > $path_b=$b)"
    return 0
  fi
  echo "FAIL: $desc"
  echo "  file:   $file"
  echo "  $path_a = $a"
  echo "  $path_b = $b"
  exit 1
}
