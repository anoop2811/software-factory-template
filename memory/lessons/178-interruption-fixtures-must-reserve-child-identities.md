# Interruption fixtures must reserve child identities

A consumed exit notification and a fresh liveness probe do not reserve a
numeric PID for a later signal. Test-only interruption helpers need the same
care as production supervisors: see the canonical boundary in
docs/adr/0151-reliable-ci-qualification.md:193.

Provenance: observed 2026-11-13 by tracing the asynchronous wait and later raw
signals in PR #180 checkpoint `8f8bbe8`; [Go's Darwin wait source](https://go.dev/src/os/wait_unimp.go)
was fetched the same day and documents this race. The independent control in
acceptance/interruption_process_controls_test.go:19 exercises terminal-state
observation, stopped interruption, cancellation and repeated cleanup. No
recycled PID or collateral signal was observed.
