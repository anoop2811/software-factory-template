#!/bin/bash
set -euo pipefail
_EVDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/events.sh
if [ -f "$_EVDIR/../lib/events.sh" ]; then . "$_EVDIR/../lib/events.sh"; else factory_log_event() { :; }; fi

# scripts/hooks/commit-message-lint.sh
# Computational enforcement of commit message conventions.
#
# Two concerns:
#   1. Conventional commits + length discipline
#      - subject: <type>(<scope>)?: <description>
#        types: feat|fix|chore|docs|refactor|test|ci|build|perf
#      - subject: no trailing period
#      - body: max 6 bullets, each bullet <= 25 words
#   2. The Verification Contract (docs/FACTORY_RULES.md,
#      memory/lessons/001-verification-contract.md)
#      - no "verified"/"fixed"/"works" claim without command + output citation
#      - "written but NOT verified" is always acceptable
#
# Usage:
#   ./scripts/hooks/commit-message-lint.sh <sha>   # check one commit
#   ./scripts/hooks/commit-message-lint.sh         # read one message from stdin
#   CI: for sha in $(git rev-list BASE..HEAD); do
#         ./scripts/hooks/commit-message-lint.sh "$sha"
#       done
#
# Merge and revert commits are exempt from the conventional-commits subject check.
# Bracket expressions ([[:space:]]) and [(] [)] are used instead of backslash
# escapes so the patterns are portable across GNU and BSD ERE (CI on Linux,
# local dev on macOS).

# Read the commit message
if [ -n "${1:-}" ]; then
  MESSAGE=$(git log --format=%B -1 "$1")
else
  if [ -t 0 ]; then
    echo "commit-message-lint: no sha argument and stdin is a terminal — pass a sha or pipe a message" >&2
    exit 2
  fi
  MESSAGE=$(cat)
fi

ERRORS=0

# Portable regex patterns (ERE via bash [[ =~ ]]).
CC_RE='^(feat|fix|chore|docs|refactor|test|ci|build|perf)([(][^)]+[)])?: .+'
PERIOD_RE='[.]$'
BULLET_RE='^[[:space:]]*[-*][[:space:]]+(.+)'
BLANK_RE='^[[:space:]]*$'
MERGE_RE='^Merge '
REVERT_RE='^Revert '

# ── 1. Subject line: conventional commits ──────────────────────────────
# First non-blank line is the subject.
SUBJECT=""
while IFS= read -r LINE; do
  if [[ ! "$LINE" =~ $BLANK_RE ]]; then
    SUBJECT="$LINE"
    break
  fi
done <<< "$MESSAGE"

# Exempt merge / revert commits from the type check.
if [[ ! "$SUBJECT" =~ $MERGE_RE && ! "$SUBJECT" =~ $REVERT_RE ]]; then
  if [[ ! "$SUBJECT" =~ $CC_RE ]]; then
    echo "COMMIT-LINT FAIL: subject line does not follow conventional commits:"
    echo "  $SUBJECT"
    echo "  Expected: <type>(<scope>)?: <description>"
    echo "  Types: feat|fix|chore|docs|refactor|test|ci|build|perf"
    ERRORS=$((ERRORS + 1))
  fi
  # Subject must not end with a period.
  if [[ "$SUBJECT" =~ $PERIOD_RE ]]; then
    echo "COMMIT-LINT FAIL: subject line ends with a period:"
    echo "  $SUBJECT"
    ERRORS=$((ERRORS + 1))
  fi
fi

# ── 2. Bullet count (max 6) and per-bullet word count (max 25) ─────────
# A bullet is a line matching ^[[:space:]]*[-*][[:space:]]+.+
# Wrapped continuation lines (non-blank, non-bullet, within the same
# paragraph as a bullet) are accumulated into the current bullet's word
# count. A blank line finalizes the current bullet. Known limitation:
# continuation lines are joined with a space, so leading indentation of
# wrapped lines does not affect the word count.
BULLET_COUNT=0
CURRENT_BULLET_TEXT=""
IN_BULLET=false

finalize_bullet() {
  if [ "$IN_BULLET" = true ]; then
    WORD_COUNT=$(printf '%s' "$CURRENT_BULLET_TEXT" | wc -w | tr -d ' ')
    if [ "$WORD_COUNT" -gt 25 ]; then
      echo "COMMIT-LINT FAIL: bullet #$BULLET_COUNT exceeds 25 words ($WORD_COUNT words):"
      echo "  $CURRENT_BULLET_TEXT"
      ERRORS=$((ERRORS + 1))
    fi
    IN_BULLET=false
    CURRENT_BULLET_TEXT=""
  fi
}

