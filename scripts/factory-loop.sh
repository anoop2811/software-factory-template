#!/bin/bash
set -euo pipefail
# docs/LOOPS.md:14 — one controller, with shared parsed configuration.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib/config.sh
. "$SCRIPT_DIR/lib/config.sh"
# shellcheck source=lib/roles.sh
. "$SCRIPT_DIR/lib/roles.sh"
# shellcheck source=lib/budget-config.sh
. "$SCRIPT_DIR/lib/budget-config.sh"
for entry in \
  "enabled false" "max_attempts 2" "timeout_seconds 900" \
  "check_timeout_seconds 120" "no_progress_limit 1"
do
  read -r key default <<< "$entry"
  value="$(factory_config_get "loop_$key" "$default")"
  var="FACTORY_LOOP_$(printf "%s" "$key" | tr "[:lower:]" "[:upper:]")"
  printf -v "$var" "%s" "$value"
  export "${var?}"
done
export FACTORY_LOOP_CHECK_COMMAND FACTORY_LOOP_TEST_PATTERNS FACTORY_LOOP_PROTECTED_PATHS FACTORY_LOOP_CONFIG_PATH
FACTORY_LOOP_CHECK_COMMAND="$(factory_config_get loop_check_command "$(factory_config_get check_command)")"
FACTORY_LOOP_TEST_PATTERNS="$(factory_config_get test_file_patterns)"
FACTORY_LOOP_PROTECTED_PATHS="$(factory_config_get protected_paths)"
FACTORY_LOOP_CONFIG_PATH="$(factory_config_file)"
for harness in codex claude opencode; do
  for role in implementer reviewer; do
    var="FACTORY_LOOP_$(printf "%s_%s_MODEL" "$harness" "$role" | tr "[:lower:]" "[:upper:]")"
    printf -v "$var" "%s" "$(factory_budget_model "$harness" "$role")"
    export "${var?}"
  done
done
exec python3 "$SCRIPT_DIR/lib/loop.py" "$@"
