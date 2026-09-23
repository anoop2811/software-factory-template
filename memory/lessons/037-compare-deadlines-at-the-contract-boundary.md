# Compare deadlines at the contract boundary

A reservation duration captured before durable publication cannot be compared
with remaining time after publication using a small arbitrary tolerance. Capture
the relevant phase boundary instead, and separately assert the original execution
context deadline. Always join test workers before closing descriptors, including
assertion-panic paths; success-path receives are insufficient synchronization.

Provenance: observed 2026-09-23 in PR97 Linux source job
https://github.com/anoop2811/software-factory-template/actions/runs/35867613075/job/107203107189
and independently reproduced with a 200ms syncFile delay via `go test -race
./internal/budget -ginkgo.focus="recomputes reservation duration" -count=1`.
The accepted boundary is documented in
 docs/adr/0070-go-budget-execution-controller.md under Admission duration regression boundary.
