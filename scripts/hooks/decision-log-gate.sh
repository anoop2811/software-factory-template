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
#   - protected_paths from factory.yaml (permanently human-reviewed)
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
# shellcheck source=../lib/events.sh
if [ -f "$SCRIPT_DIR/../lib/events.sh" ]; then . "$SCRIPT_DIR/../lib/events.sh"; else factory_log_event() { :; }; fi

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
# `|| true` here used to swallow an unresolvable ref, leaving COMMITS empty and
# passing the gate — so a typo'd base, a shallow clone without the base commit, or
# a fetch that had not happened yet all read as "no governance commits to check".
# A range this gate cannot enumerate is a range it cannot vouch for.
# Sets COMMITS to the enumerated revisions, REV_LIST_ERR to whatever git wrote on
# stderr, and returns git's exit status. stderr is captured to a file rather than
# folded in with `2>&1`: git prints "warning: refname 'x' is ambiguous" and still
# exits 0, and a warning line merged into the list becomes a bogus SHA that makes
# the checking loop die with "invalid object name 'warning'" — the gate then
# aborts without ever examining the commits it exists to examine.
rev_list_into_commits() {
  local err_file rc
  err_file="$(mktemp 2>/dev/null)" || err_file=""
  if [ -z "$err_file" ]; then
    REV_LIST_ERR="(git stderr unavailable: mktemp failed)"
    COMMITS="$(git rev-list "$@" 2>/dev/null)" && rc=0 || rc=$?
    return "$rc"
  fi
  COMMITS="$(git rev-list "$@" 2>"$err_file")" && rc=0 || rc=$?
  REV_LIST_ERR="$(cat "$err_file" 2>/dev/null || true)"
  rm -f "$err_file"
  return "$rc"
}

REV_LIST_ERR=""
if [ -n "$HEAD_REF" ]; then
  if ! rev_list_into_commits "$BASE..$HEAD_REF"; then
    echo "DECISION-LOG-GATE FAIL: cannot list commits in $BASE..$HEAD_REF" >&2
    echo "  $REV_LIST_ERR" >&2
    echo "  Fetch the base ref (a shallow clone may not have it) and retry." >&2
    factory_log_event "decision-log-gate" "commit range could not be enumerated"
    exit 1
  fi
elif [ "$BASE" = "HEAD" ]; then
  COMMITS=""
else
  if ! rev_list_into_commits "$BASE"; then
    echo "DECISION-LOG-GATE FAIL: cannot list commits from $BASE" >&2
    echo "  $REV_LIST_ERR" >&2
    factory_log_event "decision-log-gate" "commit range could not be enumerated"
    exit 1
  fi
fi
# A warning is not a pass. `git rev-list` prints "warning: refname 'x' is
# ambiguous" and still exits 0, and the range it then resolved may not be the one
# meant — possibly an empty one, which this gate would read as "no governance
# commits to check". Anything on stderr means the enumeration cannot be vouched
# for, so it fails here rather than reporting a clean range it is unsure of.
if [ -n "$REV_LIST_ERR" ]; then
  echo "DECISION-LOG-GATE FAIL: git could not enumerate the range unambiguously." >&2
  echo "  $REV_LIST_ERR" >&2
  echo "  Disambiguate the refs (refs/heads/<name>, refs/tags/<name>, or a SHA)" >&2
  echo "  and run this again — the range this resolved to may not be yours." >&2
  factory_log_event "decision-log-gate" "commit range could not be enumerated unambiguously"
  exit 1
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
  # `decision.log` was accepted as a reference on its own, so "see the decision
  # log" satisfied the gate while identifying nothing. A reference has to name
  # which decision: a number, or an ADR id.
  if echo "$MESSAGE" | grep -qiE 'Decision([[:space:]]+|:[[:space:]]*)[0-9]+|ADR-[0-9]+'; then
    # A reference is not enough: numbered Decisions must exist in the log.
    MISSING=0
    # Without the log there is nothing to verify against, so a numbered reference
    # cannot be accepted: the check would be vacuous, which is how a governance
    # gate ends up approving a Decision that was never written down.
    if [ ! -f "$DECISION_LOG" ] && echo "$MESSAGE" | grep -qiE 'Decision([[:space:]]+|:[[:space:]]*)[0-9]+'; then
      echo "DECISION-LOG-GATE FAIL: commit $short references a Decision,"
      echo "  but the configured log $DECISION_LOG does not exist, so the"
      echo "  reference cannot be verified. Create it, or fix decision_log."
      MISSING=1
    fi
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
  factory_log_event "decision-log-gate" "governance change without a Decision reference"
  exit 1
fi

echo "decision-log-gate: all governance-touching commits reference a Decision"
exit 0
