# Budget publication is not always rollback-safe

A failed write operation does not necessarily mean the ledger stayed unchanged.
When rename succeeds and directory sync fails, a reservation may already be
visible. Retrying admission with a new identity can spend another attempt;
launching after the error can lose durable ownership. Preserve the operation's
identity and report possible publication so the caller stops for inspection.

Provenance: `docs/adr/0069-go-budget-ledger-admission.md:86` defines publication
ordering and ambiguity, and `docs/adr/0069-go-budget-ledger-admission.md:174`
defines the error contract. The immutable legacy sequence is in
`scripts/lib/budget.py`, Ledger.write, at commit
`4cc771e894e11d5024032106aeb0ef788997bd29`. Qualification and recovery remain in
the ADR; this lesson does not establish installed-controller behavior.
