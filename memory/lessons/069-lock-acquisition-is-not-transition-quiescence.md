# Lock acquisition is not transition quiescence

Observed 2026-10-03 while reviewing the recovery restoration boundary after
merged PR #118. Recovery creation uses its own lock; budget preflight can start
before ledger admission. Acquiring currently available locks or reading empty
history therefore does not prove that an older process paused before admission
is absent. Preserve this distinction when assessing active runtime changes.

Provenance: scripts/lib/budget.py:484 (preflight), scripts/lib/budget.py:493
(ledger admission lock), internal/budget/runner.go:83 (Go preflight),
internal/budget/runner.go:90 (admission), and
internal/assessment/recovery_git.go:99 (separate recovery lock). The required
transition contract remains canonical in specs/001-go-runtime-conversion.md:313;
Decision 91 and docs/adr/0092-go-recovery-restoration-planning.md:15 explain why
the next operation is read-only planning. This lesson grants no new exception.
