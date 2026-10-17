# Diff counts do not prove completeness

Observed 2026-10-17 with a compiled CLI and fake TLS provider: a truncated
mode-only diff matched zero additions/deletions and falsely received complete
coverage. Validate required Git metadata pairs as well as hunk counts.

Provenance: docs/adr/0100-budgeted-batched-adversarial-review.md:29;
`acceptance/review_batch_test.go` real-Git regression controls.
