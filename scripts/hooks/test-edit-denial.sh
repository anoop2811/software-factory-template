#!/bin/bash
set -euo pipefail

# scripts/hooks/test-edit-denial.sh
# Shared enforcement: blocks the implementer role from editing test files.
# Called by the opencode plugin plus Claude Code and Codex PreToolUse hooks.
# Input: JSON on stdin with tool_name and tool_input (Claude/Codex format) or
#        a file path (opencode format — passed as the first argument).
# Exit 0 = allow; Exit 2 = deny.
#
# Test-file patterns come from factory.yaml `test_file_patterns` (Decision 2):
# space-separated extended regular expressions, normally set by the language
# pack (Go example: '_test\.go([^[:alnum:]_]|$)'). If unset, every edit is
# allowed — the gate is armed by configuration, never by guesswork.
#
# The role check is inverted: FACTORY_AGENT_ROLE must be EXPLICITLY set to
# "implementer" to be blocked. If unset or any other value, the edit is
# allowed. This prevents blocking the spec-writer (whose job is writing
# tests) when the role is not set.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/config.sh
. "$SCRIPT_DIR/../lib/config.sh"
# shellcheck source=../lib/events.sh
. "$SCRIPT_DIR/../lib/events.sh"

# Input resolution, in order:
#   1. argv (opencode plugin, evals, selftests) — never touches stdin, so an
#      interactive terminal can invoke this directly without hanging on cat.
#   2. JSON on stdin (Claude/Codex PreToolUse pipe) — read only when stdin is
#      a pipe, never when it is a TTY.
if [ -n "${1:-}" ]; then
  FILE_PATH="$1"
elif [ ! -t 0 ]; then
  INPUT=$(cat 2>/dev/null || echo "")
  FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.path // .tool_input.command // empty' 2>/dev/null || echo "")
else
  FILE_PATH=""
fi

if [ -z "$FILE_PATH" ]; then
  exit 0
fi

AGENT_ROLE="${FACTORY_AGENT_ROLE:-}"
if [ "$AGENT_ROLE" != "implementer" ]; then
  exit 0
fi

PATTERNS="$(factory_config_get test_file_patterns)"
if [ -z "$PATTERNS" ]; then
  exit 0
fi

for PATTERN in $PATTERNS; do
  if echo "$FILE_PATH" | grep -qE "$PATTERN"; then
    echo "DENIED: implementer role cannot edit test files (pattern: $PATTERN). Generator/evaluator separation." >&2
    factory_log_event "test-edit-denial" "implementer edited a test file"
    exit 2
  fi
done

exit 0
