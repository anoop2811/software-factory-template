# Exec failure can bypass shell exit traps

An EXIT trap does not reliably report a failed `exec`: the same broken-interpreter
probe bypassed the trap under Bash 3.2 and Bash 5, while Dash ran it. A controller
that accepts ordinary nonzero custom-runner exits therefore needs explicit
handoff state when it must distinguish its own adapter refusal from completed
work. Record pending before the handoff and acknowledge only a normal native
completion; retain the existing failure and ownership-uncertainty rules.

Provenance: observed 2026-11-03 via private shell probes in
`.factory/qualification/r3-runners/exit-transport-qa5h3e5l/results.json` and
`exec-trap-probe.c4jIqmgi/`. The canonical contract is
docs/adr/0133-go-source-evaluation-runners.md:193; this lesson does not extend
the trust boundary to arbitrary custom executables.
