#!/bin/bash
set -euo pipefail
# shellcheck source=lib/events.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../lib/events.sh"

# scripts/hooks/pending-lessons-push-block.sh
# Blocks push if memory/PENDING-LESSONS.md exists — the loop-close check
# wrote a reminder that was never addressed.
#
# The loop-close-check.sh (called by the dispose hook) writes
# memory/PENDING-LESSONS.md when files changed during a session but no
# corresponding lesson was written to memory/lessons/. This hook turns
# that nudge into push-time enforcement: the nudge is ignorable, the
# push block is not.
#
# Also checks for memory/.parity-stale (written by diff-aware-check.sh
# when factory-hooks.ts is modified — flags that the previous OBSERVED
# parity pass is stale and requires re-verification).
#
# Usage:
#   ./scripts/hooks/pending-lessons-push-block.sh
#
# Exit 0 = no pending reminders — push allowed
# Exit 1 = pending reminder(s) exist — push blocked

REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || echo .)}"

ERRORS=0

# ── Check 1: PENDING-LESSONS.md ───────────────────────────────────────
PENDING_FILE="$REPO_ROOT/memory/PENDING-LESSONS.md"
if [ -f "$PENDING_FILE" ]; then
  echo "PENDING-LESSONS BLOCK: $PENDING_FILE exists"
  echo "  Files changed during a previous session without a corresponding lesson."
  echo "  Either:"
  echo "    1. Write the lesson to memory/lessons/NNN-*.md with provenance, then delete the reminder"
  echo "    2. Or delete the reminder if no lesson is warranted (and accept the nudge was ignored)"
  echo ""
  cat "$PENDING_FILE"
  echo ""
  ERRORS=$((ERRORS + 1))
fi

# ── Check 2: .parity-stale flag ──────────────────────────────────────
PARITY_FLAG="$REPO_ROOT/memory/.parity-stale"
if [ -f "$PARITY_FLAG" ]; then
  echo "PARITY-STALE BLOCK: $PARITY_FLAG exists"
  echo "  factory-hooks.ts was modified since the last OBSERVED parity pass."
  echo "  The previous OBSERVED verification is now stale."
  echo "  Either:"
  echo "    1. Re-run the live parity eval (opencode as implementer, attempt *_test.go edit)"
  echo "    2. Or mark the parity claim as OPEN in wiki/opencode-harness.md"
  echo "  Then delete the flag file."
  echo ""
  cat "$PARITY_FLAG"
  echo ""
  ERRORS=$((ERRORS + 1))
fi

# ── Check 3: .pending-lesson-reminder (per-turn flag) ────────────────
REMINDER_FILE="$REPO_ROOT/memory/.pending-lesson-reminder"
if [ -f "$REMINDER_FILE" ]; then
  echo "PENDING-REMINDER: $REMINDER_FILE exists"
  echo "  A session.idle nudge fired but was not addressed."
  echo "  Reflect on whether the previous turn revealed a non-obvious fact."
  echo "  If so, write memory/lessons/NNN-*.md. Then delete the reminder."
  echo ""
  cat "$REMINDER_FILE"
  echo ""
  ERRORS=$((ERRORS + 1))
fi

if [ "$ERRORS" -gt 0 ]; then
  echo "pending-lessons-push-block: $ERRORS pending reminder(s) — push blocked"
  echo "  Address the reminder(s) above, or delete the flag file(s) to override."
  factory_log_event "pending-lessons-push-block" "unaddressed loop-close reminder"
  exit 1
fi

echo "pending-lessons-push-block: no pending reminders — push allowed"
exit 0
