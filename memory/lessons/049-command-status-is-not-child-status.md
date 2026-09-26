# Command status is not necessarily the child status

When moving an exec-based entrypoint to an in-process controller, preserve the
controller's public status contract rather than forwarding every native child
exit code. The budget runner starts with failure status 1 and maps completed,
timeout and interrupted outcomes separately; the raw native exit remains ledger
evidence. See internal/budget/runner.go:131 and internal/budget/runner.go:191.

A Cobra callback also has two channels: its Go error and the adapter's integer
status. A nil Go error can accompany an already-reported command failure. Public
routing therefore carries the integer through the outer dispatcher without
printing a second diagnostic; see internal/cli/cli.go:104 and
internal/cli/cli.go:120.

Provenance: observed 2026-09-26 during public command qualification when a test
expected native exit 7 as the public status but received the established adapter
status 1. Source inspection confirmed the contract, and the test was corrected
to assert both public status and persisted native exit. Governing requirements
remain [ADR-0078](../../docs/adr/0078-go-public-budget-loop.md).
