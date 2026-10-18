# Verifier exit is not process-group closure

Waiting for a verifier's leader does not establish that its children have stopped.
A timeout can kill the leader while descendants retain streams or keep running
after closing them. Deleting its private inputs at that point loses custody while
work is still unresolved. A watchdog is itself responsible for reaping its timer.

Use the canonical ownership policy in
docs/adr/0101-go-release-handoff.md:392. Native archive verification shares the
existing bounded supervisor; bootstrap verification owns a bounded Bash job group.
Unconfirmed closure retains private evidence and reports the primary failure.

Provenance: observed 2026-10-18 through independent separate-process-group probes
of bootstrap failure and compiled Stage cancellation. The regressions and healthy
controls are acceptance/release_bootstrap_test.go:326 and
acceptance/release_bootstrap_test.go:366. Their production boundaries are
install.sh:111 and internal/staging/authentication.go:35. These tests include
descendants with open and closed streams and preserve an unrelated process group.
