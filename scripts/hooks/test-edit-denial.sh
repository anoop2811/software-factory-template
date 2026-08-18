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
if [ -f "$SCRIPT_DIR/../lib/events.sh" ]; then . "$SCRIPT_DIR/../lib/events.sh"; else factory_log_event() { :; }; fi

# Input resolution, in order:
#   1. argv (opencode plugin, evals, selftests) — never touches stdin, so an
#      interactive terminal can invoke this directly without hanging on cat.
#   2. JSON on stdin (Claude/Codex PreToolUse pipe) — read only when stdin is
#      a pipe, never when it is a TTY.
INPUT=""
if [ -n "${1:-}" ]; then
  FILE_PATH="$1"
elif [ ! -t 0 ]; then
  INPUT=$(cat 2>/dev/null || echo "")
  FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.path // .tool_input.command // empty' 2>/dev/null || echo "")
else
  FILE_PATH=""
fi

AGENT_ROLE="${FACTORY_AGENT_ROLE:-}"

# A target we could not identify is not the same as no target. When the role is
# explicitly `implementer` and a payload arrived that produced no path, this gate
# cannot tell whether the edit was allowed — so it denies rather than allowing.
#
# It used to `exit 0` here, which meant a missing `jq`, or any payload shape the
# filter did not match, silently permitted the very edit this gate exists to
# block. The whole point of a computational control is that it does not depend on
# an optional tool being present.
#
# Absent input is still allowed: a TTY invocation or an event carrying no file at
# all has nothing to deny, and failing those would break unrelated tool calls.
# Read the pattern list before the fail-closed branch below. An adopter who has
# configured no test_file_patterns has told this gate that no path is a test
# file, so there is nothing it could deny — denying an unreadable payload in that
# configuration would block edits the contract says are always allowed.
PATTERNS="$(factory_config_get test_file_patterns)"

if [ -z "$FILE_PATH" ]; then
  if [ "$AGENT_ROLE" = "implementer" ] && [ -n "$INPUT" ] && [ -n "$PATTERNS" ]; then
    if ! command -v jq >/dev/null 2>&1; then
      echo "DENIED: implementer role, and jq is not installed, so the edit target cannot be read." >&2
      echo "  Install jq — this gate must not pass work it was unable to check." >&2
      factory_log_event "test-edit-denial" "denied: jq missing, target unreadable"
    else
      echo "DENIED: implementer role, and no edit target could be read from the hook payload." >&2
      echo "  Failing closed: the gate cannot confirm this is not a test file." >&2
      factory_log_event "test-edit-denial" "denied: unparseable payload"
    fi
    exit 2
  fi
  exit 0
fi

if [ "$AGENT_ROLE" != "implementer" ]; then
  exit 0
fi

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
