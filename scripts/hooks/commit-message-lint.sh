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
# A command citation, and separately the observation it produced. Both are needed:
# see the evidence window below.
EVIDENCE_CMD_RE='(`[^`]+`|go test|go vet|golangci-lint|gosec |grep |rg |find |ls |git |cat |sed |make |curl |xh )'
# What the command produced. Deliberately broad, because the point is to force a
# statement of the result, not to dictate its wording — and a grammar that
# rejected Go's own `ok  pkg  0.003s`, or "3 issues", was rejecting real output.
# `ok` is matched with explicit non-word neighbours: BSD grep has no \b.
EVIDENCE_OUTCOME_RE='(exit:|exit status|→|[0-9]+ (passed|failed|ok|error|errors|warning|warnings|violation|violations|issue|issues|problem|problems|finding|findings|assertion|assertions|spec|specs|example|examples|subtest|subtests|check|checks|test|tests|file|files|byte|bytes|line|lines|commit|commits)|no (diff|diffs|change|changes|output|findings|issues|problems|violations|test files)|(^|[^[:alnum:]_])(ok|OK|PASS|FAIL|passed|failed|clean|unchanged)([^[:alnum:]_]|$)|output:|--- (PASS|FAIL))'
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
    # Lowercase first, then strip. Enumerating capitalisations in the sed script
    # missed the ones people actually type — `NOT VERIFIED` kept the word
    # `VERIFIED` in the residual, so the hook rejected a correctly hedged
    # message. GNU sed's /I flag is not available on BSD sed, so fold the case
    # instead of relying on it.
    RESIDUAL="$(printf '%s' "$LINE" | tr '[:upper:]' '[:lower:]' \
      | sed -E 's/not[[:space:]]+verified//g; s/unverified//g; s/not_verified//g')"
    if ! echo "$RESIDUAL" | grep -qE '(^|[^[:alnum:]_])(verified|fixed|works)([^[:alnum:]_]|$)'; then
      PREV_LINE="$LINE"
      continue
    fi
    # A bare "Verified:" header is only acceptable when the lines beneath it
    # actually carry the evidence. It used to be skipped unconditionally, so a
    # header with nothing under it satisfied the contract.
    if echo "$LINE" | grep -qE '^[[:space:]]*(Verified|Fixed|Works):[[:space:]]*$'; then
      # Only the contiguous block under the header counts as its evidence. Scanning
      # everything after the header let an unrelated later paragraph — a footer
      # citing `make check`, say — stand in as evidence for a header that had a
      # blank line and nothing else beneath it.
      EVIDENCE_BLOCK="$(printf '%s\n' "$REST_LINES" | sed -n '/^[[:space:]]*$/q;p')"
      if printf '%s\n' "$EVIDENCE_BLOCK" | grep -qE '(`[^`]+`|→|exit:|go test|go vet|grep |rg |find |ls |git |cat |sed |make )'; then
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
    # Evidence is a command AND what it produced. A cited command on its own used
    # to satisfy this check, so "fixed by running `go mod vendor`" passed as a
    # verified claim while stating no observation at all — the RAN half of the
    # Verification Contract standing in for the OBSERVED half.
    #
    # The window is the claim's line, the line above it (a header), and the
    # contiguous block beneath it, since a command and its output are usually on
    # adjacent lines.
    # The window stops at the next bullet as well as at the next blank line, and a
    # bullet does not reach back to the bullet above it. Otherwise one verified
    # item lent its command and outcome to every unverified item under it — a list
    # where the first entry was checked read as a list where all of them were.
    EVIDENCE_TAIL="$(printf '%s\n' "$REST_LINES" |
      awk '/^[[:space:]]*$/ { exit } /^[[:space:]]*[-*][[:space:]]+/ { exit } { print }')"
    if echo "$LINE" | grep -qE '^[[:space:]]*[-*][[:space:]]+'; then
      EVIDENCE_WINDOW="$(printf '%s\n%s\n' "$LINE" "$EVIDENCE_TAIL")"
    else
      EVIDENCE_WINDOW="$(printf '%s\n%s\n%s\n' "$PREV_LINE" "$LINE" "$EVIDENCE_TAIL")"
    fi
    HAS_COMMAND=false
    HAS_OUTCOME=false
    if echo "$EVIDENCE_WINDOW" | grep -qE "$EVIDENCE_CMD_RE"; then HAS_COMMAND=true; fi
    if echo "$EVIDENCE_WINDOW" | grep -qE "$EVIDENCE_OUTCOME_RE"; then HAS_OUTCOME=true; fi

    if [ "$HAS_COMMAND" = false ] || [ "$HAS_OUTCOME" = false ]; then
      echo "COMMIT-LINT FAIL: line claims verification but lacks command + output citation:"
      echo "  $LINE"
      if [ "$HAS_COMMAND" = true ] && [ "$HAS_OUTCOME" = false ]; then
        echo "  The command is cited but not what it produced. Add the outcome —"
        echo "  'exit: 0', a count, or the line the command printed."
      else
        echo "  Every 'verified'/'fixed'/'works' claim must cite the exact command and paste its output."
      fi
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
