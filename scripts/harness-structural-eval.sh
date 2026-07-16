#!/bin/bash
set -euo pipefail

# Deterministic adapter wiring check. This does not claim live harness parity;
# it proves that each generated adapter delegates to the shared deny script and
# that the script handles that harness's documented payload shape.

HARNESS=""
for arg in "$@"; do
  case "$arg" in
    --harness=*) HARNESS="${arg#*=}" ;;
  esac
done

case "$HARNESS" in
  opencode|claude|codex) ;;
  *)
    echo "usage: $0 --harness=opencode|claude|codex" >&2
    exit 2
    ;;
esac

SCRIPT="scripts/hooks/test-edit-denial.sh"
PROBE_FILE="pkg/probe_test.go"

# Self-contained config: this eval proves adapter wiring, not the host
# repository's settings — arm the deny pattern regardless of factory.yaml.
EVAL_CFG="$(mktemp)"
trap 'rm -f "$EVAL_CFG"' EXIT
printf 'test_file_patterns: "_test\\.go([^[:alnum:]_]|$)"\n' > "$EVAL_CFG"
export FACTORY_CONFIG="$EVAL_CFG"

expect_deny() {
  local status
  set +e
  "$@" >/dev/null 2>&1
  status=$?
  set -e
  if [ "$status" -ne 2 ]; then
    echo "STRUCTURAL EVAL FAIL: $HARNESS implementer edit returned $status, expected 2" >&2
    exit 1
  fi
}

expect_allow() {
  local status
  set +e
  "$@" >/dev/null 2>&1
  status=$?
  set -e
  if [ "$status" -ne 0 ]; then
    echo "STRUCTURAL EVAL FAIL: $HARNESS non-implementer edit returned $status, expected 0" >&2
    exit 1
  fi
}

case "$HARNESS" in
  opencode)
    grep -q 'test-edit-denial.sh' .opencode/plugin/factory-hooks.ts
    grep -q 'agentNameToRole' .opencode/plugin/factory-hooks.ts
    expect_deny env FACTORY_AGENT_ROLE=implementer "$SCRIPT" "$PROBE_FILE"
    expect_allow env FACTORY_AGENT_ROLE=spec-writer "$SCRIPT" "$PROBE_FILE"
    ;;
  claude)
    grep -q 'FACTORY_AGENT_ROLE=implementer' .claude/agents/implementer.md
    grep -q 'test-edit-denial.sh' .claude/agents/implementer.md
    PAYLOAD='{"tool_name":"Write","tool_input":{"file_path":"'"$PROBE_FILE"'"}}'
    expect_deny sh -c "printf '%s' '$PAYLOAD' | FACTORY_AGENT_ROLE=implementer '$SCRIPT'"
    expect_allow sh -c "printf '%s' '$PAYLOAD' | FACTORY_AGENT_ROLE=spec-writer '$SCRIPT'"
    ;;
  codex)
    grep -q 'FACTORY_AGENT_ROLE=implementer' .codex/agents/implementer.toml
    grep -q 'test-edit-denial.sh' .codex/agents/implementer.toml
    PAYLOAD='{"tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Update File: '"$PROBE_FILE"'\n@@\n-old\n+new\n*** End Patch"}}'
    expect_deny sh -c "printf '%s' '$PAYLOAD' | FACTORY_AGENT_ROLE=implementer '$SCRIPT'"
    expect_allow sh -c "printf '%s' '$PAYLOAD' | FACTORY_AGENT_ROLE=spec-writer '$SCRIPT'"
    ;;
esac

echo "harness-structural-eval: PASS harness=$HARNESS"
echo "harness-structural-eval: live in-harness parity remains a separate OBSERVED gate"
