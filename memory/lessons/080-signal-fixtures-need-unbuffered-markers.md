# Signal fixtures need unbuffered markers

A Python signal handler that prints a test marker can interrupt another stdout
operation and raise a buffered-writer reentrancy RuntimeError. That failure can
masquerade as a process-control regression. Emit the small marker through the
file descriptor and retain the real signal, foreground and input assertions.

Provenance: Linux PR #125 job 117025821028 showed the actual exception;
`docs/adr/0098-whole-installation-upgrade-and-rollback.md:623` records the
correction contract. Python's authoritative explanation, fetched 2026-10-16:
https://docs.python.org/3/library/io.html#reentrancy

This lesson points to the decision rather than creating a second contract.
