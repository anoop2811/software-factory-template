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
