#!/bin/bash
set -euo pipefail

# Enforce ADR-0002's single Go testing dialect. The standard testing package
# is allowed only for the one RunSpecs bootstrap required by go test.

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

ERRORS=0

while IFS= read -r FILE; do
  [ -n "$FILE" ] || continue

  if ! rg -q '"testing"' "$FILE"; then
    continue
  fi

  if ! rg -q 'RunSpecs[[:space:]]*\([[:space:]]*t[[:space:]]*,' "$FILE"; then
    echo "GINKGO-ONLY FAIL: $FILE imports testing without a RunSpecs bootstrap"
    ERRORS=$((ERRORS + 1))
    continue
  fi

  TEST_FUNCTIONS="$(rg -c '^[[:space:]]*func Test[^[:space:]]*[[:space:]]*\(' "$FILE" || true)"
  TESTING_T_REFS="$(rg -o 'testing\.T' "$FILE" | /usr/bin/wc -l | /usr/bin/tr -d ' ')"
  if [ "$TEST_FUNCTIONS" -ne 1 ] || [ "$TESTING_T_REFS" -ne 1 ]; then
    echo "GINKGO-ONLY FAIL: $FILE must contain exactly one testing.T RunSpecs bootstrap"
    ERRORS=$((ERRORS + 1))
  fi

  if rg -q '\bt\.(Run|Fatal|Fatalf|Error|Errorf|Fail|FailNow|Helper|Log|Logf|Parallel|Skip|Skipf|SkipNow|TempDir|Setenv|Cleanup)\b' "$FILE"; then
    echo "GINKGO-ONLY FAIL: $FILE calls testing.T outside Ginkgo/Gomega"
    ERRORS=$((ERRORS + 1))
  fi
done < <(rg --files -g '*_test.go')

if [ "$ERRORS" -gt 0 ]; then
  echo "ginkgo-only-check: $ERRORS violation(s) found"
  exit 1
fi

echo "ginkgo-only-check: all Go behavioral tests use Ginkgo/Gomega"
