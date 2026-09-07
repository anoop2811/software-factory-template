# Shell parsing includes process semantics

A shell parser port must characterize the whole pipeline, including locale,
command substitution and exit-status propagation. Matching the visible regular
expression alone can change values: key selection precedes NUL removal, and
get can return a fallback successfully even when the underlying reader reports
an error. Locale-sensitive whitespace needs platform evidence before cutover.

Keep these boundaries in the migration decision and compare immutable shell
baselines with independent expected values at the compiled process boundary.
This lesson points to that contract; it does not approve behavior differences.

Provenance: scripts/lib/config.sh:36 contains the command substitution and
sed/head pipeline; scripts/lib/config.sh:51 returns the fallback. Decision 51
records the candidate scope and unresolved boundaries in docs/DECISION_LOG.md:1996.
The independent cases are in acceptance/readers_test.go (read-failure and NUL
selection scenarios), added during the 2026-09-06 conversion iteration.
