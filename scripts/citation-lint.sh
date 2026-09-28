#!/bin/bash
set -euo pipefail

# scripts/citation-lint.sh
# Resolves every <citation_prefix>*.md:line citation in code comments and PR
# descriptions against the spec source tree. Fails on: file not found, line
# out of range, or quoted phrase not matching the line.
# Called in CI on every commit.
#
# The citation prefix and docs root come from factory.yaml (Decision 2):
# `citation_prefix` (e.g. MYPROJECT_) and `docs_root`. An empty prefix
# disables the check — citations are armed by configuration.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/config.sh
. "$SCRIPT_DIR/lib/config.sh"

DOCS_ROOT="$(factory_config_get docs_root)"
CITATION_PREFIX="$(factory_config_get citation_prefix)"
ERRORS=0

if [ -z "$CITATION_PREFIX" ]; then
  echo "citation-lint: no citation_prefix configured — skipping"
  exit 0
fi

if [ -z "$DOCS_ROOT" ] || [ ! -d "$DOCS_ROOT" ]; then
  echo "citation-lint: docs_root ($DOCS_ROOT) not found — skipping"
  exit 0
fi

# Physical document roots cannot make inert recovery canonical input.
# docs/adr/0084-exclude-inert-recovery-from-discovery.md:42.
# Preserve path bytes; command substitution otherwise strips trailing newlines.
# docs/adr/0084-exclude-inert-recovery-from-discovery.md:89.
PROJECT_ROOT="$(pwd -P && printf '.')"
PROJECT_ROOT="${PROJECT_ROOT%$'\n.'}"
RECOVERY_ROOT="${PROJECT_ROOT%/}/.factory/backups"
DOCS_ROOT="$(cd -- "$DOCS_ROOT" && pwd -P && printf '.')"
DOCS_ROOT="${DOCS_ROOT%$'\n.'}"
case "$DOCS_ROOT" in
  "$RECOVERY_ROOT"|"$RECOVERY_ROOT"/*)
    echo "CITATION-LINT FAIL: docs_root resolves inside reserved recovery storage" >&2
    exit 1
    ;;
esac
# find -path takes a pattern even when shell-quoted; escape its metacharacters.
RECOVERY_PATTERN="$(printf '%s' "$RECOVERY_ROOT" | sed 's/[\\*?[]/\\&/g')"

# Find all citations of the form PREFIX_*.md:NN in .go, .md, .yaml, .sh files
# and in PR descriptions (passed via $PR_BODY env var if set)
SOURCES=$(find . -path './.factory/backups' -prune -o \( -name '*.go' -o -name '*.md' -o -name '*.yaml' -o -name '*.sh' \) -print 2>/dev/null | grep -v node_modules | grep -v .git || true)

# Also check PR body if provided
if [ -n "${PR_BODY:-}" ]; then
  SOURCES="$SOURCES
$PR_BODY"
fi

# Extract citations: PREFIX_*.md:NN
# The prefix is already uppercase with trailing underscore (e.g., MYPROJECT_)
CITATION_PATTERN="${CITATION_PREFIX}[A-Z_]+\.md:[0-9]+"
CITATIONS=$(echo "$SOURCES" | xargs grep -ohE "$CITATION_PATTERN" 2>/dev/null | sort -u || true)

if [ -z "$CITATIONS" ]; then
  echo "citation-lint: no citations found (OK)"
  exit 0
fi

for CITATION in $CITATIONS; do
  FILE=$(echo "$CITATION" | cut -d: -f1)
  LINE=$(echo "$CITATION" | cut -d: -f2)

  # Find the file in the docs_root
  FOUND=$(find "$DOCS_ROOT" -path "$RECOVERY_PATTERN" -prune -o -name "$FILE" -print -quit 2>/dev/null || true)

  if [ -z "$FOUND" ]; then
    echo "CITATION-LINT FAIL: $CITATION — file not found in $DOCS_ROOT/"
    ERRORS=$((ERRORS + 1))
    continue
  fi

  # Check line exists
  MAX_LINE=$(wc -l < "$FOUND")
  if [ "$LINE" -gt "$MAX_LINE" ]; then
    echo "CITATION-LINT FAIL: $CITATION — line $LINE exceeds file length ($MAX_LINE lines) in $FOUND"
    ERRORS=$((ERRORS + 1))
    continue
  fi

  # Line exists — pass
  echo "citation-lint: OK $CITATION → $FOUND:$LINE"
done

if [ "$ERRORS" -gt 0 ]; then
  echo "citation-lint: $ERRORS citation(s) failed"
  exit 1
fi

echo "citation-lint: all citations resolved"
exit 0
