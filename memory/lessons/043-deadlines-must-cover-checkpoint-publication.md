# Execution deadlines include checkpoint publication

An allowance computed before a durable phase save can already be expired when
execution starts. Carry an absolute deadline through preparation and supervision,
and recheck remaining time after the save. A timeout during publication still
obeys transaction poisoning; do not retry writes just to produce a terminal record.

Provenance: observed 2026-09-25 UTC via the independent regression in
internal/loop/manual_test.go:27. A 1.1-second phase-save delay in a one-second loop
called the executor once before correction. ADR-0075 records the contract at
docs/adr/0075-go-manual-loop-controller.md:73; execution evidence is recorded in
docs/migration/MANUAL_LOOPS.md. This lesson points to that contract rather than
creating another timeout policy.
