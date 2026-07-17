#!/bin/bash
set -euo pipefail

# Enforce the TypeScript pack's single test dialect: Vitest. Imports of other
# test frameworks or assertion libraries (Jest, Mocha, Chai, Jasmine, Ava,
# node:test) are rejected so every behavioral test shares one API — the TS
# analog of the Go pack's ginkgo-only-check and the Java pack's
# junit5-only-check. Importing from 'vitest' is the intended path.
#
# Scans the directory given as $1, or the current directory. Dependencies and
# build output are pruned.

ROOT="${1:-.}"
cd "$ROOT"

ERRORS=0

# Forbidden module specifiers in a `from '...'` import or a require('...').
FORBIDDEN="jest|@jest/globals|mocha|chai|jasmine|ava|node:test"

while IFS= read -r FILE; do
  [ -n "$FILE" ] || continue

  if grep -Eq "(from[[:space:]]+|require\()[[:space:]]*['\"](${FORBIDDEN})['\"]" "$FILE" 2>/dev/null; then
    echo "VITEST-ONLY FAIL: $FILE imports a non-Vitest test framework — behavioral tests must use Vitest"
    ERRORS=$((ERRORS + 1))
  fi
done < <(find . \( -path '*/node_modules/*' -o -path '*/dist/*' -o -path '*/build/*' -o -path '*/.git/*' \) -prune -o \
           -type f \( -name '*.test.ts' -o -name '*.test.tsx' -o -name '*.test.js' -o -name '*.test.jsx' \
                      -o -name '*.spec.ts' -o -name '*.spec.tsx' -o -name '*.spec.js' -o -name '*.spec.jsx' \) -print)

if [ "$ERRORS" -gt 0 ]; then
  echo "vitest-only-check: $ERRORS violation(s) found"
  exit 1
fi

echo "vitest-only-check: all TypeScript behavioral tests use Vitest"
