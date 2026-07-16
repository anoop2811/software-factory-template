#!/bin/bash
set -euo pipefail

# scripts/hooks/hook-existence-check.sh
# CI safety net for the fail-open decision in factory-hooks.ts.
#
# The plugin's catch block fails open: if a hook script is missing or
# unexecutable, edits are allowed with a log line. This is deliberate
# (fail-closed would halt the whole factory on a missing file, including
# test-writing by the spec-writer). But the fail-open path must never
# persist silently across a merge.
#
# This check verifies that every hook script referenced by the plugin and
# CI exists and is executable. If a script is missing or not executable,
# this check fails — catching the fail-open condition before it ships.

HOOK_SCRIPTS=(
  "scripts/lib/config.sh"
  "scripts/selftest/run.sh"
  "scripts/hooks/test-edit-denial.sh"
  "scripts/hooks/shared-script-enforcement.sh"
  "scripts/hooks/commit-message-lint.sh"
  "scripts/hooks/loop-close-check.sh"
  "scripts/hooks/diff-aware-check.sh"
  "scripts/hooks/decision-log-gate.sh"
  "scripts/hooks/pending-lessons-push-block.sh"
  "scripts/hooks/direct-main-push-block.sh"
  "scripts/citation-lint.sh"
  "scripts/sync-claude.sh"
  "scripts/sync-codex.sh"
  "scripts/harness-structural-eval.sh"
  "scripts/golden-task-eval.sh"
  "scripts/prereq-check.sh"
  "scripts/pre-push-check.sh"
  ".githooks/pre-push"
)

ERRORS=0

IN_REPO=0
git rev-parse --is-inside-work-tree >/dev/null 2>&1 && IN_REPO=1

for SCRIPT in "${HOOK_SCRIPTS[@]}"; do
  # A file that exists locally but is not tracked passes every local check
  # and then fails in CI's clean clone — check tracking, not just presence.
  if [ "$IN_REPO" -eq 1 ] && ! git ls-files --error-unmatch "$SCRIPT" >/dev/null 2>&1; then
    echo "HOOK-EXISTENCE FAIL: $SCRIPT exists locally but is NOT tracked by git (would be missing in a clean clone)"
    ERRORS=$((ERRORS + 1))
    continue
  fi
  if [ ! -f "$SCRIPT" ]; then
    echo "HOOK-EXISTENCE FAIL: $SCRIPT does not exist"
    ERRORS=$((ERRORS + 1))
    continue
  fi

  if [ ! -x "$SCRIPT" ]; then
    echo "HOOK-EXISTENCE FAIL: $SCRIPT is not executable"
    ERRORS=$((ERRORS + 1))
    continue
  fi

  echo "hook-existence: OK $SCRIPT (exists, executable)"
done

if [ "$ERRORS" -gt 0 ]; then
  echo "hook-existence-check: $ERRORS script(s) missing or not executable"
  echo "The plugin fails open when a script is missing — this check catches that before merge."
  exit 1
fi

echo "hook-existence-check: all hook scripts exist and are executable"
