# Stream parity includes decoder semantics

Whole-document-first parsing is observable: a whole array is malformed accounting,
while separate JSON lines are independently interpreted. Python splitlines has
more boundaries than LF, and its JSON decoder retains non-finite values and lone
surrogates. A default Go JSON decode can lose identity or accounting information.
Keep framing and numeric/string semantics in independent oracle comparisons.

Provenance: immutable `scripts/lib/budget.py` parse_events at
`a0216779e4f94057f3e6752ce0ef178499721af4`, observed 2026-09-08 UTC via isolated
AST-extracted Python function probes; canonical requirements are in ADR-0066.
