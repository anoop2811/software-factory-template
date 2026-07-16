#!/bin/bash
set -euo pipefail

# scripts/hooks/loop-close-check.sh
# Second-brain loop-close trigger (Karpathy pattern).
#
# Called by the opencode plugin's dispose hook at session end.
# Checks: did files change during this session without a corresponding
# lesson being written to memory/lessons/? If so, writes a reminder
# to memory/PENDING-LESSONS.md.
#
# This is a NUDGE, not enforcement. It cannot block session close
# (the opencode API has no session-close event; dispose is best-effort).
# The AGENTS.md "loop-close" rule is the agent-discipline backstop.
#
# Usage (called from factory-hooks.ts dispose hook):
#   FACTORY_SESSION_START_HEAD=<sha> ./scripts/hooks/loop-close-check.sh
#
# Exit codes:
#   0 — no changes, or changes + lessons written (silent)
#   1 — changes made but no new lesson files (reminder written)

START_HEAD="${FACTORY_SESSION_START_HEAD:-}"
REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || echo .)}"
LESSONS_DIR="$REPO_ROOT/memory/lessons"
PENDING_FILE="$REPO_ROOT/memory/PENDING-LESSONS.md"

# If no start HEAD recorded, nothing to compare against — exit silently.
if [ -z "$START_HEAD" ]; then
  exit 0
fi

# Get the list of files changed since session start (tracked changes only).
CHANGED_FILES=$(git diff --name-only "$START_HEAD" 2>/dev/null || true)

# If no tracked changes, check for untracked (new files not yet added).
UNTRACKED=$(git ls-files --others --exclude-standard 2>/dev/null || true)

# Combine: if nothing changed at all, exit silently.
if [ -z "$CHANGED_FILES" ] && [ -z "$UNTRACKED" ]; then
  exit 0
fi

# Check if any new lesson files exist (tracked or untracked).
# Tracked: new files in memory/lessons/ since START_HEAD.
NEW_TRACKED_LESSONS=$(git diff --name-only --diff-filter=A "$START_HEAD" -- memory/lessons/ 2>/dev/null || true)
# Untracked: new files in memory/lessons/ not yet staged.
NEW_UNTRACKED_LESSONS=$(git ls-files --others --exclude-standard -- memory/lessons/ 2>/dev/null || true)

if [ -n "$NEW_TRACKED_LESSONS" ] || [ -n "$NEW_UNTRACKED_LESSONS" ]; then
  # Lessons were written — loop closed properly. Remove any stale reminder.
  rm -f "$PENDING_FILE"
  exit 0
fi

# Changes exist but no lessons written — write reminder.
mkdir -p "$LESSONS_DIR"
cat > "$PENDING_FILE" << EOF
# Pending lessons — loop-close reminder

Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)
Session start HEAD: $START_HEAD

Files changed this session without a corresponding lesson in memory/lessons/:

$(echo "$CHANGED_FILES $UNTRACKED" | tr ' ' '\n' | grep -v '^$' | sed 's/^/  - /')

## What to do

If any of these changes involved a non-obvious fact — a gotcha, a version
mismatch, an API shape, a bug fix that cost time — write it to
memory/lessons/NNN-*.md with provenance (file:line, fetched URL + date, or
"observed YYYY-MM-DD via <action>"). Then delete this file.

See AGENTS.md "Second-brain loop-close" rule.
EOF

echo "loop-close-check: changes detected but no lessons written"
echo "  Reminder written to memory/PENDING-LESSONS.md"
exit 1