while IFS= read -r LINE; do
  if [[ "$LINE" =~ $BULLET_RE ]]; then
    finalize_bullet
    BULLET_COUNT=$((BULLET_COUNT + 1))
    CURRENT_BULLET_TEXT="${BASH_REMATCH[1]}"
    IN_BULLET=true
  elif [[ ! "$LINE" =~ $BLANK_RE ]]; then
    # Non-blank continuation line: accumulate if we're inside a bullet.
    if [ "$IN_BULLET" = true ]; then
      CURRENT_BULLET_TEXT="$CURRENT_BULLET_TEXT $LINE"
    fi
  else
    finalize_bullet
  fi
done <<< "$MESSAGE"
finalize_bullet

if [ "$BULLET_COUNT" -gt 6 ]; then
  echo "COMMIT-LINT FAIL: body has $BULLET_COUNT bullets, max is 6"
  echo "  Conventional commit bodies: short intro + max 5-6 points."
  ERRORS=$((ERRORS + 1))
fi

# ── 3. Verification Contract: no bare "verified"/"fixed"/"works" ───────
PREV_LINE=""
LINE_NO=0
TOTAL_LINES="$(printf '%s\n' "$MESSAGE" | wc -l | tr -d '[:space:]')"
while IFS= read -r LINE; do
  LINE_NO=$((LINE_NO + 1))
  # Everything after the current line, so a header can be judged by what follows
  # it rather than assumed innocent.
  REST_LINES="$(printf '%s\n' "$MESSAGE" | sed -n "$((LINE_NO + 1)),${TOTAL_LINES}p")"
  CLAIMS_VERIFICATION=false
  # Word-bounded so "frameworks" isn't read as a "works" claim, "prefixed" as
  # "fixed", etc. BSD grep lacks \b, so match on non-word neighbours / bounds.
  if echo "$LINE" | grep -qiE '(^|[^[:alnum:]_])(verified|fixed|works)([^[:alnum:]_]|$)'; then
    # "NOT verified" exempts the unverified statement — not every other claim that
    # happens to share the line. Previously one hedge anywhere suppressed the whole
    # line, so "fixed the parser; the rest is NOT verified" passed the `fixed`
    # claim unchecked. Strip the hedged phrases, then see whether a claim remains.
    RESIDUAL="$(printf '%s' "$LINE" | sed -E 's/(NOT|not|Not)[[:space:]]+verified//g; s/[Uu]nverified//g; s/NOT_VERIFIED//g')"
    if ! echo "$RESIDUAL" | grep -qiE '(^|[^[:alnum:]_])(verified|fixed|works)([^[:alnum:]_]|$)'; then
      PREV_LINE="$LINE"
      continue
    fi
    # A bare "Verified:" header is only acceptable when the lines beneath it
    # actually carry the evidence. It used to be skipped unconditionally, so a
    # header with nothing under it satisfied the contract.
    if echo "$LINE" | grep -qE '^[[:space:]]*(Verified|Fixed|Works):[[:space:]]*$'; then
      if printf '%s\n' "$REST_LINES" | grep -qE '(`[^`]+`|→|exit:|go test|go vet|grep |rg |find |ls |git |cat |sed |make )'; then
        PREV_LINE="$LINE"
        continue
      fi
      echo "COMMIT-LINT FAIL: a bare '$LINE' header with no evidence beneath it:"
      echo "  Put the command and its output on the following lines, or write"
      echo "  'written but NOT verified'."
      ERRORS=$((ERRORS + 1))
      PREV_LINE="$LINE"
      continue
    fi
    CLAIMS_VERIFICATION=true
  fi

  if [ "$CLAIMS_VERIFICATION" = true ]; then
    # Check if this line or the next few lines contain evidence:
    # - A command in backticks: `command`
    # - A command with output marker
    # - An "exit:" marker
    # - An explicit command citation (e.g., "go test -race" followed by output)
    HAS_EVIDENCE=false

    # Check current line for inline evidence
    if echo "$LINE" | grep -qE '(`[^`]+`|→|exit:|go test|go vet|grep |rg |find |ls |git |cat |sed |make )'; then
      HAS_EVIDENCE=true
    fi

    # Check previous line (might be a header followed by evidence)
    if [ "$HAS_EVIDENCE" = false ] && echo "$PREV_LINE" | grep -qE '(`[^`]+`|→|exit:|go test|go vet)'; then
      HAS_EVIDENCE=true
    fi

    if [ "$HAS_EVIDENCE" = false ]; then
      echo "COMMIT-LINT FAIL: line claims verification but lacks command + output citation:"
      echo "  $LINE"
      echo "  Every 'verified'/'fixed'/'works' claim must cite the exact command and paste its output."
      echo "  Or write 'written but NOT verified' if you did not execute the check."
      ERRORS=$((ERRORS + 1))
    fi
  fi

  PREV_LINE="$LINE"
done <<< "$MESSAGE"

# ── Result ─────────────────────────────────────────────────────────────
if [ "$ERRORS" -gt 0 ]; then
  echo "commit-message-lint: $ERRORS violation(s) found"
  factory_log_event "commit-message-lint" "commit message violation"
  exit 1
fi

echo "commit-message-lint: OK (conventional commits, <=6 bullets <=25 words, verification claims cited)"
