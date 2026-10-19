# Reserve markers without stealing operands

A literal token that resembles a new control flag can already be another option's
value. Keep explicit selected parsers in charge of their operands, and share the
existing legacy source/ref marker rule instead of inventing another grammar.
Reserve actual malformed controls before legacy dispatch without reinterpreting
valid detached values. This complements
[the preview-intent lesson](053-reserve-preview-intent-before-validation.md).
The contract is `docs/adr/0106-go-backup-retention.md:133` and shared implementation
is `internal/commandargs/upgrade.go:8`.

Provenance: observed 2026-10-19 while verifying PR #132's third advisory review.
Actual valid local-image previews accepted `--source=--keep-backup` but rejected
the equivalent detached spelling through the backup parser; `--prune-backup`
behaved the same way. The independent permanent criteria in
`acceptance/installation_upgrade_test.go:876` and
`acceptance/recovery_restoration_plan_test.go:227` observed six failures before the
correction, then the focused race run observed `27 Passed | 0 Failed`, including
both supported baselines, exact legacy forwarding and all malformed controls.
Read-only snapshots stayed unchanged; no authenticated apply or live model was
part of this proof. The command and full qualification scope are recorded in
`docs/migration/BACKUP_RETENTION.md:141`.
