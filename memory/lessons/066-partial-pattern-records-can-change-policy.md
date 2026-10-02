# Partial pattern records can change policy before completion

Observed 2026-10-02 via an isolated Git `check-ignore -q` probe: appending only
`/.factory` from the intended `/.factory/backups/` rule changed
`.factory/events.log` from not ignored (exit 1) to ignored (exit 0). Git parses
patterns at EOF; a missing newline does not make an incomplete append inert.

The recovery writer's independent short-write regression reproduced this before
correction. Stage a comment, sync/check the complete bytes, then activate the
complete narrow rule. Never depend on successful rollback after an I/O error.
The canonical contract is
[ADR-0091](../../docs/adr/0091-durable-local-recovery-creation.md), at
`docs/adr/0091-durable-local-recovery-creation.md:159`;
this lesson points to it rather than defining another publication protocol.
