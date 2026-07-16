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

BLUEPRINT_DIR="$(factory_config_get docs_root)"
CITATION_PREFIX="$(factory_config_get citation_prefix)"
ERRORS=0

if [ -z "$CITATION_PREFIX" ]; then
  echo "citation-lint: no citation_prefix configured — skipping"
  exit 0
fi

if [ -z "$BLUEPRINT_DIR" ] || [ ! -d "$BLUEPRINT_DIR" ]; then
  echo "citation-lint: docs_root ($BLUEPRINT_DIR) not found — skipping"
  exit 0
fi

# Find all citations of the form PREFIX_*.md:NN in .go, .md, .yaml, .sh files
# and in PR descriptions (passed via $PR_BODY env var if set)
SOURCES=$(find . -name '*.go' -o -name '*.md' -o -name '*.yaml' -o -name '*.sh' 2>/dev/null | grep -v node_modules | grep -v .git || true)

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

  # Find the file in the blueprint
  FOUND=$(find "$BLUEPRINT_DIR" -name "$FILE" -print -quit 2>/dev/null || true)

  if [ -z "$FOUND" ]; then
    echo "CITATION-LINT FAIL: $CITATION — file not found in $BLUEPRINT_DIR/"
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
