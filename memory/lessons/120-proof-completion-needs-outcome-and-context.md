# A confirmed exit code is only one part of proof completion

Observed 2026-10-31 UTC through public source selftest regression controls:
the real final healthy child completed with confirmed exit zero; a test-only
native-boundary overlay then canceled the actual CLI context and returned an
interrupted outcome. The controller still printed success and exited zero.
Other controls returned timeout/output-limit outcomes with a live parent
context. Those classifications were injected; the completed child and context
observations were real. A separate full-pipe control reproduced blocked output
after cancellation at the publication boundary.

The canonical response is `docs/adr/0120-native-source-gate-proof.md:115` and
`docs/adr/0120-native-source-gate-proof.md:123`: preserve native ownership errors,
validate completion and context, and use the existing cancellable output helper.
Expected violation exits legitimately have a `failed` native outcome; rejecting
all non-`completed` results would invalidate the proof itself.

Independent RED reports preceded the repair. The final race-enabled, two-worker
source/CLI/installed/census run passed 52 criteria in 390.434 suite seconds.
The full-pipe test controls cancellation immediately before publication; it does
not establish that a write had already blocked when the signal arrived.
