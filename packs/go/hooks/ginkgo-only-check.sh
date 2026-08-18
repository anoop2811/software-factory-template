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
  # awk's gsub, not `grep -o`: -o is a GNU extension, absent from POSIX grep and
  # from the busybox grep on minimal CI images — where this counted nothing and the
  # gate passed a file with several testing.T references. gsub returns the number
  # of substitutions, so summing it counts every occurrence, including two on one
  # line, which `grep -c` would have counted once.
  TESTING_T_REFS="$(awk '{ n += gsub(/testing\.T/, "&") } END { print n + 0 }' "$FILE")"
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

  # Package-level testing calls slipped through, because the check above only
  # looks at methods on `t`. After a valid RunSpecs bootstrap a file could still
  # branch on testing.Short(), which is stdlib test control living outside the
  # Ginkgo dialect this gate exists to keep singular.
  #
  # A call needs the '(' immediately after the identifier, so the bootstrap's own
  # `*testing.T` and `*testing.M` type references do not match.
  # One awk pass finds the call and names it. The previous form scanned twice and
  # piped `grep -o` into `head -n 1`: head exits after the first line, grep dies of
  # SIGPIPE, and under `set -o pipefail` that non-zero status propagates — so the
  # gate could abort at the exact moment it found a violation, before printing it.
  OFFENDER="$(awk '
    match($0, /(^|[^[:alnum:]_.])testing\.[A-Za-z][A-Za-z0-9_]*[[:space:]]*\(/) {
      s = substr($0, RSTART, RLENGTH)
      sub(/^[^t]*/, "", s)
      sub(/[[:space:]]*\($/, "", s)
      print s
      exit
    }' "$FILE")"
  if [ -n "$OFFENDER" ]; then
    echo "GINKGO-ONLY FAIL: $FILE calls $OFFENDER — package-level testing calls belong outside the Ginkgo dialect"
    ERRORS=$((ERRORS + 1))
  fi
done <<< "$FILES"

if [ "$ERRORS" -gt 0 ]; then
  echo "ginkgo-only-check: $ERRORS violation(s) found"
  factory_log_event "ginkgo-only-check" "$ERRORS non-Ginkgo test construct(s)"
  exit 1
fi

echo "ginkgo-only-check: all Go behavioral tests use Ginkgo/Gomega"
