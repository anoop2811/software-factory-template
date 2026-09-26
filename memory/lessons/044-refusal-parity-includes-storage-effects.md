# Refusal parity includes storage effects

Matching exit status and preventing a model call do not prove refusal parity.
Acquiring a persistent lock may create a directory and lock file before invalid
input is rejected. Characterize filesystem effects alongside output and status.
Validate input before lock acquisition where the legacy boundary requires it,
then recheck its identity under the lock before execution.

Provenance: observed 2026-09-25 UTC via the independently authored
`G2 bounded loop prompt admission ordering` compiled-CLI differential regression.
Python refused a missing admitted prompt without `.factory`; the candidate created
it. The regression failed before correction and passed in the final 52-case
instrumented run. Evidence: docs/migration/BOUNDED_LOOPS.md; canonical requirement:
docs/adr/0076-go-bounded-loop-controller.md:140.
