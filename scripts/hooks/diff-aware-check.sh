#!/bin/bash
set -euo pipefail

# scripts/hooks/diff-aware-check.sh
# Diff-aware verification map: reads changed files and dispatches the
# appropriate re-verification checks.
#
# The root cause this hook catches: modifying a file that has an associated
# verification check, then not running that check. (e.g., changing
# factory-hooks.ts after an OBSERVED pass without re-running parity eval,
# changing opencode.json without regenerating both adapters.)
#
# Usage:
#   ./scripts/hooks/diff-aware-check.sh                # diff working tree vs HEAD
#   ./scripts/hooks/diff-aware-check.sh <base> <head>  # diff base..head
#
# Exit 0 = all dispatched checks passed (or no checks needed)
# Exit 1 = one or more dispatched checks failed

BASE="${1:-HEAD}"
HEAD_REF="${2:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/config.sh
. "$SCRIPT_DIR/../lib/config.sh"

REPO_ROOT="${REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null || echo .)}"

# Get the list of changed files
if [ -n "$HEAD_REF" ]; then
  CHANGED_FILES=$(git diff --name-only "$BASE" "$HEAD_REF" 2>/dev/null || true)
else
  CHANGED_FILES=$(git diff --name-only HEAD 2>/dev/null || true)
  UNTRACKED=$(git ls-files --others --exclude-standard 2>/dev/null || true)
  CHANGED_FILES="$CHANGED_FILES
$UNTRACKED"
fi

if [ -z "$(echo "$CHANGED_FILES" | tr -d '[:space:]')" ]; then
  echo "diff-aware-check: no changes detected — nothing to verify"
  exit 0
fi

ERRORS=0
CHECKS_RAN=0

# ── Rule: opencode.json changed → sync all adapters + drift check ─────
if echo "$CHANGED_FILES" | grep -q '^opencode\.json$'; then
  echo "diff-aware-check: opencode.json changed — running harness sync + drift check"
  CHECKS_RAN=$((CHECKS_RAN + 1))
  if ! (cd "$REPO_ROOT" && make check-drift) 2>&1; then
    echo "DIFF-AWARE FAIL: opencode.json changed but harness drift check failed"
    ERRORS=$((ERRORS + 1))
  fi
fi

# ── Rule: factory-hooks.ts changed → flag parity re-eval required ──────
if echo "$CHANGED_FILES" | grep -q '^\.opencode/plugin/factory-hooks\.ts$'; then
  echo "diff-aware-check: factory-hooks.ts changed — parity re-eval REQUIRED"
  CHECKS_RAN=$((CHECKS_RAN + 1))
  if ! (cd "$REPO_ROOT" && ./scripts/hooks/shared-script-enforcement.sh) 2>&1; then
    echo "DIFF-AWARE FAIL: factory-hooks.ts changed but shared-script-enforcement failed"
    ERRORS=$((ERRORS + 1))
  fi
  echo "  WARNING: live parity (OBSERVED) re-verification required — cannot run in CI"
  echo "  The previous OBSERVED pass is now stale. Re-run manual parity eval before"
  echo "  claiming OBSERVED on factory-hooks.ts behavior."
  echo "factory-hooks.ts modified $(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    > "$REPO_ROOT/memory/.parity-stale" 2>/dev/null || true
fi

# ── Rule: adapter generator changed → run sync + drift check ─────────
if echo "$CHANGED_FILES" | grep -qE '^scripts/sync-(claude|codex)\.sh$'; then
  echo "diff-aware-check: adapter generator changed — running sync + drift check"
  CHECKS_RAN=$((CHECKS_RAN + 1))
  if ! (cd "$REPO_ROOT" && make check-drift) 2>&1; then
    echo "DIFF-AWARE FAIL: adapter generator changed but drift check failed"
    ERRORS=$((ERRORS + 1))
  fi
fi

# ── Rule: commit-message-lint.sh changed → lint full unpushed range ────
if echo "$CHANGED_FILES" | grep -q '^scripts/hooks/commit-message-lint\.sh$'; then
  echo "diff-aware-check: commit-message-lint.sh changed — linting full unpushed range"
  CHECKS_RAN=$((CHECKS_RAN + 1))
  ORIGIN_HEAD=$(git rev-parse origin/main 2>/dev/null || echo "")
  if [ -n "$ORIGIN_HEAD" ]; then
    RANGE_FAIL=0
    for sha in $(git rev-list "$ORIGIN_HEAD..HEAD" 2>/dev/null || true); do
      if ! (cd "$REPO_ROOT" && ./scripts/hooks/commit-message-lint.sh "$sha") 2>&1; then
        RANGE_FAIL=1
      fi
    done
    if [ "$RANGE_FAIL" -ne 0 ]; then
      echo "DIFF-AWARE FAIL: commit-message-lint.sh changed and unpushed range has violations"
      ERRORS=$((ERRORS + 1))
    fi
  fi
fi

# ── Rule: .opencode/agent/*.md changed → verify frontmatter matches opencode.json ──
if echo "$CHANGED_FILES" | grep -q '^\.opencode/agent/.*\.md$'; then
  echo "diff-aware-check: agent .md changed — run sync + drift check"
  CHECKS_RAN=$((CHECKS_RAN + 1))
  if ! (cd "$REPO_ROOT" && make check-drift) 2>&1; then
    echo "DIFF-AWARE FAIL: agent .md changed but sync or drift check failed"
    ERRORS=$((ERRORS + 1))
  fi
fi

# ── Rule: generated adapter edited → regenerate and detect drift ──────
if echo "$CHANGED_FILES" | grep -qE '^\.(claude|codex)/'; then
  echo "diff-aware-check: generated harness adapter changed — running drift check"
  CHECKS_RAN=$((CHECKS_RAN + 1))
  if ! (cd "$REPO_ROOT" && make check-drift) 2>&1; then
    echo "DIFF-AWARE FAIL: generated adapter does not match canonical sources"
    ERRORS=$((ERRORS + 1))
  fi
fi

# ── Rule: a protected path changed → run the configured check command ──
# Protected paths and the check command come from factory.yaml (Decision 2).
PROTECTED_CHANGED=""
for PROTECTED in $(factory_config_get protected_paths); do
  if echo "$CHANGED_FILES" | grep -q "^$PROTECTED"; then
    PROTECTED_CHANGED="$PROTECTED_CHANGED $PROTECTED"
  fi
done
if [ -n "$PROTECTED_CHANGED" ]; then
  CHECK_COMMAND="$(factory_config_get check_command)"
  echo "diff-aware-check: protected path(s) changed:$PROTECTED_CHANGED"
  if [ -n "$CHECK_COMMAND" ]; then
    CHECKS_RAN=$((CHECKS_RAN + 1))
    if ! (cd "$REPO_ROOT" && eval "$CHECK_COMMAND") 2>&1; then
      echo "DIFF-AWARE FAIL: protected path changed but check_command failed"
      ERRORS=$((ERRORS + 1))
    fi
  else
    echo "diff-aware-check: no check_command configured — protected change noted only"
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────
if [ "$CHECKS_RAN" -eq 0 ]; then
  echo "diff-aware-check: no diff-aware rules triggered for this changeset"
  exit 0
fi

if [ "$ERRORS" -gt 0 ]; then
  echo "diff-aware-check: $ERRORS check(s) failed out of $CHECKS_RAN dispatched"
  exit 1
fi

echo "diff-aware-check: all $CHECKS_RAN dispatched check(s) passed"
exit 0
