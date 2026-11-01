# Stop requests must survive PID publication

A watchdog can receive cancellation after starting its timer but before saving
the timer PID. Exiting from that window loses the child. Defer the stop request,
publish ownership, then kill and join the timer; share that pause for escalation.

Provenance:

- Observed 2026-10-31 via the public-bootstrap scheduling criterion in `acceptance/release_bootstrap_test.go:209`: the original installer completed authentication and literal forwarding but failed the pre-teardown timer-absence assertion. The repaired installer passed both available sh and dash iterations.
- The contract is `docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:203`; the shared pause is in `install.sh:75`.
- Both earlier shell controls closed captured output normally. This race does not establish the cause of the separately recorded hosted candidate timeout.
