#!/bin/bash
set -euo pipefail

# scripts/hooks/decision-log-gate.sh
# Verifies that commits touching governance-sensitive paths reference a
# Decision number in the decision log.
#
# The rule this hook enforces (from AGENTS.md):
#   "Every decision goes in the decision log (or an ADR) before code, not after."
#
# This hook catches the failure where code is written that implements a
# decision, but the decision was never recorded in the decision log.
#
# Governance-sensitive paths (files that, if changed, likely implement a
# decision):
#   - __PROTECTED_PATH__/          (permanently human-reviewed)
#   - opencode.json           (canonical config)
#   - .opencode/              (agents, plugin, skills)
#   - .claude/                (Claude adapter)
#   - .codex/                 (Codex adapter)
#   - scripts/                (hooks, enforcement logic)
#   - docs/adr/               (ADR changes are themselves decisions)
#   - .github/CODEOWNERS      (governance)
#   - .github/workflows/      (CI is governance)
#   - Makefile                (build targets are governance)
#   - specs/                  (feature specs — canonical source)
#
# Usage:
#   ./scripts/hooks/decision-log-gate.sh                # check working tree
#   ./scripts/hooks/decision-log-gate.sh <base> <head> # check base..head range
#
# Exit 0 = all governance-touching commits reference a Decision, or no governance paths changed
# Exit 1 = a governance-touching commit lacks a Decision reference

BASE="${1:-HEAD}"
HEAD_REF="${2:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/config.sh
. "$SCRIPT_DIR/../lib/config.sh"

REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || echo .)}"
DECISION_LOG="$REPO_ROOT/$(factory_config_get decision_log docs/DECISION_LOG.md)"

# Cutoff: commits that are ancestors of the configured cutoff are exempt —
# pre-rule history is not retroactively linted. Set `decision_gate_cutoff`
# in factory.yaml to the commit where this gate was adopted.
CUTOFF_SHA="$(factory_config_get decision_gate_cutoff 0000000)"

# Governance-sensitive path patterns: the factory's own surfaces, plus every
# prefix listed in factory.yaml `protected_paths` (space-separated).
GOVERNANCE_PATTERNS='^opencode\.json$|^\.opencode/|^\.claude/|^\.codex/|^scripts/|^docs/adr/|^\.github/CODEOWNERS$|^\.github/workflows/|^Makefile$|^specs/|^factory\.yaml$'
for PROTECTED in $(factory_config_get protected_paths); do
  ESCAPED="$(printf '%s' "$PROTECTED" | sed 's/[.[\*^$()+?{|]/\\&/g')"
  GOVERNANCE_PATTERNS="$GOVERNANCE_PATTERNS|^$ESCAPED"
done

# Get the list of commits to check
if [ -n "$HEAD_REF" ]; then
  COMMITS=$(git rev-list "$BASE..$HEAD_REF" 2>/dev/null || true)
elif [ "$BASE" = "HEAD" ]; then
  COMMITS=""
else
  COMMITS=$(git rev-list "$BASE" 2>/dev/null || true)
fi

ERRORS=0

# If no commits to check, check the working tree diff
if [ -z "$COMMITS" ]; then
  CHANGED=$(git diff --name-only HEAD 2>/dev/null || true)
  UNTRACKED=$(git ls-files --others --exclude-standard 2>/dev/null || true)
  CHANGED="$CHANGED
$UNTRACKED"

  GOV_CHANGED=$(echo "$CHANGED" | grep -E "$GOVERNANCE_PATTERNS" 2>/dev/null || true)
  if [ -z "$GOV_CHANGED" ]; then
    echo "decision-log-gate: no governance-sensitive paths changed"
    exit 0
  fi

  echo "decision-log-gate: governance-sensitive paths changed:"
  echo "$GOV_CHANGED" | sed 's/^/  - /'
  echo ""
  echo "  Verify that the decision driving these changes is recorded in"
  echo "  the decision log. If not, add it before committing."
  echo "  (This is a write-time rule reminder — the gate does not block on"
  echo "  uncommitted changes, only on committed ones.)"
  exit 0
fi

# Check each commit
for sha in $COMMITS; do
  # Skip pre-rule commits (ancestors of cutoff)
  if git merge-base --is-ancestor "$sha" "$CUTOFF_SHA" 2>/dev/null; then
    continue
  fi

  # Skip merge commits (2+ parents). A merge authors no new change; the
  # governance change it carries is attributed to the real commit, which is
  # checked on its own. Without this, CI's synthetic refs/pull/N/merge commit
  # (message "Merge ... into ...") fails the Decision check for changes it
  # only inherits.
  if [ "$(git rev-list --no-walk --count --merges "$sha" 2>/dev/null || echo 0)" -gt 0 ]; then
    continue
  fi

  short=$(git log --format='%h' -1 "$sha")
  CHANGED=$(git diff --name-only "$sha^" "$sha" 2>/dev/null || true)

  GOV_CHANGED=$(echo "$CHANGED" | grep -E "$GOVERNANCE_PATTERNS" 2>/dev/null || true)
  if [ -z "$GOV_CHANGED" ]; then
    continue
  fi

  # Check if the commit message references a Decision number
  MESSAGE=$(git log --format='%B' -1 "$sha")
  if echo "$MESSAGE" | grep -qiE 'Decision([[:space:]]+|:[[:space:]]*)[0-9]+|ADR-[0-9]+|decision.log'; then
    # A reference is not enough: numbered Decisions must exist in the log.
    MISSING=0
    if [ -f "$DECISION_LOG" ]; then
      for NUM in $(echo "$MESSAGE" | grep -oiE 'Decision([[:space:]]+|:[[:space:]]*)[0-9]+' | grep -oE '[0-9]+' | sort -u); do
        if ! grep -qE "^## Decision $NUM\b" "$DECISION_LOG"; then
          echo "DECISION-LOG-GATE FAIL: commit $short references Decision $NUM,"
          echo "  but $DECISION_LOG has no '## Decision $NUM' entry. Record it first."
          MISSING=1
        fi
      done
    fi
    if [ "$MISSING" -ne 0 ]; then
      ERRORS=$((ERRORS + 1))
      continue
    fi
    echo "decision-log-gate: OK $short (governance paths changed, Decision ref found)"
    continue
  fi

  echo "DECISION-LOG-GATE FAIL: commit $short touches governance-sensitive paths"
  echo "  but does not reference a Decision number or ADR in the commit message."
  echo "  Changed governance paths:"
  echo "$GOV_CHANGED" | sed 's/^/    - /'
  echo ""
  echo "  Add 'Decision N', 'Decision: N', or 'ADR-NNNN' to the commit message, or record the"
  echo "  decision in the decision log and amend the commit."
  ERRORS=$((ERRORS + 1))
done

if [ "$ERRORS" -gt 0 ]; then
  echo "decision-log-gate: $ERRORS commit(s) lack a Decision reference"
  exit 1
fi

echo "decision-log-gate: all governance-touching commits reference a Decision"
exit 0
