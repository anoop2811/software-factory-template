# Budget Ginkgo repetitions as one command

Ginkgo CLI repetition can share a single overall timeout. Check the runner's
deadline semantics before multiplying a long suite; preserve individual operation
deadlines and distinguish runner exhaustion from a behavioral failure. Retain the
whole command log: a final JSON report can describe only its last repetition.

Provenance: observed 2026-11-12 while qualifying PR #180 with the repository's
Ginkgo v2.32.1. `--repeat=2 --timeout=15m` completed two passing suites and the
third forward case, then interrupted the third reverse case at the aggregate
limit. The missing reverse case passed separately under its original budget.
The installed module's `ginkgo/run/run_command.go:100` establishes the deadline
before the iteration loop; line 149 subtracts elapsed time for subsequent suites.
`types/config.go:557` defines `--repeat=N` as N+1 total runs. These source lines
were read directly from the pinned module on the same date.
