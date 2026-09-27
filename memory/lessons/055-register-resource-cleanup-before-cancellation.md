# Register resource cleanup before checking cancellation

Observed 2026-09-27 via the independent recovery-inspection Ginkgo cases for
cancellation after opening `.factory`, `backups` and a recovery-set directory:
all three returned cancellation while leaving the newly acquired descriptor open.
The descriptor had not yet entered the list owned by deferred cleanup.

A cancellation check is an early-return boundary. After acquiring a resource,
transfer it to cleanup ownership before any such return, or close it on that
path. Test cancellation immediately after acquisition, not only during a later
read. This applies even when a helper successfully returns a resource alongside
an already-canceled context.

Provenance: `FACTORY_AGENT_ROLE=spec-writer go test ./internal/assessment
-ginkgo.focus="Recovery inventory controlled.*propagates cancellation" -ginkgo.no-color -count=1 -v` produced
`3 Passed | 3 Failed` before correction, package `0.503s`, with
`descriptor left open: assessment-entry`. The retained contract is
[ADR-0083](../../docs/adr/0083-go-recovery-set-inspection.md); the executable
regressions are in `internal/assessment/recovery_test.go`. Qualification evidence
belongs to the [recovery inspection guide](../../docs/migration/RECOVERY_INSPECTION.md).
