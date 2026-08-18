#!/bin/bash
set -euo pipefail

# Enforce ADR-0002's single Go testing dialect. The standard testing package
# is allowed only for the one RunSpecs bootstrap required by go test.

# Resolved before the cd below, which moves us out of the script's own tree.
# At install time this gate lives in scripts/hooks/, so ../lib/events.sh
# resolves; in the pack directory it does not, and the no-op keeps the gate
# working. Bookkeeping must never be why enforcement fails.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../../scripts/lib/events.sh
if [ -f "$SCRIPT_DIR/../lib/events.sh" ]; then . "$SCRIPT_DIR/../lib/events.sh"; else factory_log_event() { :; }; fi

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# File list comes from git, not ripgrep. The previous version used
# `rg --files -g '*_test.go'` inside a process substitution, which fails OPEN:
# `set -e` does not observe a failure there, so on any machine without ripgrep
# the loop iterated zero times and the gate printed success while checking
# nothing. git is already a hard requirement for every other gate, and
# `git ls-files` additionally scopes the check to files actually in the repo.
#
# All matching below uses `grep -E` (POSIX ERE) for the same reason: no gate
# should depend on a tool that may be absent. Word boundaries are spelled out
# rather than using GNU's `\b`, which BSD/macOS grep does not reliably support.
if ! FILES="$(git ls-files -- '*_test.go')"; then
  echo "GINKGO-ONLY FAIL: cannot list test files (not a git checkout?)" >&2
  factory_log_event "ginkgo-only-check" "could not enumerate test files"
  exit 1
fi

ERRORS=0

while IFS= read -r FILE; do
  [ -n "$FILE" ] || continue
  [ -f "$FILE" ] || continue

  if ! grep -qE '"testing"' "$FILE"; then
    continue
  fi

  if ! grep -qE 'RunSpecs[[:space:]]*\([[:space:]]*t[[:space:]]*,' "$FILE"; then
    echo "GINKGO-ONLY FAIL: $FILE imports testing without a RunSpecs bootstrap"
    ERRORS=$((ERRORS + 1))
    continue
  fi

  # grep -c exits 1 on zero matches, so default it rather than letting set -e fire.
  TEST_FUNCTIONS="$(grep -cE '^[[:space:]]*func Test[^[:space:]]*[[:space:]]*\(' "$FILE" || true)"
  TEST_FUNCTIONS="${TEST_FUNCTIONS:-0}"
  TESTING_T_REFS="$(grep -oE 'testing\.T' "$FILE" | wc -l | tr -d '[:space:]')"
  TESTING_T_REFS="${TESTING_T_REFS:-0}"
  if [ "$TEST_FUNCTIONS" -ne 1 ] || [ "$TESTING_T_REFS" -ne 1 ]; then
    echo "GINKGO-ONLY FAIL: $FILE must contain exactly one testing.T RunSpecs bootstrap"
    ERRORS=$((ERRORS + 1))
  fi

  # Leading boundary excludes '.' too, so a.t.Run does not match; the trailing
  # '(' keeps this to actual calls.
  if grep -qE '(^|[^[:alnum:]_.])t\.(Run|Fatal|Fatalf|Error|Errorf|Fail|FailNow|Helper|Log|Logf|Parallel|Skip|Skipf|SkipNow|TempDir|Setenv|Cleanup)[[:space:]]*\(' "$FILE"; then
    echo "GINKGO-ONLY FAIL: $FILE calls testing.T outside Ginkgo/Gomega"
    ERRORS=$((ERRORS + 1))
  fi
done <<< "$FILES"

if [ "$ERRORS" -gt 0 ]; then
  echo "ginkgo-only-check: $ERRORS violation(s) found"
  factory_log_event "ginkgo-only-check" "$ERRORS non-Ginkgo test construct(s)"
  exit 1
fi

echo "ginkgo-only-check: all Go behavioral tests use Ginkgo/Gomega"
