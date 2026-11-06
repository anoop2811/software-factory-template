# Separate startup readiness from shutdown evidence

A readiness timeout before any signal says nothing about cancellation behavior.
Keep startup within the fixture's total budget while reserving its original
post-signal completion window. A deliberately slow startup is a useful witness: it
must still reach the real handshake, then pass the unchanged shutdown assertions.

When cleanup fails, preserve its typed cause across error wrapping. Report only
bounded native ownership facts; arbitrary child stderr, tokens and error text
must remain private. A passing retry without those facts does not identify the
original failure's cause.

An early readiness/parsing failure must not bypass cleanup merely because the
test body has not stored the child identity yet. Recover validated ownership,
attempt parent termination/join before reporting errors, and retain uncertain
fixtures. The early-return regression first left a real child alive, then passed
with parent status 130 and child/group absence after the cleanup correction;
see `docs/adr/0139-go-source-budget-entrypoint.md:149`.

Provenance: observed 2026-11-06 via the controlled four-second startup RED and
the nine-criterion race GREEN in PR #170 qualification. Delayed INT readiness took
5.466 seconds, followed by status 130, compiler absence and unchanged source
within the original two-second signal window. Canonical boundaries are
`docs/adr/0139-go-source-budget-entrypoint.md:123` and
`docs/adr/0139-go-source-budget-entrypoint.md:136`.
