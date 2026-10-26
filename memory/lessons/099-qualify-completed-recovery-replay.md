# Test recovery again after its pending marker is gone

One successful recovery does not establish idempotence. A fresh process must
preview and confirm recovery again after successful cleanup removed the pending
marker. Retain the journal's historical ownership receipt; distinguish that
completed no-op from cleanup of a still-present marker. Requalify actual assets,
sources, state, direction and fresh consent before granting either operation.

Canonical correction: docs/DECISION_LOG.md:4829. The completed replay boundary is
internal/installation/completed_recovery.go:12. Do not generalize marker absence
into permission to resume a partial transaction.

Provenance: observed 2026-10-26 UTC in published RC2 run 40252592120. Its Linux
amd64 and macOS reports record successful first interrupted recovery followed by
`terminal pending observation differs from current marker` on a second preview.
The exact tagged artifact, OS, Ginkgo outcomes and stopped journals remain in the
[release run](https://github.com/anoop2811/software-factory-template/actions/runs/40252592120).
