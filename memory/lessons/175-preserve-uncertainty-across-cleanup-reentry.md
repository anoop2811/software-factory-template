# Preserve uncertainty across cleanup reentry

A read-only retry inside one cleanup call does not make signal-trap reentry
read-only. A nested cleanup call can observe different process state and resume
signaling unless uncertainty belongs to the owner, rather than a local probe.
Keep the uncertainty state until absence and terminal-child reaping are proved.
The policy remains in docs/adr/0150-bounded-source-group-reobservation.md:36.

Provenance: observed 2026-11-13 UTC through the independent private-launcher
regression in `acceptance/source_launcher_observation_test.go`. After the first
unknown observation, a handshake delivered TERM to the actual parent. Before the
latch correction, the signal witness recorded four attempts despite the final
125 refusal. The test controlled observations and recorded real PID/group
absence; this does not identify the unrecorded errno from the hosted CI failure.
