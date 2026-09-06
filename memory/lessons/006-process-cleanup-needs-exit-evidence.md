# Process cleanup needs exit evidence

Sending SIGKILL does not prove a child was reaped. Keep cleanup waits bounded,
close pipes and restore handlers on timeout, and preserve a reservation when
exit is unconfirmed. Record the spawned PID locally before fallible ledger
operations; otherwise a bookkeeping error can make a live child look unlaunched.
The contract remains in Decision 46 and docs/BUDGETS.md, not this lesson.

Provenance: PR #71 review, fetched 2026-09-06:
https://github.com/anoop2811/software-factory-template/pull/71#discussion_r3945278692
https://github.com/anoop2811/software-factory-template/pull/71#discussion_r3945278713
Python wait semantics fetched 2026-09-06:
https://docs.python.org/3/library/subprocess.html#subprocess.Popen.wait
Observed 2026-09-06 via independent actual run/execute fault injection: a
post-spawn ledger lock failure plus TimeoutExpired initially produced completed
launch_error, process_pid null and no subsequent admission blockers.
