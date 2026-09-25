# Isolate overflow supervision from natural exit

An overflow fixture that exits immediately after writing can conflate output-limit
handling with process-group signaling after natural exit. Keep the fixture alive
until the supervisor terminates it, while retaining an external watchdog and
strict output-limit, reaping and cleared-accounting assertions. Do not suppress
production ownership uncertainty to make the fixture green.

Provenance: observed 2026-09-23 via `rtk proxy go run
/private/tmp/probe-zombie-group.go`: `iteration=0 kill=operation not permitted
wait=<nil> postreap=no such process`. PR #97 macOS job
https://github.com/anoop2811/software-factory-template/actions/runs/35691739847/job/106629994181
reported rapid overflow ownership uncertainty but did not expose its syscall
error; the probe is consistent with that failure, not proof of its exact cause.
The accepted test contract is in docs/adr/0070-go-budget-execution-controller.md
under Overflow fixture lifecycle.
