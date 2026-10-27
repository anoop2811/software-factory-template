# A deadline does not prove the child is still alive

A shell `read` exits when its stdin closes. An empty-input fixture can therefore
finish naturally while a timeout test assumes it is still waiting. Preserve the
real prior-reap diagnostic instead of changing the supervisor to fit that
assumption. A test of pre-reap cleanup needs a child that cannot exit on EOF and
an outstanding waiter and PID-presence probe before its real termination signal.
Signal zero alone does not distinguish a running process from a zombie.

Canonical correction: docs/DECISION_LOG.md:4974. The fixture is
internal/native/natural_exit_test.go:77.

Provenance: observed 2026-10-26 UTC in
[PR #139's Linux source run](https://github.com/anoop2811/software-factory-template/actions/runs/40335635683).
The timeout case expected false but the actual diagnostic reported
`reaped_before_signal=true`. A separate direct `/bin/sh -c 'read line'` with stdin
from `/dev/null` exited 1 immediately. The runtime policy and deadlines are
unchanged. The local correction evidence on 2026-10-27 UTC records all 33 native
criteria passing; test-owned cleanup remains separate from the signal oracle.
