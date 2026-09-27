# Rendered metadata is not complete safety evidence

A display format can intentionally omit facts needed by a later safety decision.
The installation assessor renders ordinary permission bits, while the target
planner must also reject setuid, setgid and sticky modes. Comparing only the
rendered mode, digest and size would lose that distinction.

Preserve complete evidence internally and apply the stricter check before
candidate equality or action selection. Keep the existing report contract stable
instead of making downstream safety depend on a lossy presentation value.

Provenance: internal/assessment/assessment.go:143 renders only permission bits;
docs/adr/0080-go-migration-action-planning.md:44 requires complete internal mode
evidence for target-action planning.
