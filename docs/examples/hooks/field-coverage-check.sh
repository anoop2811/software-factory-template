#!/bin/bash
set -euo pipefail

# docs/examples/hooks/field-coverage-check.sh
# EXAMPLE domain-invariant hook — copy into scripts/hooks/ and adapt.
#
# The pattern: some structs have a companion function that derives something
# from their fields (a serialization, a signature, an index). Adding a field
# to the struct but not to the function is a silent, dangerous omission. This
# hook reads both and asserts coverage, so the omission fails CI instead of
# shipping.
#
# The example below is written against an invented internal/audit/record.go:
#   type AuditRecord struct { ... }        // the struct
#   func CoveredFields(r AuditRecord) ...  // must name every field
# Excluded fields (deliberately not covered) are listed in EXCLUDED_FIELDS.

RECORD_FILE="internal/audit/record.go"
STRUCT_NAME="AuditRecord"
COVERAGE_FUNC="CoveredFields"
EXCLUDED_FIELDS="Signature"

if [ ! -f "$RECORD_FILE" ]; then
  echo "field-coverage-check: $RECORD_FILE not found — skipping (example hook; adapt before use)"
  exit 0
fi

STRUCT_FIELDS=$(sed -n "/^type $STRUCT_NAME struct/,/^}/p" "$RECORD_FILE" | \
  grep -oE '^\s+[A-Z]\w*' | tr -d ' \t' | sort -u)

ERRORS=0
for FIELD in $STRUCT_FIELDS; do
  if echo "$EXCLUDED_FIELDS" | grep -qw "$FIELD"; then
    continue
  fi
  if ! sed -n "/^func $COVERAGE_FUNC/,/^}/p" "$RECORD_FILE" | grep -q "$FIELD"; then
    echo "FIELD-COVERAGE FAIL: $FIELD is in $STRUCT_NAME but not in $COVERAGE_FUNC"
    ERRORS=$((ERRORS + 1))
  fi
done

if [ "$ERRORS" -gt 0 ]; then
  echo "field-coverage-check: $ERRORS field(s) missing from $COVERAGE_FUNC"
  exit 1
fi
echo "field-coverage-check: all non-excluded fields covered"
