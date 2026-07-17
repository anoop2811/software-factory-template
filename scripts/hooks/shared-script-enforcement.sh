#!/bin/bash
set -euo pipefail

# scripts/hooks/shared-script-enforcement.sh
# Computational check: verifies that opencode, Claude Code, and Codex adapter
# surfaces call scripts/hooks/*.sh rather than reimplementing the rule.
#
# This is the hook that would have caught critical #5 (plugin contradicted
# the shared-script rule by reimplementing test-edit denial inline).
#
# Enforcement logic lives in shared shell scripts; all harnesses are thin wrappers.

PLUGIN_DIR=".opencode/plugin"

if [ ! -d "$PLUGIN_DIR" ]; then
  echo "shared-script-enforcement: no $PLUGIN_DIR — skipping"
  exit 0
fi

ERRORS=0

# Known enforcement scripts that must be called from the plugin, not reimplemented.
ENFORCEMENT_SCRIPTS="test-edit-denial.sh"

# Known inline patterns that indicate reimplemented enforcement logic.
INLINE_PATTERNS="_test\.go"

for PLUGIN_FILE in "$PLUGIN_DIR"/*.ts; do
  [ -f "$PLUGIN_FILE" ] || continue

  echo "shared-script-enforcement: checking $PLUGIN_FILE"

  CODE_ONLY=$(sed 's|//.*||g; s|/\*.*\*/||g' "$PLUGIN_FILE")

  for SCRIPT in $ENFORCEMENT_SCRIPTS; do
    if ! echo "$CODE_ONLY" | grep -qE "(execFile|spawn).*${SCRIPT}|${SCRIPT}.*execFile|${SCRIPT}.*spawn" 2>/dev/null; then
      if ! echo "$CODE_ONLY" | grep -qE "execFile.*script|script.*execFile" 2>/dev/null; then
        echo "SHARED-SCRIPT FAIL: $PLUGIN_FILE does not call $SCRIPT via execFile/spawn"
        echo "  Enforcement logic must call scripts/hooks/$SCRIPT."
        ERRORS=$((ERRORS + 1))
      fi
    fi
  done

  for PATTERN in $INLINE_PATTERNS; do
    if echo "$CODE_ONLY" | grep -qE "$PATTERN" 2>/dev/null; then
      if echo "$CODE_ONLY" | grep -qE "(if|&&|\?\.).*${PATTERN}|test\.*${PATTERN}" 2>/dev/null; then
        echo "SHARED-SCRIPT FAIL: $PLUGIN_FILE contains inline enforcement pattern: $PATTERN"
        echo "  This logic belongs in scripts/hooks/, not in the plugin."
        ERRORS=$((ERRORS + 1))
      fi
    fi
  done

  STRIPPED=$(echo "$CODE_ONLY" | sed 's/execFile(//g')
  if echo "$STRIPPED" | grep -qE '\bexec\('; then
    echo "SHARED-SCRIPT WARN: $PLUGIN_FILE uses exec() — use execFile() to prevent command injection"
    ERRORS=$((ERRORS + 1))
  fi
done

# Generated adapters must delegate to the same script and make the implementer
# role explicit at the hook boundary. These files are produced by
# sync-claude.sh / sync-codex.sh — if they are absent (a fresh clone before
# `make sync-harnesses`), skip rather than fail, the way hook-existence-check
# treats a missing script. The drift check runs sync first, so real
# divergence is still caught there.
if [ -f .claude/settings.json ]; then
  if ! grep -q 'scripts/hooks/test-edit-denial.sh' .claude/settings.json; then
    echo "SHARED-SCRIPT FAIL: .claude/settings.json does not call test-edit-denial.sh"
    ERRORS=$((ERRORS + 1))
  fi
  if ! grep -q 'FACTORY_AGENT_ROLE=implementer' .claude/agents/implementer.md 2>/dev/null; then
    echo "SHARED-SCRIPT FAIL: Claude implementer hook does not set FACTORY_AGENT_ROLE"
    ERRORS=$((ERRORS + 1))
  fi
else
  echo "shared-script-enforcement: no .claude adapter yet — run 'make sync-harnesses' (skipping)"
fi

if [ -f .codex/agents/implementer.toml ]; then
  if ! grep -q 'scripts/hooks/test-edit-denial.sh' .codex/agents/implementer.toml; then
    echo "SHARED-SCRIPT FAIL: .codex/agents/implementer.toml does not call test-edit-denial.sh"
    ERRORS=$((ERRORS + 1))
  fi
  if ! grep -q 'FACTORY_AGENT_ROLE=implementer' .codex/agents/implementer.toml; then
    echo "SHARED-SCRIPT FAIL: Codex implementer hook does not set FACTORY_AGENT_ROLE"
    ERRORS=$((ERRORS + 1))
  fi
else
  echo "shared-script-enforcement: no .codex adapter yet — run 'make sync-harnesses' (skipping)"
fi

if [ "$ERRORS" -gt 0 ]; then
  echo "shared-script-enforcement: $ERRORS violation(s) found"
  exit 1
fi

echo "shared-script-enforcement: all harness adapters use shared scripts correctly"
