# Process controls must reap their own fixtures

A cleanup test needs a cleanup path even when its own assertion aborts. For
process-group termination, a directly test-owned member can join the selected
group while remaining independently reapable by the evaluator. This avoids
depending on an orphan's adoption and reaping by the host. Keep a genuine nested
owner when cooperative nested cleanup is the behavior being tested.

Provenance: observed 2026-11-13 UTC in
`acceptance/fixture_process_controls_test.go`: disabling group cleanup made the
inherited-stream control fail in 6.404 seconds, and its deferred evaluator
cleanup still killed and reaped the fixture member. The unmodified four lifecycle
controls passed under `go test -race` in 3.454 seconds. The policy remains in
docs/adr/0151-reliable-ci-qualification.md:139; these observations concern the
test helper, not proof that a hosted CI failure was caused by leaked processes.
