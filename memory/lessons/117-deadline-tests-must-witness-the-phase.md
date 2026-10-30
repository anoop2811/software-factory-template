# Deadline tests must witness the intended phase

Observed 2026-10-30 UTC: PR #149 macOS base job `124287845404` failed
`Bounded loop lifecycle boundaries` with zero paid calls and a nil error.
The fixture used a 200ms allowance and delayed its second file sync. That ordinal
did not identify which phase was being published.

A private test-only overlay exhausted the actual loop context in the preceding
freshness observation. The original criterion failed in 0.664 seconds with the
same nil-error assertion; its receipt showed `starting`, then `finished`, two
saves and zero paid calls. This demonstrates a legitimate early handoff can
reach terminal cleanup instead of the intended implementer publication.

The corrected criterion observes the serialized phase and actual execution
context. It waits for that context's deadline at the implementer sync barrier,
then requires an uncommitted publication error, the durable starting checkpoint
and no paid accounting. Removing the post-sync deadline guard in a private
overlay failed the unchanged `MayHaveCommitted=false` assertion; canonical and
delayed-freshness controls passed.

Provenance: `internal/loop/bounded_test.go:88`,
`docs/DECISION_LOG.md:5317` (Decision 130), and
`docs/adr/0076-go-bounded-loop-controller.md:71`. The session's retained
`loop-deadline/old-prephase-red.json`, `phase-green.json`,
`delayed-freshness-green.json` and `missing-publication-guard-red.json` record
the executed controls. Runtime loop and budget limits were not changed.
