# Self-stop return is not continuation evidence

A multithreaded Go supervisor can execute after requesting its own SIGSTOP before
the whole process has stopped. Returning from the signal syscall does not prove
that a shell subsequently resumed it. Terminal handoff at that point can leave
the child in the background and trigger SIGTTIN on its next input read.

Use an actual continuation notification to order terminal handoff, retaining
cancellation and cleanup paths. Keep real terminal input and repeated-stop
controls in the regression criterion.

Provenance: `docs/adr/0098-whole-installation-upgrade-and-rollback.md:615`;
Linux trace and the 18-pass native race run recorded in Decision 97 of
`docs/DECISION_LOG.md`. This summarizes the decision; the ADR remains canonical.

The same distinction applies to an external SIGSTOP request in an interruption
fixture: observe native stopped state before reading the durable crash witness.
Keep one exit/reaping owner. Healthy completion before the stop is an unwitnessed
attempt, never evidence of crash recovery. The correction contract remains in
Decision 110, docs/DECISION_LOG.md:4695.

Additional provenance: observed 2026-10-24 in PR #136 Linux installation job
https://github.com/anoop2811/software-factory-template/actions/runs/40105875345/job/120907435287.
A test-only compiler overlay omitted the first actual SIGSTOP and returned a
successful acknowledgement after healthy completion: the old helper failed after trying to kill the exited child;
the corrected observer rejected that attempt and then recorded an actual stopped
committed/pending child, SIGKILL and successful recovery. The race-enabled overlay
run passed all four selected criteria. This is fixture qualification, not live
published-candidate evidence.
