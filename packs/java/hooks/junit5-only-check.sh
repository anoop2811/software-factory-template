#!/bin/bash
set -euo pipefail

# Enforce the Java pack's single test dialect: JUnit 5 (Jupiter). JUnit 4 and
# JUnit 3 (org.junit.*, junit.framework.*) imports are rejected so every
# behavioral test shares one API — the Java analog of the Go pack's
# ginkgo-only-check. org.junit.jupiter.* is JUnit 5 and is allowed.
#
# Scans the directory given as $1, or the current directory. Build output and
# VCS metadata are pruned.

ROOT="${1:-.}"
cd "$ROOT"

ERRORS=0

while IFS= read -r FILE; do
  [ -n "$FILE" ] || continue

  # JUnit 4/3 imports (including static imports of org.junit.Assert). The
  # alternation deliberately does not match org.junit.jupiter.* (JUnit 5).
  if grep -Eq '^[[:space:]]*import[[:space:]]+(static[[:space:]]+)?(org\.junit\.(runner|runners|rules|experimental)|org\.junit\.(Test|Before|After|BeforeClass|AfterClass|Ignore|Rule|Assert|Assume|ClassRule)|junit\.framework)' "$FILE" 2>/dev/null; then
    echo "JUNIT5-ONLY FAIL: $FILE imports a JUnit 4/3 API — behavioral tests must use JUnit 5 (org.junit.jupiter)"
    ERRORS=$((ERRORS + 1))
  fi
done < <(find . \( -path '*/build/*' -o -path '*/target/*' -o -path '*/.git/*' \) -prune -o \
           -type f \( -name '*Test.java' -o -name '*Tests.java' -o -name '*IT.java' -o -name '*ITCase.java' \) -print)

if [ "$ERRORS" -gt 0 ]; then
  echo "junit5-only-check: $ERRORS violation(s) found"
  exit 1
fi

echo "junit5-only-check: all Java behavioral tests use JUnit 5 (Jupiter)"
