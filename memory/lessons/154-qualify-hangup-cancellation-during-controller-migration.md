# Qualify hangup cancellation during controller migration

Compare the old controller's signal set when replacing its runtime. Covering
interrupt and termination alone can miss terminal hangup: an unhandled SIGHUP
can kill the controller before its child cleanup and accounting execute.

Observed 2026-11-06 via independent budget and loop regressions: after a fake
native child's stdin/PID readiness, the original Go parent exited -1 on SIGHUP;
the budget child stayed alive and its ledger row stayed active/running. The
legacy execution helper handled hangup. Adding SIGHUP to the existing shared
notification context reused its ownership cleanup instead of adding a handler.
The subsequent 36-criterion race focus passed: budget returned 130 with child
exit and interrupted accounting; loop returned 2 with an interrupted checkpoint.

Provenance: `internal/configuredcmd/command.go:25`,
`acceptance/source_budget_entrypoint_test.go:497`,
`acceptance/public_commands_test.go:176`, and
`docs/adr/0139-go-source-budget-entrypoint.md:91`.
Local RED receipts are `budget-hup-red` and `loop-hup-red`; GREEN is
`budget-public-green`, under `.factory/qualification/r3-budget/`. Decision 154
retains the contract and qualification scope. These probes used fake harnesses.
