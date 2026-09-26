# Check cancellation during reads, not only around them

Where a helper promises context-error identity, an already-canceled-context test
cannot prove the read-error path preserves it. Generic validation wrapping may
hide cancellation returned by a bounded reader even when entry and success-path
context checks are correct. Exercise the failure after reading has begun and
assert that no partial value escapes.

Provenance: observed 2026-09-26 via PR #103's two deterministic mid-read regression
failures and the subsequent 34-case bounded internal race run. Canonical contract:
docs/adr/0076-go-bounded-loop-controller.md:154. Evidence:
docs/migration/BOUNDED_LOOPS.md, prompt cancellation review follow-up.
