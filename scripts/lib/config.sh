#!/bin/bash
# scripts/lib/config.sh
# Reader for factory.yaml — the template's runtime configuration (Decision 2).
#
# factory.yaml is a deliberately constrained format so this parser stays tiny:
#   - flat `key: value` pairs, one per line, no nesting
#   - lists are space-separated values on one line
#   - values may be double-quoted; a trailing ` # comment` is stripped
# Anything more expressive belongs in a hook, not in configuration.
#
# Usage (from a hook, after sourcing this file):
#   value="$(factory_config_get test_file_patterns)"
#   value="$(factory_config_get check_command 'make check')"   # with default
#
# FACTORY_CONFIG overrides the config path (used by the break/fix self-tests).

factory_config_file() {
  if [ -n "${FACTORY_CONFIG:-}" ]; then
    printf '%s' "$FACTORY_CONFIG"
    return
  fi
  local root
  root="$(git rev-parse --show-toplevel 2>/dev/null || echo .)"
  printf '%s/factory.yaml' "$root"
}

factory_config_get() {
  local key="$1"
  local default="${2:-}"
  local file value
  file="$(factory_config_file)"
  if [ ! -f "$file" ]; then
    printf '%s' "$default"
    return
  fi
  value="$(sed -n "s/^${key}:[[:space:]]*//p" "$file" | head -n 1)"
  # Strip a trailing comment, then surrounding double quotes, then whitespace.
  value="$(printf '%s' "$value" | sed 's/[[:space:]]#.*$//; s/^"\(.*\)"$/\1/; s/[[:space:]]*$//')"
  if [ -z "$value" ]; then
    printf '%s' "$default"
    return
  fi
  printf '%s' "$value"
}
