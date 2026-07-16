#!/bin/bash
set -euo pipefail

# scripts/pre-push-check.sh
# Pre-push gate: runs all checks that must pass before pushing to origin.
# This is the cobblers-children gate — it catches the miss where we
# wrote a rule, didn't run the check, and broke our own rule.
#
# Install as a git pre-push hook:
#   cp scripts/pre-push-check.sh .git/hooks/pre-push
#   chmod +x .git/hooks/pre-push
#
# Or run manually before pushing:
#   ./scripts/pre-push-check.sh
#
# Exit 0 = all checks passed — push allowed
# Exit 1 = one or more checks failed — push blocked

REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || echo .)}"
cd "$REPO_ROOT"

ERRORS=0

echo "========================================"
echo "  PRE-PUSH CHECK"
echo "========================================"
echo ""

# ── 1. Direct-main denial ────────────────────────────────────────────
echo "[1/7] direct-main-push-block (feature branches + PRs only)"
if ./scripts/hooks/direct-main-push-block.sh; then
  echo "  PASS"
else
  echo "  FAIL"
  exit 1
fi
echo ""

# ── 2. make check (lint + test + sec + vuln + citation-lint + hooks) ──
echo "[2/7] make check (lint, test, security, citations, hooks)"
if make check 2>&1; then
  echo "  PASS"
else
  echo "  FAIL"
  ERRORS=$((ERRORS + 1))
fi
echo ""

# ── 3. make check-drift (sync adapters + verify no drift) ────────────
echo "[3/7] make check-drift (Claude + Codex adapter drift)"
if make check-drift 2>&1; then
  echo "  PASS"
else
  echo "  FAIL"
  ERRORS=$((ERRORS + 1))
fi
echo ""

# ── 4. Full-range commit-message lint (origin/main..HEAD) ────────────
echo "[4/7] commit-message-lint (full unpushed range)"
ORIGIN_HEAD=$(git rev-parse origin/main 2>/dev/null || echo "")
if [ -n "$ORIGIN_HEAD" ]; then
  RANGE_FAIL=0
  RANGE_TOTAL=0
  for sha in $(git rev-list "$ORIGIN_HEAD..HEAD" 2>/dev/null || true); do
    RANGE_TOTAL=$((RANGE_TOTAL + 1))
    if ! ./scripts/hooks/commit-message-lint.sh "$sha" 2>&1; then
      RANGE_FAIL=1
    fi
  done
  if [ "$RANGE_FAIL" -ne 0 ]; then
    echo "  FAIL ($RANGE_TOTAL commits checked, at least one violation)"
    ERRORS=$((ERRORS + 1))
  elif [ "$RANGE_TOTAL" -eq 0 ]; then
    echo "  SKIP (no unpushed commits)"
  else
    echo "  PASS ($RANGE_TOTAL commits checked)"
  fi
else
  echo "  SKIP (no origin/main found)"
fi
echo ""

# ── 5. diff-aware-check (dispatch checks based on changed files) ──────
echo "[5/7] diff-aware-check (dispatch checks for changed files)"
ORIGIN_HEAD=$(git rev-parse origin/main 2>/dev/null || echo "HEAD")
if ./scripts/hooks/diff-aware-check.sh "$ORIGIN_HEAD" HEAD 2>&1; then
  echo "  PASS"
else
  echo "  FAIL"
  ERRORS=$((ERRORS + 1))
fi
echo ""

# ── 6. decision-log-gate (governance commits reference a Decision) ────
echo "[6/7] decision-log-gate (governance commits reference a Decision)"
ORIGIN_HEAD=$(git rev-parse origin/main 2>/dev/null || echo "")
if [ -n "$ORIGIN_HEAD" ]; then
  if ./scripts/hooks/decision-log-gate.sh "$ORIGIN_HEAD" HEAD 2>&1; then
    echo "  PASS"
  else
    echo "  FAIL"
    ERRORS=$((ERRORS + 1))
  fi
else
  echo "  SKIP (no origin/main found)"
fi
echo ""

# ── 7. pending-lessons-push-block ─────────────────────────────────────
echo "[7/7] pending-lessons-push-block (no unaddressed loop-close reminders)"
if ./scripts/hooks/pending-lessons-push-block.sh 2>&1; then
  echo "  PASS"
else
  echo "  FAIL"
  ERRORS=$((ERRORS + 1))
fi
echo ""

# ── Summary ───────────────────────────────────────────────────────────
echo "========================================"
if [ "$ERRORS" -gt 0 ]; then
  echo "  PRE-PUSH: $ERRORS CHECK(S) FAILED — PUSH BLOCKED"
  echo "========================================"
  exit 1
fi

echo "  PRE-PUSH: ALL CHECKS PASSED — PUSH ALLOWED"
echo "========================================"
exit 0
